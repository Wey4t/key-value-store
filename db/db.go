package db

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	. "types"
	. "utils"
)

const TABLE_PREFIX_MIN = 1
const (
	TYPE_ERROR uint32 = iota
	TYPE_BYTES
	TYPE_INT64
)

// table cell
type Value struct {
	Type uint32
	I64  int64
	Str  []byte
}
type Record struct {
	Cols []string
	Vals []Value
}

func (rec *Record) AddStr(key string, val []byte) *Record {
	rec.Cols = append(rec.Cols, key)
	rec.Vals = append(rec.Vals, Value{Type: TYPE_BYTES, Str: val})
	return rec
}
func (rec *Record) AddInt64(key string, val int64) *Record {
	rec.Cols = append(rec.Cols, key)
	rec.Vals = append(rec.Vals, Value{Type: TYPE_INT64, I64: val})
	return rec
}
func (rec *Record) Get(key string) *Value {
	for i, col := range rec.Cols {
		if col == key {
			return &rec.Vals[i]
		}
	}
	return &Value{Type: TYPE_ERROR} // not found
}

type DB struct {
	Path string
	// internals
	kv     *KV
	tables map[string]*TableDef // cached table definition
}

// table definition
type TableDef struct {
	// user defined
	Name    string
	Types   []uint32 // column types
	Cols    []string // column names
	PKeys   int      // the first PKeys columns are the primary key
	Indexes [][]string
	// auto-assigned B-tree key prefixes for different tables/indexes
	Prefix        uint32
	IndexPrefixes []uint32
}

// internal table: metadata
var TDEF_META = &TableDef{
	Prefix: 1,
	Name:   "@meta",
	Types:  []uint32{TYPE_BYTES, TYPE_BYTES},
	Cols:   []string{"key", "val"},
	PKeys:  1,
}

// internal table: table schemas
var TDEF_TABLE = &TableDef{
	Prefix: 2,
	Name:   "@table",
	Types:  []uint32{TYPE_BYTES, TYPE_BYTES},
	Cols:   []string{"name", "def"},
	PKeys:  1,
}

// get a single row by the primary key
// func dbGet(db *DB, tdef *TableDef, rec *Record) (bool, error) {

//		values, err := checkRecord(tdef, *rec, tdef.PKeys)
//		if err != nil {
//			return false, err
//		}
//		key := encodeKey(nil, tdef.Prefix, values[:tdef.PKeys])
//		// fmt.Println("prefix", tdef.Prefix, "values:", values[:tdef.PKeys], "record", rec)
//		// fmt.Printf("serach for key: %s\n", key)
//		val, ok := db.kv.Get(key)
//		if !ok {
//			return false, nil
//		}
//		for i := tdef.PKeys; i < len(tdef.Cols); i++ {
//			values[i].Type = tdef.Types[i]
//		}
//		decodeValues(val, values[tdef.PKeys:])
//		rec.Cols = append(rec.Cols, tdef.Cols[tdef.PKeys:]...)
//		rec.Vals = append(rec.Vals, values[tdef.PKeys:]...)
//		return true, nil
//	}

func (db *DB) Open() {
	db.kv = &KV{Path: db.Path}
	db.kv.Open()
}
func (db *DB) Close() {
	db.kv.Path = db.Path
	db.kv.Close()
}

func dbGet(db *DB, tdef *TableDef, rec *Record) (bool, error) {
	sc := Scanner{
		Cmp1: CMP_GE,
		Cmp2: CMP_LE,
		Key1: *rec,
		Key2: *rec,
	}
	if err := dbScan(db, tdef, &sc); err != nil {
		return false, err
	}
	if sc.Valid() {
		sc.Deref(rec)
		return true, nil
	} else {
		return false, nil
	}
}

// reorder a record and check for missing columns.
// n == tdef.PKeys: record is exactly a primary key
// n == len(tdef.Cols): record contains all columns
func checkRecord(tdef *TableDef, rec Record, n int) ([]Value, error) {
	// omitted...
	if n < tdef.PKeys || n > len(tdef.Cols) {
		return nil, errors.New("invalid record length")
	}

	if n == tdef.PKeys {
		// primary key only
		values := make([]Value, len(tdef.Cols))
		for i := 0; i < tdef.PKeys; i++ {
			values[i] = *rec.Get(tdef.Cols[i])
			if values[i].Type != tdef.Types[i] {
				return nil, errors.New("invalid type for primary key")
			}
		}
		return values, nil
	}
	if n == len(tdef.Cols) {
		values := make([]Value, len(tdef.Cols))
		for i := 0; i < n; i++ {
			values[i] = *rec.Get(tdef.Cols[i])
			if values[i].Type != tdef.Types[i] {
				return nil, errors.New("invalid type for primary key")
			}
		}
		return values, nil
	}

	return nil, errors.New("record must contain primary key columns only")
}

//	func encodeValues(out []byte, vals []Value) []byte {
//		for _, val := range vals {
//			encodeVal, _ := json.Marshal(val)
//			uint32Len := uint32(len(encodeVal))
//			var buf [4]byte
//			binary.BigEndian.PutUint32(buf[:], uint32Len)
//			out = append(out, buf[:]...) // write the length of the value
//			out = append(out, encodeVal...)
//		}
//		return out // omitted: encode each Value to the output slice
//	}
func encodeValues(out []byte, vals []Value) []byte {
	for _, v := range vals {
		switch v.Type {
		case TYPE_INT64:
			var buf [8]byte
			u := uint64(v.I64) + (1 << 63)
			binary.BigEndian.PutUint64(buf[:], u)
			out = append(out, buf[:]...)
		case TYPE_BYTES:
			out = append(out, escapeString(v.Str)...)
			out = append(out, 0) // null-terminated
		default:
			panic("what?")
		}
	}
	return out
}

// Strings are encoded as nul terminated strings,
// escape the nul byte so that strings contain no null byte.
func escapeString(in []byte) []byte {
	zeros := bytes.Count(in, []byte{0})
	ones := bytes.Count(in, []byte{1})
	if zeros+ones == 0 {
		return in
	}
	out := make([]byte, len(in)+zeros+ones)
	pos := 0
	for _, ch := range in {
		if ch <= 1 {
			out[pos+0] = 0x01
			out[pos+1] = ch + 1
			pos += 2
		} else {
			out[pos] = ch
			pos += 1
		}
	}
	return out
}

// expect in is null end
func unEscapeString(in []byte) []byte {
	if len(in) == 0 {
		return nil
	}

	out := make([]byte, len(in))
	count := 0
	for i := 0; i < len(in); {
		ch := in[i]
		if ch == 1 {
			Assert(i+1 < len(in))
			if in[i+1] == 1 {
				out[count] = 0
			} else {
				out[count] = 1
			}
			i += 2
		} else {
			out[count] = ch
			i += 1
		}
		count += 1
	}
	return out[:count]

}

func decodeValues(in []byte, out []Value) {
	pos := 0
	for i, val := range out {
		switch val.Type {
		case TYPE_INT64:
			// Need at least 8 bytes for int64
			Assert(pos+8 <= len(in))

			// Read 8 bytes and decode int64
			u := binary.BigEndian.Uint64(in[pos : pos+8])
			i64 := int64(u - (1 << 63)) // Reverse the bias encoding
			out[i].I64 = i64
			pos += 8

		case TYPE_BYTES:
			// Find the null terminator
			nullPos := -1
			for i := pos; i < len(in); i++ {
				if in[i] == 0 {
					nullPos = i
					break
				}
			}
			Assert(nullPos != -1)
			// Extract the escaped string (without null terminator)
			escapedStr := in[pos:nullPos]

			// Unescape the string
			unescapedStr := unEscapeString(escapedStr)

			out[i].Str = unescapedStr
			pos = nullPos + 1 // Skip past the null terminator
		default:
			panic("bad type")
		}
	}
}

// for primary keys
func encodeKey(out []byte, prefix uint32, vals []Value) []byte {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], prefix)
	out = append(out, buf[:]...)
	out = encodeValues(out, vals)
	return out
}
func (db *DB) Get(table string, rec *Record) (bool, error) {
	tdef := getTableDef(db, table)
	if tdef == nil {
		return false, fmt.Errorf("table not found: %s", table)
	}
	return dbGet(db, tdef, rec)
}

// get the table definition by name
func getTableDef(db *DB, name string) *TableDef {
	tdef, ok := db.tables[name]
	if !ok {
		if db.tables == nil {
			db.tables = map[string]*TableDef{}
		}
		tdef = getTableDefDB(db, name)
		if tdef != nil {
			db.tables[name] = tdef
		}
	}
	return tdef
}
func getTableDefDB(db *DB, name string) *TableDef {
	rec := (&Record{}).AddStr("name", []byte(name))
	// fmt.Println("get the table def from intenal")
	ok, err := dbGet(db, TDEF_TABLE, rec)
	Assert(err == nil)
	if !ok {
		return nil
	}
	tdef := &TableDef{}
	err = json.Unmarshal(rec.Get("def").Str, tdef)
	Assert(err == nil)
	return tdef
}

func checkIndexKeys(tdef *TableDef, index []string) ([]string, error) {
	icols := map[string]bool{}
	for _, c := range index {
		// check the index columns
		// omitted...
		icols[c] = true
	}
	// add the primary key to the index
	for _, c := range tdef.Cols[:tdef.PKeys] {
		if !icols[c] {
			index = append(index, c)
		}
	}
	Assert(len(index) < len(tdef.Cols))
	return index, nil
}
func colIndex(tdef *TableDef, col string) int {
	for i, c := range tdef.Cols {
		if c == col {
			return i
		}
	}
	return -1
}
func tableDefCheck(tdef *TableDef) error {
	// verify the table definition
	// omitted...
	// verify the indexes
	for i, index := range tdef.Indexes {
		index, err := checkIndexKeys(tdef, index)
		if err != nil {
			return err
		}
		tdef.Indexes[i] = index
	}
	return nil
}
