package archive

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
)

const (
	entrySizeA = 88
	entrySizeB = 96

	pathHashNone  = 0
	pathHashFNV64 = 1

	checksumSHA256 = 1
)

func marshalEntries(entries []buildEntry, layout Layout) ([]byte, error) {
	size := entrySize(layout)
	if size == 0 {
		return nil, ErrInvalidIndex
	}

	buf := make([]byte, len(entries)*size)
	for i, entry := range entries {
		off := i * size
		binary.LittleEndian.PutUint32(buf[off+0:], entry.pathOff)
		binary.LittleEndian.PutUint32(buf[off+4:], entry.pathLen)
		binary.LittleEndian.PutUint32(buf[off+8:], entry.linkOff)
		binary.LittleEndian.PutUint32(buf[off+12:], entry.linkLen)
		binary.LittleEndian.PutUint64(buf[off+16:], entry.dataOffset)
		binary.LittleEndian.PutUint64(buf[off+24:], entry.compSize)
		binary.LittleEndian.PutUint64(buf[off+32:], entry.rawSize)
		copy(buf[off+40:off+72], entry.hash[:])
		binary.LittleEndian.PutUint32(buf[off+72:], entry.mode)
		binary.LittleEndian.PutUint64(buf[off+76:], uint64(entry.mtime))
		buf[off+84] = byte(entry.compression)
		buf[off+85] = byte(entry.typeFlag)
		// off+86..off+87 reserved

		if layout == LayoutB {
			binary.LittleEndian.PutUint64(buf[off+88:], entry.pathHash)
		}
	}

	return buf, nil
}

func buildHashTable(entries []buildEntry) ([]byte, uint32, error) {
	if len(entries) == 0 {
		return nil, 0, nil
	}

	slots := nextPow2(uint32(len(entries) * 2))
	table := make([]uint32, slots)
	for i := range table {
		table[i] = math.MaxUint32
	}

	mask := uint64(slots - 1)
	for i, entry := range entries {
		idx := uint32(i)
		for probe := uint64(0); probe < uint64(slots); probe++ {
			slot := (entry.pathHash + probe) & mask
			if table[slot] == math.MaxUint32 {
				table[slot] = idx
				break
			}
			if probe == uint64(slots-1) {
				return nil, 0, ErrInvalidIndex
			}
		}
	}

	buf := make([]byte, int(slots)*4)
	for i, v := range table {
		binary.LittleEndian.PutUint32(buf[i*4:], v)
	}
	return buf, slots, nil
}

func buildIndex(entriesBytes, stringsBytes, hashBytes []byte, hashSlots uint32, layout Layout) ([]byte, error) {
	entrySize := entrySize(layout)
	if entrySize == 0 {
		return nil, ErrInvalidIndex
	}
	if len(entriesBytes)%entrySize != 0 {
		return nil, ErrInvalidIndex
	}

	entryCount := uint32(len(entriesBytes) / entrySize)
	indexSize := headerSize + len(entriesBytes) + len(stringsBytes) + len(hashBytes)

	if indexSize == 0 {
		return nil, ErrInvalidIndex
	}
	if indexSize < headerSize {
		return nil, ErrInvalidIndex
	}

	entriesOff := headerSize
	stringsOff := entriesOff + len(entriesBytes)
	hashOff := stringsOff + len(stringsBytes)

	buf := make([]byte, indexSize)
	copy(buf[0:8], []byte(magic))
	binary.LittleEndian.PutUint16(buf[8:], version)
	binary.LittleEndian.PutUint16(buf[10:], headerSize)
	binary.LittleEndian.PutUint32(buf[12:], 0)
	binary.LittleEndian.PutUint32(buf[16:], entryCount)
	binary.LittleEndian.PutUint16(buf[20:], uint16(entrySize))

	pathHashAlg := uint8(pathHashNone)
	if layout == LayoutB {
		pathHashAlg = pathHashFNV64
	}
	buf[22] = pathHashAlg
	buf[23] = checksumSHA256
	binary.LittleEndian.PutUint64(buf[24:], uint64(indexSize))
	binary.LittleEndian.PutUint64(buf[32:], uint64(entriesOff))
	binary.LittleEndian.PutUint64(buf[40:], uint64(len(entriesBytes)))
	binary.LittleEndian.PutUint64(buf[48:], uint64(stringsOff))
	binary.LittleEndian.PutUint64(buf[56:], uint64(len(stringsBytes)))

	if layout == LayoutB {
		binary.LittleEndian.PutUint64(buf[64:], uint64(hashOff))
		binary.LittleEndian.PutUint64(buf[72:], uint64(len(hashBytes)))
		binary.LittleEndian.PutUint32(buf[80:], hashSlots)
	}

	copy(buf[entriesOff:], entriesBytes)
	copy(buf[stringsOff:], stringsBytes)
	if len(hashBytes) > 0 {
		copy(buf[hashOff:], hashBytes)
	}

	sum := sha256.Sum256(buf[headerSize:])
	copy(buf[88:], sum[:])

	return buf, nil
}

func entrySize(layout Layout) int {
	switch layout {
	case LayoutA:
		return entrySizeA
	case LayoutB:
		return entrySizeB
	default:
		return 0
	}
}

func nextPow2(v uint32) uint32 {
	if v <= 1 {
		return 1
	}
	if v > 1<<31 {
		return 0
	}
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v++
	return v
}

func validateIndexBounds(off, length uint64, size int) error {
	if off > uint64(size) {
		return ErrInvalidIndex
	}
	if length > uint64(size)-off {
		return ErrInvalidIndex
	}
	return nil
}

func validateEntryBounds(off, length uint32, size int) error {
	end := uint64(off) + uint64(length)
	if end > uint64(size) {
		return ErrInvalidIndex
	}
	return nil
}

func ensureIndexSize(indexSize uint64, size int) error {
	if indexSize != uint64(size) {
		return fmt.Errorf("index size: %w", ErrInvalidIndex)
	}
	return nil
}
