package db

import (
	"fmt"
	. "types"
	. "utils"
)

// the iterator for range queries
type Scanner struct {
	// the range, from Key1 to Key2
	Cmp1 int // CMP_?
	Cmp2 int
	Key1 Record
	Key2 Record
	// internal
	tdef   *TableDef
	iter   *BIter // the underlying B-tree iterator
	keyEnd []byte // the encoded Key2
}

// within the range or not?
func (sc *Scanner) Valid() bool {
	if !sc.iter.Valid() {
		return false
	}
	key, _ := sc.iter.Deref()
	return CmpOK(key, sc.Cmp2, sc.keyEnd)
}

// move the underlying B-tree iterator
func (sc *Scanner) Next() {
	Assert(sc.Valid())
	if sc.Cmp1 > 0 {
		sc.iter.Next()
	} else {
		sc.iter.Prev()
	}
}

// fetch the current row
func (sc *Scanner) Deref(rec *Record) {
	values, err := checkRecord(sc.tdef, *rec, sc.tdef.PKeys)
	if err != nil {
		return
	}
	// key := encodeKey(nil, sc.tdef.Prefix, values[:sc.tdef.PKeys])
	// fmt.Println("prefix", sc.tdef.Prefix, "values:", values[:sc.tdef.PKeys], "record", rec)
	// fmt.Printf("serach for key: %s\n", key)
	_, val := sc.iter.Deref()

	for i := sc.tdef.PKeys; i < len(sc.tdef.Cols); i++ {
		values[i].Type = sc.tdef.Types[i]
	}
	decodeValues(val, values[sc.tdef.PKeys:])
	rec.Cols = append(rec.Cols, sc.tdef.Cols[sc.tdef.PKeys:]...)
	rec.Vals = append(rec.Vals, values[sc.tdef.PKeys:]...)
}
func (db *DB) Scan(table string, req *Scanner) error {
	tdef := getTableDef(db, table)
	if tdef == nil {
		return fmt.Errorf("table not found: %s", table)
	}
	return dbScan(db, tdef, req)
}
func dbScan(db *DB, tdef *TableDef, req *Scanner) error {
	// sanity checks
	switch {
	case req.Cmp1 > 0 && req.Cmp2 < 0:
	case req.Cmp2 > 0 && req.Cmp1 < 0:
	default:
		return fmt.Errorf("bad range")
	}
	values1, err := checkRecord(tdef, req.Key1, tdef.PKeys)
	if err != nil {
		return err
	}
	values2, err := checkRecord(tdef, req.Key2, tdef.PKeys)
	if err != nil {
		return err
	}
	req.tdef = tdef
	// seek to the start key
	keyStart := encodeKey(nil, tdef.Prefix, values1[:tdef.PKeys])
	req.keyEnd = encodeKey(nil, tdef.Prefix, values2[:tdef.PKeys])
	req.iter = db.kv.GetTree().Seek(keyStart, req.Cmp1)
	return nil
}
