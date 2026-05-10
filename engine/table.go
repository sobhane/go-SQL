package engine

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"strings"

	"github.com/sobhane/golang-database/storage"
)

// Table — Row-Level Operations
//
// This file handles the conversion between Go values and the binary
// format stored in the B-Tree. Each row is stored as:
//   Key   = primary key value (serialized)
//   Value = all column values (serialized using gob encoding)
//
// WHY SERIALIZATION?
// The B-Tree stores raw bytes. We need to convert Go types (int64, string, bool)
// to bytes for storage and back again for retrieval. We use Go's "encoding/gob"
// for simplicity — real databases use custom binary encodings for performance.

// Row represents a single row of data as a map of column names to values.
type Row map[string]interface{}

// SerializeRow converts a Row into bytes for B-Tree storage.
func SerializeRow(row Row) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(row); err != nil {
		return nil, fmt.Errorf("failed to serialize row: %w", err)
	}
	return buf.Bytes(), nil
}

// DeserializeRow converts bytes from B-Tree storage back into a Row.
func DeserializeRow(data []byte) (Row, error) {
	var row Row
	buf := bytes.NewReader(data)
	dec := gob.NewDecoder(buf)
	if err := dec.Decode(&row); err != nil {
		return nil, fmt.Errorf("failed to deserialize row: %w", err)
	}
	return row, nil
}

// SerializeKey converts a primary key value into bytes for B-Tree key storage.
func SerializeKey(value interface{}) []byte {
	switch v := value.(type) {
	case int64:
		// Fixed-width 8-byte big-endian encoding for proper sorting
		buf := make([]byte, 8)
		// Shift to unsigned for correct byte ordering (handles negatives)
		uv := uint64(v) ^ (1 << 63)
		buf[0] = byte(uv >> 56)
		buf[1] = byte(uv >> 48)
		buf[2] = byte(uv >> 40)
		buf[3] = byte(uv >> 32)
		buf[4] = byte(uv >> 24)
		buf[5] = byte(uv >> 16)
		buf[6] = byte(uv >> 8)
		buf[7] = byte(uv)
		return buf
	case string:
		return []byte(v)
	case bool:
		if v {
			return []byte{1}
		}
		return []byte{0}
	default:
		return []byte(fmt.Sprintf("%v", v))
	}
}

// InsertRow inserts a row into a table's B-Tree.
func InsertRow(pager *storage.Pager, table *TableInfo, row Row) error {
	// Get or generate primary key
	var pkValue interface{}
	if table.PrimaryKey != "" {
		pkValue = row[table.PrimaryKey]
		if pkValue == nil {
			return fmt.Errorf("missing primary key '%s'", table.PrimaryKey)
		}
	} else {
		// Auto-increment row ID
		pkValue = table.NextRowID
		row["_rowid"] = table.NextRowID
		table.NextRowID++
	}

	key := SerializeKey(pkValue)
	value, err := SerializeRow(row)
	if err != nil {
		return err
	}

	// Check for duplicate primary key
	btree := storage.LoadBTree(pager, table.RootPage)
	existing, err := btree.Search(key)
	if err != nil {
		return err
	}
	if existing != nil {
		return fmt.Errorf("duplicate primary key: %v", pkValue)
	}

	// Insert into B-Tree
	if err := btree.Insert(key, value); err != nil {
		return err
	}

	// Update root page in case of split
	table.RootPage = btree.RootPage()

	return nil
}

// ScanAllRows returns all rows from a table.
func ScanAllRows(pager *storage.Pager, table *TableInfo) ([]Row, error) {
	btree := storage.LoadBTree(pager, table.RootPage)
	kvPairs, err := btree.Scan()
	if err != nil {
		return nil, err
	}

	rows := make([]Row, 0, len(kvPairs))
	for _, kv := range kvPairs {
		row, err := DeserializeRow(kv.Value)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}

	return rows, nil
}

// DeleteRow deletes a row by its primary key value.
func DeleteRow(pager *storage.Pager, table *TableInfo, pkValue interface{}) (bool, error) {
	key := SerializeKey(pkValue)
	btree := storage.LoadBTree(pager, table.RootPage)
	return btree.Delete(key)
}

// UpdateRow updates a row identified by its primary key.
func UpdateRow(pager *storage.Pager, table *TableInfo, pkValue interface{}, updates map[string]interface{}) error {
	key := SerializeKey(pkValue)
	btree := storage.LoadBTree(pager, table.RootPage)

	// Find existing row
	data, err := btree.Search(key)
	if err != nil {
		return err
	}
	if data == nil {
		return fmt.Errorf("row with key %v not found", pkValue)
	}

	// Deserialize, update, re-serialize
	row, err := DeserializeRow(data)
	if err != nil {
		return err
	}

	for col, val := range updates {
		row[col] = val
	}

	newValue, err := SerializeRow(row)
	if err != nil {
		return err
	}

	return btree.Insert(key, newValue)
}

// FormatTable formats rows as a pretty ASCII table for display.
func FormatTable(columns []string, rows []Row) string {
	if len(rows) == 0 {
		return "(0 rows)"
	}

	// Calculate column widths
	widths := make([]int, len(columns))
	for i, col := range columns {
		widths[i] = len(col)
	}
	for _, row := range rows {
		for i, col := range columns {
			val := fmt.Sprintf("%v", row[col])
			if row[col] == nil {
				val = "NULL"
			}
			if len(val) > widths[i] {
				widths[i] = len(val)
			}
		}
	}

	var sb strings.Builder

	// Header separator
	writeSep := func() {
		sb.WriteByte('+')
		for _, w := range widths {
			sb.WriteString(strings.Repeat("-", w+2))
			sb.WriteByte('+')
		}
		sb.WriteByte('\n')
	}

	// Header
	writeSep()
	sb.WriteByte('|')
	for i, col := range columns {
		sb.WriteString(fmt.Sprintf(" %-*s |", widths[i], col))
	}
	sb.WriteByte('\n')
	writeSep()

	// Data rows
	for _, row := range rows {
		sb.WriteByte('|')
		for i, col := range columns {
			val := fmt.Sprintf("%v", row[col])
			if row[col] == nil {
				val = "NULL"
			}
			sb.WriteString(fmt.Sprintf(" %-*s |", widths[i], val))
		}
		sb.WriteByte('\n')
	}
	writeSep()

	sb.WriteString(fmt.Sprintf("(%d rows)\n", len(rows)))
	return sb.String()
}
