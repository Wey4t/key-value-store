package db

import (
	"encoding/binary"
	"fmt"
	. "types"
	. "utils"
)

// The node format:
// | type | size | total | next | pointers |
// | 2B   | 2B   | 8B    | 8B   | size * 8B |

const BNODE_FREE_LIST = 3
const FREE_LIST_HEADER = 4 + 8 + 8

const FREE_LIST_CAP = (BTREE_PAGE_SIZE - FREE_LIST_HEADER) / 8

type FreeList struct {
	head uint64
	// callbacks for managing on-disk pages
	get func(uint64) BNode  // dereference a pointer
	new func(BNode) uint64  // append a new page
	use func(uint64, BNode) // reuse a page
}

// number of items in the list
func (fl *FreeList) Total() int {
	node := fl.get(fl.head)
	return int(binary.LittleEndian.Uint64(node[4:12]))
}

// remove popn pointers and add some new pointers
// func (fl *FreeList) Update(popn int, freed []uint64)
func flnSize(node BNode) int {
	return int(binary.LittleEndian.Uint16(node[2:4]))
}
func flnNext(node BNode) uint64 {
	return binary.LittleEndian.Uint64(node[12:20])
}
func flnPtr(node BNode, idx int) uint64 {
	return binary.LittleEndian.Uint64(node[FREE_LIST_HEADER+8*idx:])
}
func flnSetPtr(node BNode, idx int, ptr uint64) {
	binary.LittleEndian.PutUint64(node[FREE_LIST_HEADER+8*idx:], ptr)
}
func flnSetHeader(node BNode, size uint16, next uint64) {
	binary.LittleEndian.PutUint16(node[0:2], BNODE_FREE_LIST)
	binary.LittleEndian.PutUint16(node[2:4], size)
	binary.LittleEndian.PutUint64(node[12:20], next)
}
func flnSetTotal(node BNode, total uint64) {
	binary.LittleEndian.PutUint64(node[4:12], total)
}

func (fl *FreeList) Get(topn int) uint64 {
	Assert(0 <= topn && topn < fl.Total())
	node := fl.get(fl.head)
	for flnSize(node) <= topn {
		topn -= flnSize(node)
		next := flnNext(node)
		Assert(next != 0)
		node = fl.get(next)
	}
	return flnPtr(node, flnSize(node)-topn-1)
}

func flPush(fl *FreeList, freed []uint64, reuse []uint64) {
	for len(freed) > 0 {
		new := BNode(make([]byte, BTREE_PAGE_SIZE))
		// construct a new node
		size := len(freed)
		if size > FREE_LIST_CAP {
			size = FREE_LIST_CAP
		}
		// prepend new head to the list
		flnSetHeader(new, uint16(size), fl.head)
		for i, ptr := range freed[:size] {
			flnSetPtr(new, i, ptr)
		}
		freed = freed[size:]
		// update the total count
		total := uint64(fl.Total() + size)
		flnSetTotal(new, total)
		if len(reuse) > 0 {
			// reuse a pointer from the list
			fl.head, reuse = reuse[0], reuse[1:]
			fl.use(fl.head, new)
		} else {
			// or append a page to house the new node
			fl.head = fl.new(new)
		}
	}
	// checkAssertion(len(reuse) == 0)
	// don't know why reuse should == 0
	if len(reuse) > 0 {
		flPush(fl, reuse, nil)
	}
}

func (fl *FreeList) Update(popn int, freed []uint64) {
	Assert(popn <= fl.Total())
	if popn == 0 && len(freed) == 0 {
		return // nothing to do
	}
	// prepare to construct the new list
	total := fl.Total()
	reuse := []uint64{}
	for fl.head != 0 && len(reuse)*FREE_LIST_CAP < len(freed) {
		node := fl.get(fl.head)
		freed = append(freed, fl.head) // recyle the node itself
		if popn >= flnSize(node) {
			// phase 1
			// remove all pointers in this node
			popn -= flnSize(node)
		} else {
			// phase 2:
			// remove some pointers
			remain := flnSize(node) - popn
			popn = 0
			// reuse pointers from the free list itself
			for remain > 0 && len(reuse)*FREE_LIST_CAP < len(freed)+remain {
				remain--
				reuse = append(reuse, flnPtr(node, remain))
			}
			// move the node into the freed list
			for i := 0; i < remain; i++ {
				freed = append(freed, flnPtr(node, i))
			}
		}
		// discard the node and move to the next node
		total -= flnSize(node)
		fl.head = flnNext(node)
	}
	Assert(len(reuse)*FREE_LIST_CAP >= len(freed) || fl.head == 0)
	// phase 3: prepend new nodes
	flPush(fl, freed, reuse)
	// done
	flnSetTotal(fl.get(fl.head), uint64(total+len(freed)))
}

func (fl *FreeList) DebugPrint() {
	fmt.Println("\n=== FREE LIST DEBUG ===")
	fmt.Printf("Head: %d\n", fl.head)
	fmt.Printf("Total Free Pages: %d\n", fl.Total())

	if fl.head == 0 {
		fmt.Println("Free list is empty")
		return
	}

	nodeNum := 1
	current := fl.head
	totalFound := 0

	for current != 0 {
		node := fl.get(current)
		size := flnSize(node)
		next := flnNext(node)

		fmt.Printf("\n--- Node %d (Page %d) ---\n", nodeNum, current)
		fmt.Printf("  Size: %d pointers\n", size)
		fmt.Printf("  Total: %d\n", binary.LittleEndian.Uint64(node[4:12]))
		fmt.Printf("  Next: %d\n", next)
		fmt.Printf("  Pointers: [")

		for i := 0; i < size; i++ {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("%d", flnPtr(node, i))
		}
		fmt.Printf("]\n")

		totalFound += size
		nodeNum++
		current = next

		// Safety check to prevent infinite loops
		if nodeNum > 100 {
			fmt.Printf("  ... (truncated - too many nodes)")
			break
		}
	}

	fmt.Printf("\nSummary: %d nodes, %d total pointers found\n", nodeNum-1, totalFound)
	fmt.Printf("========================\n")
}
