package types

// B-tree iterator The BIter type allows us to traverse
// a B-tree iteratively.
type BIter struct {
	tree *BTree
	path []BNode  // from root to leaf
	pos  []uint16 // indexes into nodes
}

// precondition of the Deref()
func (iter *BIter) Valid() bool {
	return iter.tree.Root != 0 && len(iter.path) > 0
}
func (iter *BIter) Init() {
	checkAssertion(iter.tree.Root != 0)
	ptr := iter.tree.Root
	iter.path = make([]BNode, 0)
	iter.pos = make([]uint16, 0)
	for ptr != 0 {
		node := iter.tree.Get(ptr)
		iter.path = append(iter.path, node)
		iter.pos = append(iter.pos, 0)
		ptr = node.GetPtr(0)
	}
}
func (iter *BIter) HasNext() bool {
	return true
}

// moving backward and forward
func (iter *BIter) Next() {
	iterNext(iter, len(iter.path)-1)
}
func iterNext(iter *BIter, level int) {
	if iter.pos[level] < iter.path[level].Nkeys()-1 {
		iter.pos[level]++ // move within this node
	} else if level > 0 {
		iterNext(iter, level-1) // move to a slibing node
	} else {
		return // dummy key
	}
	if level+1 < len(iter.pos) {
		// update the kid node
		node := iter.path[level]
		kid := iter.tree.Get(node.GetPtr(iter.pos[level]))
		iter.path[level+1] = kid
		iter.pos[level+1] = 0
	}
}

// Moving the iterator is simply moving the positions or nodes to a sibling
func (iter *BIter) Prev() {
	iterPrev(iter, len(iter.path)-1)
}
func iterPrev(iter *BIter, level int) {
	if iter.pos[level] > 0 {
		iter.pos[level]-- // move within this node
	} else if level > 0 {
		iterPrev(iter, level-1) // move to a slibing node
	} else {
		return // dummy key
	}
	if level+1 < len(iter.pos) {
		// update the kid node
		node := iter.path[level]
		kid := iter.tree.Get(node.GetPtr(iter.pos[level]))
		iter.path[level+1] = kid
		iter.pos[level+1] = kid.Nkeys() - 1
	}
}

// get the current KV pair
func (iter *BIter) Deref() ([]byte, []byte) {
	if iter.Valid() {
		level := len(iter.path) - 1
		node := iter.path[level]
		key := node.GetKey(iter.pos[level])
		val := node.GetVal(iter.pos[level])
		return key, val
	}
	return nil, nil

}

// find the closest position that is less or equal to the input key
func (tree *BTree) SeekLE(key []byte) *BIter {
	iter := &BIter{tree: tree}
	for ptr := tree.Root; ptr != 0; {
		node := tree.Get(ptr)
		idx := nodeLookupLE(node, key)
		iter.path = append(iter.path, node)
		iter.pos = append(iter.pos, idx)
		if node.Ntype() == BNODE_NODE {
			ptr = node.GetPtr(idx)
		} else {
			ptr = 0
		}
	}
	return iter
}
