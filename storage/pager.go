package storage

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync"
)

// PAGE SIZE: 4096 bytes (4KB)
//
// WHY 4KB?
// Real databases like SQLite, PostgreSQL, and MySQL all use page-based storage.
// A "page" is the smallest unit of I/O — the database reads and writes entire pages.
// 4KB aligns with the OS memory page size and disk sector size, making I/O efficient.
//
// Each page is read from and written to the file at offset = pageNum * PAGE_SIZE.
const PageSize = 4096

// MaxPages is the maximum number of pages our database file can hold.
const MaxPages = 10000

// Pager manages reading and writing fixed-size pages to/from the database file.
//
// HOW IT WORKS:
// - The database file is divided into 4KB pages, numbered 0, 1, 2, ...
// - Page 0 is reserved for metadata (total pages, etc.)
// - When you request page N, the pager seeks to offset N*4096 in the file and reads 4096 bytes.
// - Pages are cached in memory to avoid repeated disk reads.
// - When you modify a page, you call WritePage() to flush it to disk.
type Pager struct {
	file     *os.File
	fileSize int64
	numPages uint32            // total number of pages in the file
	cache    map[uint32][]byte // in-memory page cache: pageNum → page data
	mu       sync.RWMutex
}

// OpenPager opens an existing database file or creates a new one.
// It reads the file header (page 0) to determine the number of pages.
func OpenPager(path string) (*Pager, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open database file: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat database file: %w", err)
	}

	p := &Pager{
		file:     file,
		fileSize: info.Size(),
		cache:    make(map[uint32][]byte),
	}

	if info.Size() == 0 {
		// New database — initialize with a header page
		p.numPages = 1 // page 0 is the header
		header := make([]byte, PageSize)
		copy(header[0:4], []byte("MYDB"))                      // magic bytes
		binary.LittleEndian.PutUint32(header[4:8], p.numPages) // page count
		if err := p.writePageRaw(0, header); err != nil {
			file.Close()
			return nil, err
		}
		p.fileSize = PageSize
	} else {
		// Existing database — read header
		header := make([]byte, PageSize)
		if _, err := file.ReadAt(header, 0); err != nil {
			file.Close()
			return nil, fmt.Errorf("failed to read header page: %w", err)
		}
		magic := string(header[0:4])
		if magic != "MYDB" {
			file.Close()
			return nil, fmt.Errorf("invalid database file (bad magic: %q)", magic)
		}
		p.numPages = binary.LittleEndian.Uint32(header[4:8])
	}

	return p, nil
}

// Close flushes any cached writes and closes the database file.
func (p *Pager) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.file.Close()
}

// NumPages returns the current number of pages in the database file.
func (p *Pager) NumPages() uint32 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.numPages
}

// ReadPage reads a page from disk (or cache) and returns a copy of its data.
func (p *Pager) ReadPage(pageNum uint32) ([]byte, error) {
	p.mu.RLock()
	if cached, ok := p.cache[pageNum]; ok {
		p.mu.RUnlock()
		cpy := make([]byte, PageSize)
		copy(cpy, cached)
		return cpy, nil
	}
	p.mu.RUnlock()

	if pageNum >= p.numPages {
		return nil, fmt.Errorf("page %d out of range (have %d pages)", pageNum, p.numPages)
	}

	data := make([]byte, PageSize)
	offset := int64(pageNum) * int64(PageSize)
	if _, err := p.file.ReadAt(data, offset); err != nil {
		return nil, fmt.Errorf("failed to read page %d: %w", pageNum, err)
	}

	// Cache it
	p.mu.Lock()
	p.cache[pageNum] = make([]byte, PageSize)
	copy(p.cache[pageNum], data)
	p.mu.Unlock()

	return data, nil
}

// WritePage writes a page to both cache and disk.
func (p *Pager) WritePage(pageNum uint32, data []byte) error {
	if len(data) != PageSize {
		return fmt.Errorf("page data must be exactly %d bytes, got %d", PageSize, len(data))
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Update cache
	p.cache[pageNum] = make([]byte, PageSize)
	copy(p.cache[pageNum], data)

	// Write to disk
	return p.writePageRaw(pageNum, data)
}

// AllocatePage creates a new page and returns its page number.
// The page is filled with zeros.
func (p *Pager) AllocatePage() (uint32, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.numPages >= MaxPages {
		return 0, fmt.Errorf("maximum number of pages (%d) reached", MaxPages)
	}

	newPageNum := p.numPages
	p.numPages++

	// Write empty page
	emptyPage := make([]byte, PageSize)
	if err := p.writePageRaw(newPageNum, emptyPage); err != nil {
		return 0, err
	}

	// Update header with new page count
	if err := p.updateHeader(); err != nil {
		return 0, err
	}

	p.fileSize = int64(p.numPages) * int64(PageSize)
	return newPageNum, nil
}

// --- Internal helpers ---

// writePageRaw writes raw bytes to a page offset on disk.
func (p *Pager) writePageRaw(pageNum uint32, data []byte) error {
	offset := int64(pageNum) * int64(PageSize)
	if _, err := p.file.WriteAt(data, offset); err != nil {
		return fmt.Errorf("failed to write page %d: %w", pageNum, err)
	}
	return p.file.Sync()
}

// updateHeader writes the current numPages to the header page (page 0).
func (p *Pager) updateHeader() error {
	header, ok := p.cache[0]
	if !ok {
		header = make([]byte, PageSize)
		copy(header[0:4], []byte("MYDB"))
	}
	binary.LittleEndian.PutUint32(header[4:8], p.numPages)
	p.cache[0] = header
	return p.writePageRaw(0, header)
}

// InvalidateCache clears the page cache, forcing re-reads from disk.
func (p *Pager) InvalidateCache() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = make(map[uint32][]byte)
}
