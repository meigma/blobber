package archive

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"math"
	"sort"
	"time"
)

// OpenOption configures OpenIndex behavior.
type OpenOption func(*openOptions)

type openOptions struct {
	lookupCache bool
}

// WithLookupCache enables an in-memory lookup map for faster path lookups.
// This increases memory usage and adds build cost during OpenIndex.
func WithLookupCache() OpenOption {
	return func(o *openOptions) {
		o.lookupCache = true
	}
}

// Index represents a parsed index blob.
type Index struct {
	layout    Layout
	entries   []entryRecord
	strings   []byte
	hashTable []uint32

	lookupCache map[string]int
	entryCache  []Entry
}

// OpenIndex parses an index blob from a ReaderAt.
func OpenIndex(r io.ReaderAt, size int64, opts ...OpenOption) (*Index, error) {
	if size < headerSize {
		return nil, ErrInvalidHeader
	}
	if size > int64(^uint(0)>>1) {
		return nil, ErrInvalidHeader
	}

	buf := make([]byte, size)
	if err := readFullAt(r, buf, 0); err != nil {
		return nil, err
	}

	hdr, layout, err := parseHeader(buf)
	if err != nil {
		return nil, err
	}
	if err := ensureIndexSize(hdr.indexSize, len(buf)); err != nil {
		return nil, err
	}

	sum := sha256.Sum256(buf[headerSize:])
	if !bytes.Equal(sum[:], hdr.checksum[:]) {
		return nil, ErrInvalidChecksum
	}

	if err := validateIndexBounds(hdr.entriesOff, hdr.entriesLen, len(buf)); err != nil {
		return nil, err
	}
	if err := validateIndexBounds(hdr.stringsOff, hdr.stringsLen, len(buf)); err != nil {
		return nil, err
	}
	if layout == LayoutB {
		if err := validateIndexBounds(hdr.hashOff, hdr.hashLen, len(buf)); err != nil {
			return nil, err
		}
	}

	entriesStart := int(hdr.entriesOff)
	entriesEnd := int(hdr.entriesOff + hdr.entriesLen)
	entries, err := parseEntries(buf[entriesStart:entriesEnd], layout)
	if err != nil {
		return nil, err
	}
	stringsStart := int(hdr.stringsOff)
	stringsEnd := int(hdr.stringsOff + hdr.stringsLen)
	strings := buf[stringsStart:stringsEnd]

	if err := validateEntries(entries, strings); err != nil {
		return nil, err
	}

	var table []uint32
	if layout == LayoutB {
		hashStart := int(hdr.hashOff)
		hashEnd := int(hdr.hashOff + hdr.hashLen)
		table, err = parseHashTable(buf[hashStart:hashEnd], hdr.hashSlots)
		if err != nil {
			return nil, err
		}
	}

	idx := &Index{
		layout:    layout,
		entries:   entries,
		strings:   strings,
		hashTable: table,
	}

	options := openOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	if options.lookupCache {
		if err := idx.BuildLookupCache(); err != nil {
			return nil, err
		}
	}

	return idx, nil
}

// Lookup returns the entry for a path if it exists.
func (idx *Index) Lookup(path string) (Entry, bool, error) {
	clean, err := normalizePath(path)
	if err != nil {
		return Entry{}, false, err
	}

	return idx.LookupNormalized(clean)
}

// LookupIndex returns the entry index for a path if it exists.
func (idx *Index) LookupIndex(path string) (int, bool, error) {
	clean, err := normalizePath(path)
	if err != nil {
		return 0, false, err
	}

	return idx.LookupIndexNormalized(clean)
}

// LookupNormalized returns the entry for a canonical path without re-validating it.
// Call NormalizePath before using this method with untrusted input.
func (idx *Index) LookupNormalized(path string) (Entry, bool, error) {
	entryIndex, ok, err := idx.LookupIndexNormalized(path)
	if err != nil || !ok {
		return Entry{}, ok, err
	}
	if entryIndex < 0 || entryIndex >= len(idx.entries) {
		return Entry{}, false, ErrInvalidIndex
	}
	if len(idx.entryCache) > 0 {
		return idx.entryCache[entryIndex], true, nil
	}
	return idx.toEntry(idx.entries[entryIndex]), true, nil
}

// LookupIndexNormalized returns the entry index for a canonical path without re-validating it.
// Call NormalizePath before using this method with untrusted input.
func (idx *Index) LookupIndexNormalized(path string) (int, bool, error) {
	if path == "" {
		return 0, false, ErrInvalidPath
	}
	if idx.lookupCache != nil {
		entryIndex, ok := idx.lookupCache[path]
		if !ok {
			return 0, false, nil
		}
		if entryIndex < 0 || entryIndex >= len(idx.entries) {
			return 0, false, ErrInvalidIndex
		}
		return entryIndex, true, nil
	}

	pathBytes := []byte(path)
	switch idx.layout {
	case LayoutA:
		pos := sort.Search(len(idx.entries), func(i int) bool {
			return bytes.Compare(idx.entryPathBytes(idx.entries[i]), pathBytes) >= 0
		})
		if pos >= len(idx.entries) {
			return 0, false, nil
		}
		if !bytes.Equal(idx.entryPathBytes(idx.entries[pos]), pathBytes) {
			return 0, false, nil
		}
		return pos, true, nil
	case LayoutB:
		entryIndex, ok := idx.lookupHash(pathBytes)
		if !ok {
			return 0, false, nil
		}
		return entryIndex, true, nil
	default:
		return 0, false, ErrInvalidIndex
	}
}

// Entries returns all entries in sorted path order.
func (idx *Index) Entries() []Entry {
	if len(idx.entryCache) > 0 {
		cached := make([]Entry, len(idx.entryCache))
		copy(cached, idx.entryCache)
		return cached
	}
	out := make([]Entry, len(idx.entries))
	for i, rec := range idx.entries {
		out[i] = idx.toEntry(rec)
	}
	return out
}

// Layout returns the index layout.
func (idx *Index) Layout() Layout {
	return idx.layout
}

// EntryAt returns the entry by index in sorted order.
func (idx *Index) EntryAt(i int) (Entry, bool) {
	if i < 0 || i >= len(idx.entries) {
		return Entry{}, false
	}
	if len(idx.entryCache) > 0 {
		return idx.entryCache[i], true
	}
	return idx.toEntry(idx.entries[i]), true
}

// BuildLookupCache builds an in-memory map for faster lookups.
// It is safe to call multiple times; subsequent calls are no-ops.
func (idx *Index) BuildLookupCache() error {
	if idx.lookupCache != nil {
		return nil
	}

	cache := make(map[string]int, len(idx.entries))
	entries := make([]Entry, len(idx.entries))
	for i, rec := range idx.entries {
		path := string(idx.entryPathBytes(rec))
		if _, exists := cache[path]; exists {
			return ErrInvalidIndex
		}
		cache[path] = i

		entries[i] = idx.toEntry(rec)
	}

	idx.lookupCache = cache
	idx.entryCache = entries
	return nil
}

// BuildLookupMap builds the in-memory lookup map without prebuilding Entry values.
func (idx *Index) BuildLookupMap() error {
	if idx.lookupCache != nil {
		return nil
	}

	cache := make(map[string]int, len(idx.entries))
	for i, rec := range idx.entries {
		path := string(idx.entryPathBytes(rec))
		if _, exists := cache[path]; exists {
			return ErrInvalidIndex
		}
		cache[path] = i
	}

	idx.lookupCache = cache
	idx.entryCache = nil
	return nil
}

type entryRecord struct {
	pathOff     uint32
	pathLen     uint32
	linkOff     uint32
	linkLen     uint32
	dataOff     uint64
	compSize    uint64
	rawSize     uint64
	hash        [32]byte
	mode        uint32
	mtime       int64
	compression Compression
	typeFlag    EntryType
	pathHash    uint64
}

type header struct {
	indexSize  uint64
	entriesOff uint64
	entriesLen uint64
	stringsOff uint64
	stringsLen uint64
	hashOff    uint64
	hashLen    uint64
	hashSlots  uint32
	checksum   [32]byte
}

func parseHeader(buf []byte) (header, Layout, error) {
	if len(buf) < headerSize {
		return header{}, 0, ErrInvalidHeader
	}
	if !bytes.Equal(buf[0:8], []byte(magic)) {
		return header{}, 0, ErrInvalidMagic
	}
	if binary.LittleEndian.Uint16(buf[8:]) != version {
		return header{}, 0, ErrUnsupportedVersion
	}
	if binary.LittleEndian.Uint16(buf[10:]) != headerSize {
		return header{}, 0, ErrInvalidHeader
	}

	entryCount := binary.LittleEndian.Uint32(buf[16:])
	entrySize := binary.LittleEndian.Uint16(buf[20:])
	pathHashAlg := buf[22]
	checksumAlg := buf[23]

	if checksumAlg != checksumSHA256 {
		return header{}, 0, ErrInvalidHeader
	}

	var layout Layout
	switch pathHashAlg {
	case pathHashNone:
		layout = LayoutA
		if entrySize != entrySizeA {
			return header{}, 0, ErrInvalidHeader
		}
	case pathHashFNV64:
		layout = LayoutB
		if entrySize != entrySizeB {
			return header{}, 0, ErrInvalidHeader
		}
	default:
		return header{}, 0, ErrInvalidHeader
	}

	hdr := header{
		indexSize:  binary.LittleEndian.Uint64(buf[24:]),
		entriesOff: binary.LittleEndian.Uint64(buf[32:]),
		entriesLen: binary.LittleEndian.Uint64(buf[40:]),
		stringsOff: binary.LittleEndian.Uint64(buf[48:]),
		stringsLen: binary.LittleEndian.Uint64(buf[56:]),
		hashOff:    binary.LittleEndian.Uint64(buf[64:]),
		hashLen:    binary.LittleEndian.Uint64(buf[72:]),
		hashSlots:  binary.LittleEndian.Uint32(buf[80:]),
	}
	copy(hdr.checksum[:], buf[88:120])

	if hdr.entriesLen != uint64(entryCount)*uint64(entrySize) {
		return header{}, 0, ErrInvalidHeader
	}
	if hdr.entriesOff < headerSize {
		return header{}, 0, ErrInvalidHeader
	}
	if layout == LayoutA {
		if hdr.hashOff != 0 || hdr.hashLen != 0 || hdr.hashSlots != 0 {
			return header{}, 0, ErrInvalidHeader
		}
	}
	if layout == LayoutB {
		if hdr.hashOff == 0 || hdr.hashLen == 0 || hdr.hashSlots == 0 {
			return header{}, 0, ErrInvalidHeader
		}
	}

	return hdr, layout, nil
}

func parseEntries(buf []byte, layout Layout) ([]entryRecord, error) {
	size := entrySize(layout)
	if size == 0 {
		return nil, ErrInvalidIndex
	}
	if len(buf)%size != 0 {
		return nil, ErrInvalidIndex
	}

	entries := make([]entryRecord, len(buf)/size)
	for i := range entries {
		off := i * size
		entries[i] = entryRecord{
			pathOff:     binary.LittleEndian.Uint32(buf[off+0:]),
			pathLen:     binary.LittleEndian.Uint32(buf[off+4:]),
			linkOff:     binary.LittleEndian.Uint32(buf[off+8:]),
			linkLen:     binary.LittleEndian.Uint32(buf[off+12:]),
			dataOff:     binary.LittleEndian.Uint64(buf[off+16:]),
			compSize:    binary.LittleEndian.Uint64(buf[off+24:]),
			rawSize:     binary.LittleEndian.Uint64(buf[off+32:]),
			mode:        binary.LittleEndian.Uint32(buf[off+72:]),
			mtime:       int64(binary.LittleEndian.Uint64(buf[off+76:])),
			compression: Compression(buf[off+84]),
			typeFlag:    EntryType(buf[off+85]),
		}
		copy(entries[i].hash[:], buf[off+40:off+72])
		if layout == LayoutB {
			entries[i].pathHash = binary.LittleEndian.Uint64(buf[off+88:])
		}
	}

	return entries, nil
}

func parseHashTable(buf []byte, slots uint32) ([]uint32, error) {
	if slots == 0 {
		return nil, ErrInvalidIndex
	}
	if len(buf) != int(slots)*4 {
		return nil, ErrInvalidIndex
	}

	table := make([]uint32, slots)
	for i := range table {
		table[i] = binary.LittleEndian.Uint32(buf[i*4:])
	}
	return table, nil
}

func validateEntries(entries []entryRecord, strings []byte) error {
	for i := range entries {
		if err := validateEntryBounds(entries[i].pathOff, entries[i].pathLen, len(strings)); err != nil {
			return err
		}
		if entries[i].typeFlag == EntrySymlink {
			if err := validateEntryBounds(entries[i].linkOff, entries[i].linkLen, len(strings)); err != nil {
				return err
			}
		}
		if entries[i].typeFlag > EntrySymlink {
			return ErrInvalidIndex
		}
		switch entries[i].compression {
		case CompressionNone, CompressionGzip, CompressionZstd:
		default:
			return ErrInvalidIndex
		}
	}

	for i := 1; i < len(entries); i++ {
		prev := entryPathBytes(entries[i-1], strings)
		curr := entryPathBytes(entries[i], strings)
		cmp := bytes.Compare(prev, curr)
		if cmp > 0 {
			return ErrInvalidIndex
		}
		if cmp == 0 {
			return ErrInvalidIndex
		}
	}
	return nil
}

func (idx *Index) entryPathBytes(entry entryRecord) []byte {
	return entryPathBytes(entry, idx.strings)
}

func entryPathBytes(entry entryRecord, strings []byte) []byte {
	start := int(entry.pathOff)
	end := start + int(entry.pathLen)
	return strings[start:end]
}

func (idx *Index) lookupHash(path []byte) (int, bool) {
	if len(idx.hashTable) == 0 {
		return 0, false
	}
	hash := fnv1a64(path)
	mask := uint64(len(idx.hashTable) - 1)

	for probe := uint64(0); probe < uint64(len(idx.hashTable)); probe++ {
		slot := (hash + probe) & mask
		entryIndex := idx.hashTable[slot]
		if entryIndex == math.MaxUint32 {
			return 0, false
		}
		if int(entryIndex) >= len(idx.entries) {
			return 0, false
		}
		entry := idx.entries[entryIndex]
		if entry.pathHash != hash {
			continue
		}
		if bytes.Equal(idx.entryPathBytes(entry), path) {
			return int(entryIndex), true
		}
	}
	return 0, false
}

func (idx *Index) toEntry(entry entryRecord) Entry {
	path := string(idx.entryPathBytes(entry))
	link := ""
	if entry.typeFlag == EntrySymlink {
		start := int(entry.linkOff)
		end := start + int(entry.linkLen)
		link = string(idx.strings[start:end])
	}

	return Entry{
		Path:        path,
		Type:        entry.typeFlag,
		Mode:        entry.mode,
		MTime:       time.Unix(entry.mtime, 0).UTC(),
		LinkTarget:  link,
		DataOffset:  entry.dataOff,
		CompSize:    entry.compSize,
		RawSize:     entry.rawSize,
		Hash:        entry.hash,
		Compression: entry.compression,
	}
}

func readFullAt(r io.ReaderAt, buf []byte, off int64) error {
	read := 0
	for read < len(buf) {
		n, err := r.ReadAt(buf[read:], off+int64(read))
		read += n
		if err != nil {
			if err == io.EOF && read == len(buf) {
				return nil
			}
			return err
		}
	}
	return nil
}
