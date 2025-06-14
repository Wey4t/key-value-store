package types

const (
	MODE_UPSERT      = 0 // insert or replace
	MODE_UPDATE_ONLY = 1 // update existing keys
	MODE_INSERT_ONLY = 2 // only add new keys
)

type InsertReq struct {
	tree *BTree
	// out
	Added bool // added a new key
	// in
	Key  []byte
	Val  []byte
	Mode int
}
type DeleteReq struct {
	tree *BTree
	// in
	Key []byte
	// out
	Old []byte
}
