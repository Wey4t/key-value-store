package types

import (
	"bytes"
	"encoding/binary"
)

type BNode []byte

// | key_size | val_size | key | val |
// |    2B    |    2B    | ... | ... |
// getters
// Here is our node format. The 2nd row is the encoded field size in bytes.

// | type | nkeys |  pointers  |  offsets   | key-values | unused |
// |  2B  |   2B  | nkeys × 8B | nkeys × 2B |     ...    |        |

const (
	BNODE_NODE = 1 // internal nodes with pointers
	BNODE_LEAF = 2 // leaf nodes with values

)
const HEADER = 4
const BTREE_PAGE_SIZE = 4096
const BTREE_MAX_KEY_SIZE = 1000
const BTREE_MAX_VAL_SIZE = 3000

func checkAssertion(cond bool) {
	if !cond {
		panic("Assertion failed")
	}
}
func (node BNode) Ntype() uint16 {
	return binary.LittleEndian.Uint16(node[0:2])
}
func (node BNode) Nkeys() uint16 {
	return binary.LittleEndian.Uint16(node[2:4])
}

func (node BNode) SetHeader(btype uint16, nkeys uint16) {
	binary.LittleEndian.PutUint16(node[0:2], btype)
	binary.LittleEndian.PutUint16(node[2:4], nkeys)
}

// read and write the child pointers array
func (node BNode) GetPtr(idx uint16) uint64 {

	checkAssertion(idx < node.Nkeys())
	pos := 4 + 8*idx
	return binary.LittleEndian.Uint64(node[pos:])
}
func (node BNode) SetPtr(idx uint16, val uint64) {

	checkAssertion(idx < node.Nkeys())
	pos := 4 + 8*idx
	binary.LittleEndian.PutUint64(node[pos:], val)
}
func (node BNode) SetOffset(idx uint16, val uint16) {
	pos := 4 + 8*node.Nkeys() + 2*(idx-1)
	binary.LittleEndian.PutUint16(node[pos:], val)
}
func (node BNode) GetOffset(idx uint16) uint16 {
	if idx == 0 {
		return 0
	}
	pos := 4 + 8*node.Nkeys() + 2*(idx-1)
	return binary.LittleEndian.Uint16(node[pos:])
}
func (node BNode) kvPos(idx uint16) uint16 {
	checkAssertion(idx <= node.Nkeys())
	return 4 + 8*node.Nkeys() + 2*node.Nkeys() + node.GetOffset(idx)
}
func (node BNode) GetKey(idx uint16) []byte {
	checkAssertion(idx < node.Nkeys())
	pos := node.kvPos(idx)
	klen := binary.LittleEndian.Uint16(node[pos:])
	return node[pos+4:][:klen]
}
func (node BNode) GetVal(idx uint16) []byte {
	checkAssertion(idx < node.Nkeys())
	pos := node.kvPos(idx)
	klen := binary.LittleEndian.Uint16(node[pos+0:])
	vlen := binary.LittleEndian.Uint16(node[pos+2:])
	return node[pos+4+klen:][:vlen]
}

func nodeAppendKV(new BNode, idx uint16, ptr uint64, key []byte, val []byte) {

	new.SetPtr(idx, ptr)
	ofs := new.GetOffset(idx)
	pos := new.kvPos(idx) // get the position for the new key-value pair
	// write the key size and value size
	binary.LittleEndian.PutUint16(new[pos:], uint16(len(key)))
	binary.LittleEndian.PutUint16(new[pos+2:], uint16(len(val)))
	// write the key and value
	copy(new[pos+4:], key)
	copy(new[pos+4+uint16(len(key)):], val)
	new.SetOffset(idx+1, ofs+uint16(len(key)+len(val)+4)) // update the offset for next key-value
}
func (node BNode) Nbytes() uint16 {
	return node.kvPos(node.Nkeys()) // uses the offset value of the last key
}

func LeafInsert(
	new BNode, old BNode, idx uint16, key []byte, val []byte,
) {
	new.SetHeader(BNODE_LEAF, old.Nkeys()+1)
	nodeAppendRange(new, old, 0, 0, idx)                   // copy the keys before `idx`
	nodeAppendKV(new, idx, 0, key, val)                    // the new key
	nodeAppendRange(new, old, idx+1, idx, old.Nkeys()-idx) // keys from `idx`
}
func NodeDeleteKV(new BNode, old BNode, target uint16) {
	// delete the key at `target` from `old` and copy to `new`
	new.SetHeader(old.Ntype(), old.Nkeys()-1)
	nodeAppendRange(new, old, 0, 0, target)                             // copy keys before target
	nodeAppendRange(new, old, target, target+1, old.Nkeys()-(target+1)) // copy keys after target
	// reset the offsets for the new node
	for i := uint16(0); i < new.Nkeys(); i++ {
		new.SetOffset(i+1, new.GetOffset(i))
	}

}
func nodeAppendRange(
	new BNode, old BNode,
	dstNew uint16, srcOld uint16, n uint16,
) {
	checkAssertion(srcOld+n <= old.Nkeys())
	checkAssertion(dstNew+n <= new.Nkeys())
	// print the key in old
	if n == 0 {
		return
	}
	// pointers
	for i := uint16(0); i < n; i++ {
		new.SetPtr(dstNew+i, old.GetPtr(srcOld+i))
	}
	// offsets
	dstBegin := new.GetOffset(dstNew)
	srcBegin := old.GetOffset(srcOld)
	for i := uint16(1); i <= n; i++ { // NOTE: the range is [1, n]
		offset := dstBegin + old.GetOffset(srcOld+i) - srcBegin
		new.SetOffset(dstNew+i, offset)
	}
	// KVs
	begin := old.kvPos(srcOld)
	end := old.kvPos(srcOld + n)

	copy(new[new.kvPos(dstNew):], old[begin:end])
}

// copy multiple keys, values, and pointers into the position
// func nodeAppendRange(
//
//	new BNode, old BNode, dstNew uint16, srcOld uint16, n uint16,
//
//	) {
//		for i := uint16(0); i < n; i++ {
//			dst, src := dstNew+i, srcOld+i
//			nodeAppendKV(new, dst,
//				old.GetPtr(src), old.GetKey(src), old.GetVal(src))
//		}
//	}
func LeafUpdate(
	new BNode, old BNode, idx uint16, key []byte, val []byte,
) {
	new.SetHeader(BNODE_LEAF, old.Nkeys())
	nodeAppendRange(new, old, 0, 0, idx)
	nodeAppendKV(new, idx, 0, key, val)
	nodeAppendRange(new, old, idx+1, idx+1, old.Nkeys()-(idx+1))
}

// find the last postion that is less than or equal to the key
func nodeLookupLE(node BNode, key []byte) uint16 {
	nkeys := node.Nkeys()
	var i uint16
	for i = 0; i < nkeys; i++ {
		cmp := bytes.Compare(node.GetKey(i), key)
		if cmp == 0 {
			return i
		}
		if cmp > 0 {
			return i - 1
		}
	}
	return i - 1
}

// Split an oversized node into 2 nodes. The 2nd node always fits.
func nodeSplit2(left BNode, right BNode, old BNode) {
	// the initial guess
	nleft := old.Nkeys() / 2
	// try to fit the left half
	left_bytes := func() uint16 {
		return 4 + 8*nleft + 2*nleft + old.GetOffset(nleft)
	}
	for left_bytes() > BTREE_PAGE_SIZE {
		nleft--
	}
	checkAssertion(nleft >= 1)
	// try to fit the right half
	right_bytes := func() uint16 {
		return old.Nbytes() - left_bytes() + 4
	}
	for right_bytes() > BTREE_PAGE_SIZE {
		nleft++
	}
	checkAssertion(nleft < old.Nkeys())
	nright := old.Nkeys() - nleft
	// new nodes
	left.SetHeader(old.Ntype(), nleft)
	right.SetHeader(old.Ntype(), nright)
	nodeAppendRange(left, old, 0, 0, nleft)
	nodeAppendRange(right, old, 0, nleft, nright)
	// NOTE: the left half may be still too big
	checkAssertion(right.Nbytes() <= BTREE_PAGE_SIZE)
}
func NodeReplaceKidN(
	tree *BTree, new BNode, old BNode, idx uint16,
	kids ...BNode,
) {
	inc := uint16(len(kids))
	new.SetHeader(BNODE_NODE, old.Nkeys()+inc-1)
	nodeAppendRange(new, old, 0, 0, idx)
	for i, node := range kids {
		nodeAppendKV(new, idx+uint16(i), tree.New(node), node.GetKey(0), nil)
	}
	nodeAppendRange(new, old, idx+inc, idx+1, old.Nkeys()-(idx+1))
}
func NodeSplit3(old BNode) (uint16, [3]BNode) {
	if old.Nbytes() <= BTREE_PAGE_SIZE {
		old = old[:BTREE_PAGE_SIZE]
		return 1, [3]BNode{old} // not split
	}
	left := BNode(make([]byte, 2*BTREE_PAGE_SIZE)) // might be split later
	right := BNode(make([]byte, BTREE_PAGE_SIZE))
	nodeSplit2(left, right, old)
	if left.Nbytes() <= BTREE_PAGE_SIZE {
		left = left[:BTREE_PAGE_SIZE]
		return 2, [3]BNode{left, right} // 2 nodes
	}
	leftleft := BNode(make([]byte, BTREE_PAGE_SIZE))
	middle := BNode(make([]byte, BTREE_PAGE_SIZE))
	nodeSplit2(leftleft, middle, left)
	checkAssertion(leftleft.Nbytes() <= BTREE_PAGE_SIZE)
	return 3, [3]BNode{leftleft, middle, right} // 3 nodes
}
