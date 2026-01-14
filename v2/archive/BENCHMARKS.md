# Archive Index Benchmark Strategy

This document defines how we will benchmark index layout A vs B vs estargz.

## Goals

- Validate that new formats outperform estargz for target workloads
- Quantify index size, parse time, and lookup latency
- Measure round-trip counts and bytes read for key access patterns

## Variants

- Layout A: sorted entries, binary search
- Layout B: sorted entries + hash table lookup
- estargz: current TOC-based lookup (baseline)

## Datasets

Use generated datasets that simulate real workloads:

1) Small: 1k files, 1-8 KB each
2) Medium: 10k files, 1-8 KB each
3) Mixed: 10k files, 1 KB to 10 MB
4) Deep paths: depth 1-6, varied directory fanout

Paths should be unique and normalized. For consistency, seed the generator.

## Access patterns

1) Index-only parse
2) List all entries
3) Random lookups: 1, 10, 100, 1000 files
4) Mixed: list + open N random files
5) Full extract (sequential reads of all files)

## Metrics

- Index size (bytes)
- Parse time (ns/op)
- Allocations (allocs/op, bytes/op)
- Lookup latency (ns/op)
- Bytes read for index and data
- Simulated range-read count
- First-byte latency to open a file (index + data)

## Harness approach

- Provide a single dataset generator shared by all variants.
- Create reference implementations that read from a synthetic ReaderAt which
  counts ReadAt calls and total bytes read.
- For estargz, wrap its ReaderAt and report TOC read size and parse time.
- For A and B, measure index parse and lookup latency separately.

## Fairness rules

- All variants read the same logical dataset.
- Avoid filesystem I/O; use in-memory byte slices or temp files consistently.
- Use the same compression settings across variants when relevant.
- Repeat runs with identical RNG seed.

## Reporting

- Compare A vs B vs estargz for each dataset and access pattern.
- Record median and p95 latencies.
- Highlight cases where A or B is worse than estargz and explain why.

## Success criteria

- Layout A and B should reduce index parse time and ReadAt count vs estargz.
- Layout B should outperform A for random lookup latency.
- Index size growth for B should stay within a tolerable multiple (target <= 1.5x A).
