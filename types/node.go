package types

type Node struct {
	keys [][]byte
	// one of the following
	vals [][]byte // for leaf nodes only
	kids []*Node  // for internal nodes only
}

func Encode(node *Node) []byte {
	return []byte{} // Placeholder for encoding logic
}
func Decode(page []byte) (*Node, error) {
	return nil, nil // Placeholder for decoding logic
}
