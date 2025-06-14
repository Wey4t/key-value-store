package db

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	. "types"
	. "utils"
)

func mmapInit(fp *os.File) (int, []byte, error) {
	fi, err := fp.Stat()
	if err != nil {
		return 0, nil, fmt.Errorf("stat: %w", err)
	}
	if fi.Size()%BTREE_PAGE_SIZE != 0 {
		return 0, nil, errors.New("File size is not a multiple of page size.")
	}
	mmapSize := 64 << 20
	Assert(mmapSize%BTREE_PAGE_SIZE == 0)
	for mmapSize < int(fi.Size()) {
		mmapSize *= 2
	}
	// mmapSize can be larger than the file
	chunk, err := syscall.Mmap(
		int(fp.Fd()), 0, mmapSize,
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED,
	)
	if err != nil {
		return 0, nil, fmt.Errorf("mmap: %w", err)
	}
	return int(fi.Size()), chunk, nil
}

type KV struct {
	Path string
	// internals
	fp   *os.File
	tree BTree
	free FreeList
	mmap struct {
		file   int      // file size, can be larger than the database size
		total  int      // mmap size, can be larger than the file size
		chunks [][]byte // multiple mmaps, can be non-continuous
	}
	page struct {
		flushed uint64   // database size in number of pages
		temp    [][]byte // newly allocated pages
		nfree   int      // number of pages taken from the free list
		nappend int      // number of pages to be appended
		// newly allocated or deallocated pages keyed by the pointer.
		// nil value denotes a deallocated page.
		updates map[uint64][]byte
	}
	mu     sync.Mutex
	writer sync.Mutex
}

// extend the mmap by adding new mappings.
func extendMmap(db *KV, npages int) error {
	if db.mmap.total >= npages*BTREE_PAGE_SIZE {
		return nil
	}
	// double the address space
	chunk, err := syscall.Mmap(
		int(db.fp.Fd()), 0, db.mmap.total,
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED,
	)
	if err != nil {
		return fmt.Errorf("mmap: %w", err)
	}
	db.mmap.total += db.mmap.total
	db.mmap.chunks = append(db.mmap.chunks, chunk)
	return nil
}
func NewKv(path string) *KV {
	kv := &KV{Path: path}
	kv.page.updates = make(map[uint64][]byte)          // simulate some free pages
	kv.page.updates[0] = make([]byte, BTREE_PAGE_SIZE) // simulate a master page

	// db.free.Update(db.page.nfree, []uint64{})          // initialize free list

	return kv
}
func (db *KV) Update(key []byte, val []byte, mode int) (bool, error) {
	_, ok := db.Get(key)
	if ok && mode == MODE_INSERT_ONLY {
		return false, errors.New("key exist")
	} else if !ok && mode == MODE_UPDATE_ONLY {
		return false, errors.New("key not exist")
	}
	err := db.Set(key, val)
	if err != nil {
		return false, err
	} else {
		return true, nil
	}
	// return false, errors.New("operation error")

}

func (db *KV) pageGet(ptr uint64) BNode {
	if page, ok := db.page.updates[ptr]; ok {
		Assert(page != nil)
		return BNode(page) // for new pages
	}
	return pageGetMapped(db, ptr) // for written pages
}
func pageGetMapped(db *KV, ptr uint64) []byte {
	start := uint64(0)
	for _, chunk := range db.mmap.chunks {
		end := start + uint64(len(chunk))/BTREE_PAGE_SIZE

		if ptr < end {
			offset := BTREE_PAGE_SIZE * (ptr - start)
			return chunk[offset : offset+BTREE_PAGE_SIZE]
		}
		start = end
	}
	fmt.Println("ptr:", ptr, "start:", start, "chunks:", len(db.mmap.chunks))
	panic("bad ptr")
}

const DB_SIG = "BuildYourOwnDB05"

// the master page format.
// it contains the pointer to the root and other important bits.
// | sig | btree_root | page_used |
// | 16B | 8B | 8B |
func masterLoad(db *KV) error {
	if db.mmap.file == 0 {
		// empty file, the master page will be created on the first write.
		db.page.flushed = 1 // reserved for the master page
		return nil
	}
	return loadMeta(db, db.mmap.chunks[0])

}

// update the master page. it must be atomic.
func masterStore(db *KV) error {
	var data [BTREE_PAGE_SIZE]byte
	copy(data[:16], []byte(DB_SIG))
	db.page.flushed = uint64(len(db.page.updates))
	binary.LittleEndian.PutUint64(data[16:], db.tree.Root)
	binary.LittleEndian.PutUint64(data[24:], db.page.flushed)
	binary.LittleEndian.PutUint64(data[32:], db.free.head)
	// NOTE: Updating the page via mmap is not atomic.
	// Use the pwrite() syscall instead.
	_, err := db.fp.WriteAt(data[:], 0)
	if err != nil {
		return fmt.Errorf("write master page: %w", err)
	}
	return nil
}

func (db *KV) pageNew(node []byte) uint64 {
	Assert(len(node) <= BTREE_PAGE_SIZE)
	ptr := uint64(0)
	if db.page.nfree < db.free.Total() {
		// reuse a deallocated page
		ptr = db.free.Get(db.page.nfree)
		db.page.nfree++
	} else {
		// append a new page
		ptr = db.page.flushed + uint64(db.page.nappend)
		db.page.nappend++
	}
	db.page.updates[ptr] = node
	return ptr
}

// callback for BTree, deallocate a page.
func (db *KV) pageDel(ptr uint64) {
	db.page.updates[ptr] = nil
}

// callback for FreeList, allocate a new page.
func (db *KV) pageAppend(node BNode) uint64 {
	Assert(len(node) <= BTREE_PAGE_SIZE)
	ptr := db.page.flushed + uint64(db.page.nappend)
	db.page.nappend++
	db.page.updates[ptr] = node
	return ptr
}

// callback for FreeList, reuse a page.
func (db *KV) pageUse(ptr uint64, node BNode) {
	// clean the page to avoid stale data.

	db.page.updates[ptr] = node
}

// extend the file to at least npages .
func extendFile(db *KV, npages int) error {
	filePages := db.mmap.file / BTREE_PAGE_SIZE
	if filePages >= npages {
		return nil
	}
	for filePages < npages {
		// the file size is increased exponentially,
		// so that we don't have to extend the file for every update.
		inc := filePages / 8
		if inc < 1 {
			inc = 1
		}
		filePages += inc
	}
	fileSize := filePages * BTREE_PAGE_SIZE
	err := syscall.Ftruncate(int(db.fp.Fd()), int64(fileSize))
	if err != nil {
		return fmt.Errorf("fallocate: %w", err)
	}
	db.mmap.file = fileSize
	return nil
}

func (db *KV) Open() error {
	// open or create the DB file
	fp, err := os.OpenFile(db.Path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("OpenFile: %w", err)
	}
	db.page.updates = make(map[uint64][]byte)
	node := make(BNode, BTREE_PAGE_SIZE)
	db.page.updates[0] = node
	db.fp = fp
	// create the initial mmap
	sz, chunk, err := mmapInit(db.fp)
	if err != nil {
		goto fail
	}

	// btree callbacks
	db.mmap.file = sz
	db.mmap.total = len(chunk)
	db.mmap.chunks = [][]byte{chunk}

	// btree callbacks
	db.tree.Get = db.pageGet
	db.tree.New = db.pageNew
	db.tree.Del = db.pageDel
	db.free.get = db.pageGet
	db.free.new = db.pageAppend
	db.free.use = db.pageUse

	// db.page.nappend = 0
	// db.page.nfree = 0
	// free list
	// read the master page
	err = masterLoad(db)
	if err != nil {
		goto fail
	}

	for i := uint64(1); i < db.page.flushed; i++ {
		node := make(BNode, BTREE_PAGE_SIZE)
		copy(node, pageGetMapped(db, i))
		db.page.updates[i] = node
	}
	// done
	// if db.free.head == 0 {

	// 	node := make(BNode, BTREE_PAGE_SIZE)
	// 	db.free.head = db.free.new(node)
	// }
	return nil
fail:
	db.Close()
	return fmt.Errorf("KV.Open: %w", err)
}

// cleanups
func (db *KV) Close() {
	writePages(db)
	syncPages(db)

	db.fp.Sync()
	masterStore(db) // update the master page
	for _, chunk := range db.mmap.chunks {
		err := syscall.Munmap(chunk)
		Assert(err == nil)
	}
	db.fp.Close()
}

// read the db
func (db *KV) Get(key []byte) ([]byte, bool) {
	return db.tree.Read(key)
}

// update the db
func (db *KV) Set(key []byte, val []byte) error {
	db.tree.Insert(key, val)
	return flushPages(db)
}
func (db *KV) Del(key []byte) (bool, error) {
	deleted := db.tree.Delete(key)
	return deleted, flushPages(db)
}

// persist the newly allocated pages after updates
func flushPages(db *KV) error {
	if err := writePages(db); err != nil {
		return err
	}
	return syncPages(db)
}

// func writePages(db *KV) error {
// 	// extend the file & mmap if needed
// 	npages := int(db.page.flushed) + len(db.page.temp)
// 	if err := extendFile(db, npages); err != nil {
// 		return err
// 	}
// 	if err := extendMmap(db, npages); err != nil {
// 		return err
// 	}
// 	// copy data to the file
// 	for i, page := range db.page.temp {
// 		ptr := db.page.flushed + uint64(i)
// 		copy(db.pageGet(ptr), page)
// 	}
// 	return nil
// }

func writePages(db *KV) error {
	// update the free list
	freed := []uint64{}
	for ptr, page := range db.page.updates {
		if page == nil {
			freed = append(freed, ptr)
		}
	}
	db.free.Update(db.page.nfree, freed)
	// extend the file & mmap if needed
	npages := int(db.page.flushed) + db.page.nappend
	// fmt.Println("npages:", npages)
	if err := extendFile(db, npages); err != nil {
		return err
	}
	if err := extendMmap(db, npages); err != nil {
		return err
	}

	// copy pages to the file
	for ptr, page := range db.page.updates {
		if page != nil && ptr != 0 {
			copy(pageGetMapped(db, ptr), page)
		}
	}
	return nil
}
func syncPages(db *KV) error {
	// flush data to the disk. must be done before updating the master page.
	if err := db.fp.Sync(); err != nil {
		return fmt.Errorf("fsync: %w", err)
	}
	db.page.flushed += uint64(len(db.page.temp))
	db.page.temp = db.page.temp[:0]
	// update & flush the master page
	if err := masterStore(db); err != nil {
		return err
	}
	if err := db.fp.Sync(); err != nil {
		return fmt.Errorf("fsync: %w", err)
	}
	return nil
}
func loadMeta(db *KV, data []byte) error {
	Assert(bytes.Compare(data[:16], []byte(DB_SIG)) == 0)
	root := binary.LittleEndian.Uint64(data[16:])
	used := binary.LittleEndian.Uint64(data[24:])
	head := binary.LittleEndian.Uint64(data[32:])
	// verify the page
	if !bytes.Equal([]byte(DB_SIG), data[:16]) {
		return errors.New("Bad signature.")
	}
	bad := !(1 <= used && used <= uint64(db.mmap.file/BTREE_PAGE_SIZE))
	bad = bad || !(0 <= root && root < used)
	if bad {
		return errors.New("Bad master page.")
	}
	db.tree.Root = root
	db.page.flushed = used
	db.free.head = head
	return nil
}
func saveMeta(db *KV) []byte {
	var data [32]byte
	copy(data[:16], []byte(DB_SIG))
	binary.LittleEndian.PutUint64(data[16:], db.tree.Root)
	binary.LittleEndian.PutUint64(data[24:], db.page.flushed)
	return data[:]
}
func readRoot(db *KV, fileSize int64) error {
	if fileSize == 0 { // empty file
		db.page.flushed = 1 // the meta page is initialized on the 1st write
		return nil
	}
	// read the page
	data := db.mmap.chunks[0]
	loadMeta(db, data)
	// verify the page
	// ...
	return nil
}
func updateRoot(db *KV) error {
	if _, err := syscall.Pwrite(int(db.fp.Fd()), saveMeta(db), 0); err != nil {
		return fmt.Errorf("write meta page: %w", err)
	}
	return nil
}
func Bnode_to_string(b BNode, id uint64) string {
	if len(b) == 0 {
		return "(empty)"
	}
	var str string
	str += fmt.Sprintf("(%d)", id)
	for i := uint16(0); i < b.Nkeys(); i++ {
		str += fmt.Sprintf("%x,%s ||||", b.GetKey(i), b.GetVal(i))
	}
	return str
}

func (kv *KV) GetTree() *BTree {
	return &kv.tree
}
