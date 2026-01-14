# Blobber Archive Spec B (Sorted Index + Hash Table)

This document defines the on-disk format for the Blobber archive index using a
sorted entry table plus a hash table for O(1) average lookups.

## Goals

- Fast random access for 10k+ files
- Deterministic listing order
- Low round trips (index as separate blob)
- Strong integrity: index checksum + per-file hash

## Non-goals

- Partial file reads
- Multi-layer sharding
- Deduplication or chunking
- Backwards compatibility with tar/zip

## Overview

An archive consists of two blobs:

1) Index blob (this format)
2) Data blob (concatenated file payloads)

Entries are sorted by path bytes for listing. A hash table maps path hashes to
entry indices for fast lookup.

## Header (fixed 128 bytes, little-endian)

All offsets are from the start of the index blob.

| Field | Size | Notes |
| --- | --- | --- |
| Magic | 8 | "BLBRIDX\0" |
| Version | u16 | Format version |
| HeaderSize | u16 | Always 128 |
| Flags | u32 | Reserved, must be 0 |
| EntryCount | u32 | Number of entries |
| EntrySize | u16 | Always 96 |
| PathHashAlg | u8 | 1 (fnv1a64) |
| ChecksumAlg | u8 | 1 (sha256) |
| IndexSize | u64 | Total index blob size in bytes |
| EntriesOff | u64 | Offset to entries table |
| EntriesLen | u64 | Entries table length |
| StringsOff | u64 | Offset to string table |
| StringsLen | u64 | String table length |
| HashOff | u64 | Offset to hash table |
| HashLen | u64 | Hash table length |
| HashSlots | u32 | Number of slots (power of two) |
| Reserved | u32 | 0 |
| Checksum | 32 | SHA-256 over bytes [HeaderSize, IndexSize) |
| Reserved2 | 8 | 0 |

## Entry (96 bytes, little-endian)

Entries are sorted by path bytes.

| Field | Size | Notes |
| --- | --- | --- |
| PathOff | u32 | Offset into string table |
| PathLen | u32 | Length of path bytes |
| LinkOff | u32 | Offset to symlink target (0 if not symlink) |
| LinkLen | u32 | Length of symlink target (0 if not symlink) |
| DataOff | u64 | Offset into data blob |
| CompSize | u64 | Compressed size in data blob |
| RawSize | u64 | Uncompressed size |
| ContentHash | 32 | SHA-256 of uncompressed content |
| Mode | u32 | Permission bits (e.g. 0o644) |
| MTime | i64 | Unix seconds |
| Compression | u8 | 0=none,1=zstd,2=gzip |
| Type | u8 | 0=file,1=dir,2=symlink |
| Reserved | u16 | 0 |
| PathHash | u64 | FNV-1a 64 over normalized path bytes |

### Type semantics

- File: DataOff/CompSize/RawSize are valid.
- Dir: DataOff/CompSize/RawSize are 0, ContentHash is all zeros.
- Symlink: LinkOff/LinkLen reference target; data fields are 0.

## String table

- Concatenated UTF-8 path bytes, no terminators.
- Paths are case-sensitive and use '/' separators.
- Paths MUST be relative, MUST NOT contain "..", and MUST NOT contain NUL.

## Hash table

- Open addressing, power-of-two slot count.
- Each slot is a u32 entry index.
- Empty slot sentinel: 0xFFFFFFFF.
- Probe sequence: linear (index = (hash + i) & (slots-1)).
- On hash match, compare path bytes to confirm.

## Data blob

- Concatenated file payloads in the same order as entries.
- Each file payload is a contiguous segment (full-file reads only).
- If Compression=none, payload bytes are raw file contents.
- If Compression!=none, payload bytes are the compressed full file.

## Parsing and validation

1) Read header and verify Magic/Version/HeaderSize.
2) Verify Checksum over index body.
3) Validate offsets and lengths are within IndexSize.
4) Parse entries table and hash table.
5) Optionally validate that entries are sorted by path bytes.
6) Validate hash table entries are in range.

## Lookup algorithm

- Normalize input path to '/' and verify it meets path rules.
- Compute FNV-1a 64 hash.
- Probe hash table for entry index; confirm with path bytes.

## Integrity checks

- Index checksum protects metadata from corruption.
- Per-file SHA-256 is verified after full-file read and decompression.
- Errors must be returned on mismatch.

## Forward compatibility

- Header includes EntrySize for future expansion.
- Unknown fields must be ignored if EntrySize exceeds known size.
