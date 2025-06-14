package types

import (
	"bytes"
)

type BTree struct {
	// Root pointer (a nonzero page number)
	Root uint64
	// callbacks for managing on-disk pages
	Get func(uint64) BNode  // read data from a page number
	New func([]byte) uint64 // allocate a new page number with data
	Del func(uint64)        // deallocate a page number
}

func treeInsert(tree *BTree, node BNode, key []byte, val []byte) BNode {
	// the result node.
	// it's allowed to be bigger than 1 page and will be split if so
	new := BNode(make([]byte, 2*BTREE_PAGE_SIZE))
	// where to insert the key?
	idx := nodeLookupLE(node, key)
	// act depending on the node type
	switch node.Ntype() {
	case BNODE_LEAF:
		// leaf, node.getKey(idx) <= key
		if bytes.Equal(key, node.GetKey(idx)) {
			// found the key, update it.
			LeafUpdate(new, node, idx, key, val)
		} else {
			// insert it after the position.
			LeafInsert(new, node, idx+1, key, val)
		}
	case BNODE_NODE:
		// internal node, insert it to a kid node.
		nodeInsert(tree, new, node, idx, key, val)
	default:
		panic("bad node!")
	}
	return new
}

// part of the treeInsert(): KV insertion to an internal node
func nodeInsert(
	tree *BTree, new BNode, node BNode, idx uint16,
	key []byte, val []byte,
) {
	// get and deallocate the kid node
	kptr := node.GetPtr(idx)
	knode := tree.Get(kptr)
	tree.Del(kptr)
	// recursive insertion to the kid node
	knode = treeInsert(tree, knode, key, val)
	// split the result
	nsplit, splited := NodeSplit3(knode)
	// update the kid links
	NodeReplaceKidN(tree, new, node, idx, splited[:nsplit]...)
}
func TreefindKey(tree *BTree, node BNode, key []byte) ([]byte, bool) {
	// find the key in the node
	idx := nodeLookupLE(node, key)
	switch node.Ntype() {
	case BNODE_LEAF: // leaf node
		if idx <= node.Nkeys()-1 && bytes.Equal(node.GetKey(idx), key) {
			val := node.GetVal(idx)
			return val, true // found
		} else if idx+1 <= node.Nkeys()-1 && bytes.Equal(node.GetKey(idx+1), key) {
			val := node.GetVal(idx + 1)
			return val, true // found
		}
	case BNODE_NODE:
		kptr := node.GetPtr(idx)
		return TreefindKey(tree, tree.Get(kptr), key) // found
	}
	return nil, false
}
func (tree *BTree) Read(key []byte) ([]byte, bool) {
	if tree.Root == 0 {
		return nil, false // empty tree
	}

	node := BNode(tree.Get(tree.Root))
	return TreefindKey(tree, node, key)
}

func (tree *BTree) Delete(key []byte) bool {
	checkAssertion(len(key) != 0)
	checkAssertion(len(key) <= BTREE_MAX_KEY_SIZE)
	if tree.Root == 0 {
		return false
	}
	updated := TreeDelete(tree, tree.Get(tree.Root), key)
	if len(updated) == 0 {
		return false // not found
	}
	tree.Del(tree.Root)
	if updated.Ntype() == BNODE_NODE && updated.Nkeys() == 1 {
		// remove a level
		tree.Root = updated.GetPtr(0)
	} else {
		tree.Root = tree.New(updated)
	}
	return true
}
func (tree *BTree) Insert(key []byte, val []byte) error {
	// 1. check the length limit imposed by the node format
	// if err := checkLimit(key, val); err != nil {
	// 	return err // the only way for an update to fail
	// }
	// 2. create the first node
	if tree.Root == 0 {
		root := BNode(make([]byte, BTREE_PAGE_SIZE))
		root.SetHeader(BNODE_LEAF, 2)
		// a dummy key, this makes the tree cover the whole key space.
		// thus a lookup can always find a containing node.
		nodeAppendKV(root, 0, 0, nil, nil)
		nodeAppendKV(root, 1, 0, key, val)
		tree.Root = tree.New(root)
		return nil
	}
	node := tree.Get(tree.Root)
	tree.Del(tree.Root)
	node = treeInsert(tree, node, key, val)
	nsplit, splitted := NodeSplit3(node)
	if nsplit > 1 {
		// the root was split, add a new level.
		root := BNode(make([]byte, BTREE_PAGE_SIZE))
		root.SetHeader(BNODE_NODE, nsplit)
		for i, knode := range splitted[:nsplit] {
			ptr, key := tree.New(knode), knode.GetKey(0)
			nodeAppendKV(root, uint16(i), ptr, key, nil)
		}
		tree.Root = tree.New(root)
	} else {
		tree.Root = tree.New(splitted[0])
	}
	return nil
}

// remove a key from a leaf node
func leafDelete(new BNode, old BNode, idx uint16) {
	new.SetHeader(BNODE_LEAF, old.Nkeys()-1)
	nodeAppendRange(new, old, 0, 0, idx)
	nodeAppendRange(new, old, idx, idx+1, old.Nkeys()-(idx+1))
}

// merge 2 nodes into 1
func nodeMerge(new BNode, left BNode, right BNode) {
	checkAssertion(left.Ntype() == right.Ntype())
	new.SetHeader(left.Ntype(), left.Nkeys()+right.Nkeys())
	nodeAppendRange(new, left, 0, 0, left.Nkeys()) // copy left
	nodeAppendRange(new, right, left.Nkeys(), 0, right.Nkeys())
	// reset the pointers for the merged node
	for i := uint16(0); i < new.Nkeys(); i++ {
		new.SetPtr(i, new.GetPtr(i))
	}

}

// replace 2 adjacent links with 1
func nodeReplace2Kid(new BNode, old BNode, idx uint16, ptr uint64, key []byte) {
	new.SetHeader(BNODE_NODE, old.Nkeys()-1)
	nodeAppendRange(new, old, 0, 0, idx)
	nodeAppendKV(new, idx, ptr, key, nil)
	nodeAppendRange(new, old, idx+1, idx+2, new.Nkeys()-(idx+1))
}

// should the updated kid be merged with a sibling?
func shouldMerge(tree *BTree, node BNode, idx uint16, updated BNode) (int, BNode) {
	if updated.Nbytes() > BTREE_PAGE_SIZE/4 {
		return 0, BNode{}
	}

	if idx > 0 {
		sibling := BNode(tree.Get(node.GetPtr(idx - 1)))
		merged := sibling.Nbytes() + updated.Nbytes() - HEADER
		if merged <= BTREE_PAGE_SIZE {
			return -1, sibling
		}
	}

	if idx+1 < node.Nkeys() {
		sibling := BNode(tree.Get(node.GetPtr(idx + 1)))
		merged := sibling.Nbytes() + updated.Nbytes() - HEADER
		if merged <= BTREE_PAGE_SIZE {
			return +1, sibling
		}
	}

	return 0, BNode{}
}

// delete a key from the tree
func TreeDelete(tree *BTree, node BNode, key []byte) BNode {
	// where to find the key?
	idx := nodeLookupLE(node, key)
	// act depending on the node type
	switch node.Ntype() {
	case BNODE_LEAF:
		if !bytes.Equal(key, node.GetKey(idx)) {
			return BNode{} // not found
		}
		// delete the key in the leaf
		new := BNode(make([]byte, BTREE_PAGE_SIZE))
		leafDelete(new, node, idx)
		return new
	case BNODE_NODE:
		return nodeDelete(tree, node, idx, key)
	default:
		panic("bad node!")
	}
	// recurse into the kid
}

// delete a key from an internal node; part of the treeDelete()

// func nodeDelete(tree *BTree, node BNode, idx uint16, key []byte) BNode {
// 	// recurse into the kid
// 	kptr := node.GetPtr(idx)
// 	updated := TreeDelete(tree, tree.get(kptr), key)
// 	if len(updated) == 0 {
// 		node.SetPtr(idx, 0) // update the pointer

// 		return BNode{} // not found
// 	}
// 	tree.del(kptr)
// 	// check for merging
// 	new := BNode(make([]byte, BTREE_PAGE_SIZE))
// 	mergeDir, sibling := shouldMerge(tree, node, idx, updated)
// 	switch {
// 	case mergeDir < 0: // left
// 		merged := BNode(make([]byte, BTREE_PAGE_SIZE))
// 		nodeMerge(merged, sibling, updated)
// 		tree.del(node.GetPtr(idx - 1))
// 		nodeReplace2Kid(new, node, idx-1, tree.new(merged), merged.GetKey(0))
// 	case mergeDir > 0: // right
// 		merged := BNode(make([]byte, BTREE_PAGE_SIZE))
// 		nodeMerge(merged, updated, sibling)
// 		tree.del(node.GetPtr(idx + 1))
// 		nodeReplace2Kid(new, node, idx, tree.new(merged), merged.GetKey(0))
// 	case mergeDir == 0 && updated.Nkeys() == 0:
// 		// assert(node.nkeys() == 1 && idx == 0) // 1 empty child but no sibling
// 		node.SetPtr(idx, 0) // update the pointer

// 	case mergeDir == 0 && updated.Nkeys() > 0: // no merge
// 		NodeReplaceKidN(tree, new, node, idx, updated)

// 	}
// 	return new
// }

func nodeDelete(tree *BTree, node BNode, idx uint16, key []byte) BNode {
	// recurse into the kid
	kptr := node.GetPtr(idx)
	updated := TreeDelete(tree, tree.Get(kptr), key)
	if len(updated) == 0 {
		return BNode{} // not found
	}
	tree.Del(kptr)
	new := BNode(make([]byte, BTREE_PAGE_SIZE))
	// check for merging
	mergeDir, sibling := shouldMerge(tree, node, idx, updated)

	switch {
	case mergeDir < 0: // left
		merged := BNode(make([]byte, BTREE_PAGE_SIZE))
		nodeMerge(merged, sibling, updated)
		tree.Del(node.GetPtr(idx - 1))
		nodeReplace2Kid(new, node, idx-1, tree.New(merged), merged.GetKey(0))
	case mergeDir > 0: // right
		merged := BNode(make([]byte, BTREE_PAGE_SIZE))
		nodeMerge(merged, updated, sibling)
		tree.Del(node.GetPtr(idx + 1))
		nodeReplace2Kid(new, node, idx, tree.New(merged), merged.GetKey(0))
	case mergeDir == 0:
		NodeReplaceKidN(tree, new, node, idx, updated)
	}
	return new
}

func (tree *BTree) InsertEx(req *InsertReq) {
	return
}
