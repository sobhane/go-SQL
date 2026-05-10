package engine

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"sync"

	"github.com/sobhane/golang-database/storage"
)

// Catalog — The System Table Registry
//
// WHAT IS A CATALOG?
// Every real database has a "catalog" (also called "system tables" or
// "information_schema"). It's a registry that stores metadata about
// all user tables: their names, columns, column types, primary keys, etc.
//
// When you run CREATE TABLE, the table definition goes into the catalog.
// When you run SELECT, the executor checks the catalog to know which
// columns the table has and what types they expect.
//
// OUR IMPLEMENTATION:
// We store the catalog as a serialized Go struct on a dedicated page
// in the database file (page 1 — right after the file header on page 0).
// When the database opens, we deserialize it into memory.
// When tables are created/dropped, we update memory and re-serialize to disk.

// ColumnInfo stores metadata about a single column.
type ColumnInfo struct {
	Name       string
	DataType   string // "INT", "TEXT", "BOOL"
	PrimaryKey bool
	NotNull    bool
}

// TableInfo stores metadata about a table.
type TableInfo struct {
	Name         string
	Columns      []ColumnInfo
	PrimaryKey   string // name of the primary key column
	RootPage     uint32 // which B-Tree page stores this table's data
	NextRowID    int64  // auto-increment counter for implicit row IDs
}

// Catalog is the in-memory registry of all tables.
type Catalog struct {
	Tables map[string]*TableInfo
	pager  *storage.Pager
	mu     sync.RWMutex
}

// NewCatalog creates or loads a catalog from the database file.
// The catalog is stored on page 1.
func NewCatalog(pager *storage.Pager) (*Catalog, error) {
	cat := &Catalog{
		Tables: make(map[string]*TableInfo),
		pager:  pager,
	}

	// If the database only has page 0 (header), we need to allocate page 1 for the catalog
	if pager.NumPages() <= 1 {
		_, err := pager.AllocatePage() // allocate page 1
		if err != nil {
			return nil, fmt.Errorf("failed to allocate catalog page: %w", err)
		}
		// Write empty catalog
		if err := cat.Save(); err != nil {
			return nil, err
		}
	} else {
		// Load existing catalog from page 1
		if err := cat.Load(); err != nil {
			return nil, err
		}
	}

	return cat, nil
}

// Save serializes the catalog to page 1.
// Format: [dataLen:4 bytes][gob data:dataLen bytes][padding:zeros]
func (c *Catalog) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(c.Tables); err != nil {
		return fmt.Errorf("failed to encode catalog: %w", err)
	}

	data := buf.Bytes()
	if len(data)+4 > storage.PageSize {
		return fmt.Errorf("catalog too large for single page (%d bytes)", len(data))
	}

	page := make([]byte, storage.PageSize)
	// Write the data length first (4 bytes), then the actual gob data
	binary.LittleEndian.PutUint32(page[0:4], uint32(len(data)))
	copy(page[4:], data)
	return c.pager.WritePage(1, page)
}

// Load deserializes the catalog from page 1.
// Format: [dataLen:4 bytes][gob data:dataLen bytes][padding:zeros]
func (c *Catalog) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	page, err := c.pager.ReadPage(1)
	if err != nil {
		return fmt.Errorf("failed to read catalog page: %w", err)
	}

	// Read the data length from the first 4 bytes
	dataLen := binary.LittleEndian.Uint32(page[0:4])

	if dataLen == 0 {
		// Empty catalog
		c.Tables = make(map[string]*TableInfo)
		return nil
	}

	if dataLen > uint32(storage.PageSize-4) {
		return fmt.Errorf("catalog data length invalid: %d", dataLen)
	}

	buf := bytes.NewReader(page[4 : 4+dataLen])
	dec := gob.NewDecoder(buf)
	tables := make(map[string]*TableInfo)
	if err := dec.Decode(&tables); err != nil {
		return fmt.Errorf("failed to decode catalog: %w", err)
	}
	c.Tables = tables

	return nil
}

// CreateTable registers a new table in the catalog and allocates a B-Tree for it.
func (c *Catalog) CreateTable(name string, columns []ColumnInfo, primaryKey string) (*TableInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.Tables[name]; exists {
		return nil, fmt.Errorf("table '%s' already exists", name)
	}

	// Create a new B-Tree for this table's data
	btree, err := storage.NewBTree(c.pager)
	if err != nil {
		return nil, fmt.Errorf("failed to create B-Tree for table '%s': %w", name, err)
	}

	table := &TableInfo{
		Name:       name,
		Columns:    columns,
		PrimaryKey: primaryKey,
		RootPage:   btree.RootPage(),
		NextRowID:  1,
	}

	c.Tables[name] = table

	// Persist catalog
	c.mu.Unlock()
	err = c.Save()
	c.mu.Lock()

	return table, err
}

// DropTable removes a table from the catalog.
func (c *Catalog) DropTable(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.Tables[name]; !exists {
		return fmt.Errorf("table '%s' does not exist", name)
	}

	delete(c.Tables, name)

	c.mu.Unlock()
	err := c.Save()
	c.mu.Lock()

	return err
}

// GetTable returns the metadata for a table, or an error if not found.
func (c *Catalog) GetTable(name string) (*TableInfo, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	table, exists := c.Tables[name]
	if !exists {
		return nil, fmt.Errorf("table '%s' does not exist", name)
	}
	return table, nil
}

// GetColumnIndex returns the index of a column by name, or -1 if not found.
func (t *TableInfo) GetColumnIndex(name string) int {
	for i, col := range t.Columns {
		if col.Name == name {
			return i
		}
	}
	return -1
}
