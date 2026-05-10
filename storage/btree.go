package storage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
)

// B-TREE: The Data Structure That Powers Databases
//
// WHY B-TREE?
// Nearly every relational database uses B-Trees (or B+Trees) for storage.
// A B-Tree keeps data sorted and allows searches, insertions, and deletions
// in O(log n) time. Unlike a binary tree (2 children per node), a B-Tree
// has many children per node, which minimizes disk reads.
//
// KEY CONCEPTS:
// - Each node is stored in one page (4KB).
// - Leaf nodes contain actual data (key-value pairs = rows).
// - Internal nodes contain keys and pointers (page numbers) to children.
// - When a leaf gets full, it "splits" into two leaves, pushing a key up.
//
// OUR IMPLEMENTATION:
// We use a B+Tree variant where all data lives in leaf nodes.
// Internal nodes only store keys and child pointers for navigation.

// Page layout constants
const (
	// Page header (first 8 bytes of every B-Tree page):
	// [0]     = node type: 1=leaf, 2=internal
	// [1..4]  = number of cells (uint32, but we use uint16 for space)
	// [5..8]  = right child pointer (internal) or next leaf pointer (leaf)
	nodeTypeOffset    = 0
	numCellsOffset    = 1
	rightPointerOffset = 5
	headerSize        = 9

	// Node types
	leafNode     byte = 1
	internalNode byte = 2

	// Leaf cell layout: [keyLen:2][key:keyLen][valLen:4][val:valLen]
	// Internal cell layout: [childPageNum:4][keyLen:2][key:keyLen]

	// Maximum order (children per internal node) — tuned for 4KB pages
	maxLeafCells     = 30  // max key-value pairs in a leaf
	maxInternalCells = 100 // max keys in an internal node
)

// BTree represents a B+Tree rooted at a specific page.
type BTree struct {
	pager    *Pager
	rootPage uint32
}

// NewBTree creates a new B-Tree with an empty leaf node as the root.
func NewBTree(pager *Pager) (*BTree, error) {
	rootPageNum, err := pager.AllocatePage()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate root page: %w", err)
	}

	// Initialize root as empty leaf
	page := make([]byte, PageSize)
	page[nodeTypeOffset] = leafNode
	binary.LittleEndian.PutUint32(page[numCellsOffset:numCellsOffset+4], 0)
	binary.LittleEndian.PutUint32(page[rightPointerOffset:rightPointerOffset+4], 0) // no next leaf

	if err := pager.WritePage(rootPageNum, page); err != nil {
		return nil, err
	}

	return &BTree{pager: pager, rootPage: rootPageNum}, nil
}

// LoadBTree loads an existing B-Tree from a known root page number.
func LoadBTree(pager *Pager, rootPage uint32) *BTree {
	return &BTree{pager: pager, rootPage: rootPage}
}

// RootPage returns the root page number of this B-Tree.
func (bt *BTree) RootPage() uint32 {
	return bt.rootPage
}

// ---------------------------------------------------------------------------
// SEARCH — O(log n) lookup by key
// ---------------------------------------------------------------------------

// Search finds the value associated with a key, or returns nil if not found.
func (bt *BTree) Search(key []byte) ([]byte, error) {
	return bt.searchNode(bt.rootPage, key)
}

func (bt *BTree) searchNode(pageNum uint32, key []byte) ([]byte, error) {
	page, err := bt.pager.ReadPage(pageNum)
	if err != nil {
		return nil, err
	}

	nodeType := page[nodeTypeOffset]

	if nodeType == leafNode {
		return bt.searchLeaf(page, key)
	}
	return bt.searchInternal(page, key)
}

func (bt *BTree) searchLeaf(page []byte, searchKey []byte) ([]byte, error) {
	numCells := binary.LittleEndian.Uint32(page[numCellsOffset : numCellsOffset+4])
	offset := headerSize

	for i := uint32(0); i < numCells; i++ {
		keyLen := binary.LittleEndian.Uint16(page[offset : offset+2])
		offset += 2
		key := page[offset : offset+int(keyLen)]
		offset += int(keyLen)
		valLen := binary.LittleEndian.Uint32(page[offset : offset+4])
		offset += 4
		val := page[offset : offset+int(valLen)]
		offset += int(valLen)

		if bytes.Equal(key, searchKey) {
			result := make([]byte, len(val))
			copy(result, val)
			return result, nil
		}
	}

	return nil, nil // not found
}

func (bt *BTree) searchInternal(page []byte, key []byte) ([]byte, error) {
	numCells := binary.LittleEndian.Uint32(page[numCellsOffset : numCellsOffset+4])
	rightChild := binary.LittleEndian.Uint32(page[rightPointerOffset : rightPointerOffset+4])

	offset := headerSize
	for i := uint32(0); i < numCells; i++ {
		childPage := binary.LittleEndian.Uint32(page[offset : offset+4])
		offset += 4
		keyLen := binary.LittleEndian.Uint16(page[offset : offset+2])
		offset += 2
		cellKey := page[offset : offset+int(keyLen)]
		offset += int(keyLen)

		if bytes.Compare(key, cellKey) < 0 {
			return bt.searchNode(childPage, key)
		}
	}

	// Key is >= all keys, go to right child
	return bt.searchNode(rightChild, key)
}

// ---------------------------------------------------------------------------
// INSERT — add a key-value pair, splitting nodes when full
// ---------------------------------------------------------------------------

// Insert adds a key-value pair to the B-Tree.
// If the key already exists, the value is updated.
func (bt *BTree) Insert(key, value []byte) error {
	// Try inserting into the tree
	split, err := bt.insertNode(bt.rootPage, key, value)
	if err != nil {
		return err
	}

	// If the root split, create a new root with two children
	if split != nil {
		newRootPage, err := bt.pager.AllocatePage()
		if err != nil {
			return err
		}

		page := make([]byte, PageSize)
		page[nodeTypeOffset] = internalNode
		binary.LittleEndian.PutUint32(page[numCellsOffset:numCellsOffset+4], 1)
		binary.LittleEndian.PutUint32(page[rightPointerOffset:rightPointerOffset+4], split.rightPage)

		// Write single cell: [leftChild:4][keyLen:2][key:keyLen]
		offset := headerSize
		binary.LittleEndian.PutUint32(page[offset:offset+4], split.leftPage)
		offset += 4
		binary.LittleEndian.PutUint16(page[offset:offset+2], uint16(len(split.key)))
		offset += 2
		copy(page[offset:], split.key)

		if err := bt.pager.WritePage(newRootPage, page); err != nil {
			return err
		}

		bt.rootPage = newRootPage
	}

	return nil
}

// splitResult is returned when a node splits during insertion.
type splitResult struct {
	key       []byte // the key that gets promoted to the parent
	leftPage  uint32 // left child after split
	rightPage uint32 // right child after split (the new page)
}

func (bt *BTree) insertNode(pageNum uint32, key, value []byte) (*splitResult, error) {
	page, err := bt.pager.ReadPage(pageNum)
	if err != nil {
		return nil, err
	}

	nodeType := page[nodeTypeOffset]

	if nodeType == leafNode {
		return bt.insertLeaf(pageNum, page, key, value)
	}
	return bt.insertInternal(pageNum, page, key, value)
}

func (bt *BTree) insertLeaf(pageNum uint32, page []byte, key, value []byte) (*splitResult, error) {
	// Read all existing cells
	cells := bt.readLeafCells(page)

	// Check for existing key (update case)
	for i, cell := range cells {
		if bytes.Equal(cell.key, key) {
			cells[i].value = value
			return bt.writeLeafCells(pageNum, cells)
		}
	}

	// Insert new cell in sorted position
	newCell := leafCell{key: key, value: value}
	idx := sort.Search(len(cells), func(i int) bool {
		return bytes.Compare(cells[i].key, key) >= 0
	})
	cells = append(cells, leafCell{})
	copy(cells[idx+1:], cells[idx:])
	cells[idx] = newCell

	return bt.writeLeafCells(pageNum, cells)
}

func (bt *BTree) insertInternal(pageNum uint32, page []byte, key, value []byte) (*splitResult, error) {
	numCells := binary.LittleEndian.Uint32(page[numCellsOffset : numCellsOffset+4])
	rightChild := binary.LittleEndian.Uint32(page[rightPointerOffset : rightPointerOffset+4])

	// Find which child to descend into
	offset := headerSize
	targetChild := rightChild

	for i := uint32(0); i < numCells; i++ {
		childPage := binary.LittleEndian.Uint32(page[offset : offset+4])
		offset += 4
		keyLen := binary.LittleEndian.Uint16(page[offset : offset+2])
		offset += 2
		cellKey := page[offset : offset+int(keyLen)]
		offset += int(keyLen)

		if bytes.Compare(key, cellKey) < 0 {
			targetChild = childPage
			break
		}
	}

	// Recurse into child
	split, err := bt.insertNode(targetChild, key, value)
	if err != nil {
		return nil, err
	}

	if split == nil {
		return nil, nil // no split below, nothing to do
	}

	// Child split — we need to insert the promoted key into this internal node
	cells := bt.readInternalCells(page)
	newCell := internalCell{childPage: split.leftPage, key: split.key}

	idx := sort.Search(len(cells), func(i int) bool {
		return bytes.Compare(cells[i].key, split.key) >= 0
	})
	cells = append(cells, internalCell{})
	copy(cells[idx+1:], cells[idx:])
	cells[idx] = newCell

	// The right pointer of the cell at idx should now point to split.rightPage
	// We need to adjust: cells after the new one keep their children,
	// but the new cell's right neighbor (or rightChild) should be split.rightPage
	newRightChild := rightChild
	if idx < len(cells)-1 {
		// The cell just after the inserted key points to the right page
		cells[idx+1].childPage = split.rightPage
	} else {
		newRightChild = split.rightPage
	}

	return bt.writeInternalCells(pageNum, cells, newRightChild)
}

// ---------------------------------------------------------------------------
// SCAN — iterate through all key-value pairs (full table scan)
// ---------------------------------------------------------------------------

// Scan returns all key-value pairs in sorted order.
func (bt *BTree) Scan() ([]KeyValue, error) {
	var results []KeyValue
	return bt.scanNode(bt.rootPage, results)
}

// KeyValue represents a key-value pair from the B-Tree.
type KeyValue struct {
	Key   []byte
	Value []byte
}

func (bt *BTree) scanNode(pageNum uint32, results []KeyValue) ([]KeyValue, error) {
	page, err := bt.pager.ReadPage(pageNum)
	if err != nil {
		return nil, err
	}

	nodeType := page[nodeTypeOffset]

	if nodeType == leafNode {
		cells := bt.readLeafCells(page)
		for _, cell := range cells {
			results = append(results, KeyValue{Key: cell.key, Value: cell.value})
		}
		return results, nil
	}

	// Internal node: traverse children in order
	numCells := binary.LittleEndian.Uint32(page[numCellsOffset : numCellsOffset+4])
	rightChild := binary.LittleEndian.Uint32(page[rightPointerOffset : rightPointerOffset+4])

	offset := headerSize
	for i := uint32(0); i < numCells; i++ {
		childPage := binary.LittleEndian.Uint32(page[offset : offset+4])
		offset += 4
		keyLen := binary.LittleEndian.Uint16(page[offset : offset+2])
		offset += 2
		offset += int(keyLen) // skip key

		results, err = bt.scanNode(childPage, results)
		if err != nil {
			return nil, err
		}
	}

	// Scan the rightmost child
	results, err = bt.scanNode(rightChild, results)
	if err != nil {
		return nil, err
	}

	return results, nil
}

// ---------------------------------------------------------------------------
// DELETE — remove a key from the B-Tree
// ---------------------------------------------------------------------------

// Delete removes a key-value pair from the B-Tree.
// Returns true if the key was found and deleted, false otherwise.
func (bt *BTree) Delete(key []byte) (bool, error) {
	return bt.deleteFromNode(bt.rootPage, key)
}

func (bt *BTree) deleteFromNode(pageNum uint32, key []byte) (bool, error) {
	page, err := bt.pager.ReadPage(pageNum)
	if err != nil {
		return false, err
	}

	nodeType := page[nodeTypeOffset]

	if nodeType == leafNode {
		cells := bt.readLeafCells(page)
		found := false
		newCells := make([]leafCell, 0, len(cells))
		for _, cell := range cells {
			if bytes.Equal(cell.key, key) {
				found = true
				continue
			}
			newCells = append(newCells, cell)
		}
		if !found {
			return false, nil
		}
		_, err := bt.writeLeafCells(pageNum, newCells)
		return true, err
	}

	// Internal node: find and recurse into the correct child
	numCells := binary.LittleEndian.Uint32(page[numCellsOffset : numCellsOffset+4])
	rightChild := binary.LittleEndian.Uint32(page[rightPointerOffset : rightPointerOffset+4])

	offset := headerSize
	for i := uint32(0); i < numCells; i++ {
		childPage := binary.LittleEndian.Uint32(page[offset : offset+4])
		offset += 4
		keyLen := binary.LittleEndian.Uint16(page[offset : offset+2])
		offset += 2
		cellKey := page[offset : offset+int(keyLen)]
		offset += int(keyLen)

		if bytes.Compare(key, cellKey) < 0 {
			return bt.deleteFromNode(childPage, key)
		}
	}

	return bt.deleteFromNode(rightChild, key)
}

// ---------------------------------------------------------------------------
// Cell reading/writing helpers
// ---------------------------------------------------------------------------

type leafCell struct {
	key   []byte
	value []byte
}

type internalCell struct {
	childPage uint32
	key       []byte
}

func (bt *BTree) readLeafCells(page []byte) []leafCell {
	numCells := binary.LittleEndian.Uint32(page[numCellsOffset : numCellsOffset+4])
	cells := make([]leafCell, 0, numCells)
	offset := headerSize

	for i := uint32(0); i < numCells; i++ {
		keyLen := binary.LittleEndian.Uint16(page[offset : offset+2])
		offset += 2
		key := make([]byte, keyLen)
		copy(key, page[offset:offset+int(keyLen)])
		offset += int(keyLen)
		valLen := binary.LittleEndian.Uint32(page[offset : offset+4])
		offset += 4
		val := make([]byte, valLen)
		copy(val, page[offset:offset+int(valLen)])
		offset += int(valLen)
		cells = append(cells, leafCell{key: key, value: val})
	}

	return cells
}

func (bt *BTree) readInternalCells(page []byte) []internalCell {
	numCells := binary.LittleEndian.Uint32(page[numCellsOffset : numCellsOffset+4])
	cells := make([]internalCell, 0, numCells)
	offset := headerSize

	for i := uint32(0); i < numCells; i++ {
		childPage := binary.LittleEndian.Uint32(page[offset : offset+4])
		offset += 4
		keyLen := binary.LittleEndian.Uint16(page[offset : offset+2])
		offset += 2
		key := make([]byte, keyLen)
		copy(key, page[offset:offset+int(keyLen)])
		offset += int(keyLen)
		cells = append(cells, internalCell{childPage: childPage, key: key})
	}

	return cells
}

// writeLeafCells writes leaf cells to the page. Splits if cells exceed capacity.
func (bt *BTree) writeLeafCells(pageNum uint32, cells []leafCell) (*splitResult, error) {
	if len(cells) <= maxLeafCells {
		// Fits in one page — write directly
		page := make([]byte, PageSize)
		page[nodeTypeOffset] = leafNode
		binary.LittleEndian.PutUint32(page[numCellsOffset:numCellsOffset+4], uint32(len(cells)))

		offset := headerSize
		for _, cell := range cells {
			binary.LittleEndian.PutUint16(page[offset:offset+2], uint16(len(cell.key)))
			offset += 2
			copy(page[offset:], cell.key)
			offset += len(cell.key)
			binary.LittleEndian.PutUint32(page[offset:offset+4], uint32(len(cell.value)))
			offset += 4
			copy(page[offset:], cell.value)
			offset += len(cell.value)
		}

		return nil, bt.pager.WritePage(pageNum, page)
	}

	// SPLIT! This is a core B-Tree operation.
	// We split the cells in half. The left half stays in the current page,
	// the right half goes to a new page. The middle key is promoted up.
	mid := len(cells) / 2
	leftCells := cells[:mid]
	rightCells := cells[mid:]
	promotedKey := make([]byte, len(rightCells[0].key))
	copy(promotedKey, rightCells[0].key)

	// Write left cells to current page
	leftPage := make([]byte, PageSize)
	leftPage[nodeTypeOffset] = leafNode
	binary.LittleEndian.PutUint32(leftPage[numCellsOffset:numCellsOffset+4], uint32(len(leftCells)))
	offset := headerSize
	for _, cell := range leftCells {
		binary.LittleEndian.PutUint16(leftPage[offset:offset+2], uint16(len(cell.key)))
		offset += 2
		copy(leftPage[offset:], cell.key)
		offset += len(cell.key)
		binary.LittleEndian.PutUint32(leftPage[offset:offset+4], uint32(len(cell.value)))
		offset += 4
		copy(leftPage[offset:], cell.value)
		offset += len(cell.value)
	}
	if err := bt.pager.WritePage(pageNum, leftPage); err != nil {
		return nil, err
	}

	// Allocate new page for right cells
	newPageNum, err := bt.pager.AllocatePage()
	if err != nil {
		return nil, err
	}
	rightPage := make([]byte, PageSize)
	rightPage[nodeTypeOffset] = leafNode
	binary.LittleEndian.PutUint32(rightPage[numCellsOffset:numCellsOffset+4], uint32(len(rightCells)))
	offset = headerSize
	for _, cell := range rightCells {
		binary.LittleEndian.PutUint16(rightPage[offset:offset+2], uint16(len(cell.key)))
		offset += 2
		copy(rightPage[offset:], cell.key)
		offset += len(cell.key)
		binary.LittleEndian.PutUint32(rightPage[offset:offset+4], uint32(len(cell.value)))
		offset += 4
		copy(rightPage[offset:], cell.value)
		offset += len(cell.value)
	}
	if err := bt.pager.WritePage(newPageNum, rightPage); err != nil {
		return nil, err
	}

	return &splitResult{
		key:       promotedKey,
		leftPage:  pageNum,
		rightPage: newPageNum,
	}, nil
}

// writeInternalCells writes internal node cells. Splits if they exceed capacity.
func (bt *BTree) writeInternalCells(pageNum uint32, cells []internalCell, rightChild uint32) (*splitResult, error) {
	if len(cells) <= maxInternalCells {
		page := make([]byte, PageSize)
		page[nodeTypeOffset] = internalNode
		binary.LittleEndian.PutUint32(page[numCellsOffset:numCellsOffset+4], uint32(len(cells)))
		binary.LittleEndian.PutUint32(page[rightPointerOffset:rightPointerOffset+4], rightChild)

		offset := headerSize
		for _, cell := range cells {
			binary.LittleEndian.PutUint32(page[offset:offset+4], cell.childPage)
			offset += 4
			binary.LittleEndian.PutUint16(page[offset:offset+2], uint16(len(cell.key)))
			offset += 2
			copy(page[offset:], cell.key)
			offset += len(cell.key)
		}

		return nil, bt.pager.WritePage(pageNum, page)
	}

	// Split internal node
	mid := len(cells) / 2
	leftCells := cells[:mid]
	promotedKey := make([]byte, len(cells[mid].key))
	copy(promotedKey, cells[mid].key)
	rightCells := cells[mid+1:]
	newLeftRightChild := cells[mid].childPage

	// Write left
	leftPage := make([]byte, PageSize)
	leftPage[nodeTypeOffset] = internalNode
	binary.LittleEndian.PutUint32(leftPage[numCellsOffset:numCellsOffset+4], uint32(len(leftCells)))
	binary.LittleEndian.PutUint32(leftPage[rightPointerOffset:rightPointerOffset+4], newLeftRightChild)
	offset := headerSize
	for _, cell := range leftCells {
		binary.LittleEndian.PutUint32(leftPage[offset:offset+4], cell.childPage)
		offset += 4
		binary.LittleEndian.PutUint16(leftPage[offset:offset+2], uint16(len(cell.key)))
		offset += 2
		copy(leftPage[offset:], cell.key)
		offset += len(cell.key)
	}
	if err := bt.pager.WritePage(pageNum, leftPage); err != nil {
		return nil, err
	}

	// Allocate and write right
	newPageNum, err := bt.pager.AllocatePage()
	if err != nil {
		return nil, err
	}
	rightPage := make([]byte, PageSize)
	rightPage[nodeTypeOffset] = internalNode
	binary.LittleEndian.PutUint32(rightPage[numCellsOffset:numCellsOffset+4], uint32(len(rightCells)))
	binary.LittleEndian.PutUint32(rightPage[rightPointerOffset:rightPointerOffset+4], rightChild)
	offset = headerSize
	for _, cell := range rightCells {
		binary.LittleEndian.PutUint32(rightPage[offset:offset+4], cell.childPage)
		offset += 4
		binary.LittleEndian.PutUint16(rightPage[offset:offset+2], uint16(len(cell.key)))
		offset += 2
		copy(rightPage[offset:], cell.key)
		offset += len(cell.key)
	}
	if err := bt.pager.WritePage(newPageNum, rightPage); err != nil {
		return nil, err
	}

	return &splitResult{
		key:       promotedKey,
		leftPage:  pageNum,
		rightPage: newPageNum,
	}, nil
}
