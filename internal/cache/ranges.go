package cache

import "sort"

// mergeRanges takes a slice of ranges and returns a new slice with overlapping
// or adjacent ranges merged. The returned slice is sorted by offset.
func mergeRanges(ranges []Range) []Range {
	if len(ranges) == 0 {
		return nil
	}

	// Sort by offset
	sorted := make([]Range, len(ranges))
	copy(sorted, ranges)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Offset < sorted[j].Offset
	})

	// Merge overlapping or adjacent ranges
	merged := make([]Range, 0, len(sorted))
	current := sorted[0]

	for i := 1; i < len(sorted); i++ {
		next := sorted[i]
		// Check if ranges overlap or are adjacent
		if next.Offset <= current.End() {
			// Extend current range if next extends beyond
			if next.End() > current.End() {
				current.Length = next.End() - current.Offset
			}
		} else {
			// No overlap, add current and start new
			merged = append(merged, current)
			current = next
		}
	}
	merged = append(merged, current)

	return merged
}

// addRange adds a new range to an existing slice of ranges and merges overlaps.
// Returns the merged result.
func addRange(ranges []Range, newRange Range) []Range {
	return mergeRanges(append(ranges, newRange))
}

// findGaps returns the byte ranges that are missing between the given ranges
// and the total size. The returned ranges cover all bytes from 0 to totalSize
// that are not covered by any input range.
func findGaps(ranges []Range, totalSize int64) []Range {
	if totalSize <= 0 {
		return nil
	}

	if len(ranges) == 0 {
		return []Range{{Offset: 0, Length: totalSize}}
	}

	// Ensure ranges are merged and sorted
	merged := mergeRanges(ranges)

	var gaps []Range
	pos := int64(0)

	for _, r := range merged {
		if r.Offset > pos {
			gaps = append(gaps, Range{
				Offset: pos,
				Length: r.Offset - pos,
			})
		}
		if r.End() > pos {
			pos = r.End()
		}
	}

	// Check for gap at the end
	if pos < totalSize {
		gaps = append(gaps, Range{
			Offset: pos,
			Length: totalSize - pos,
		})
	}

	return gaps
}

// totalCoverage returns the total number of bytes covered by the ranges.
func totalCoverage(ranges []Range) int64 {
	if len(ranges) == 0 {
		return 0
	}

	merged := mergeRanges(ranges)
	var total int64
	for _, r := range merged {
		total += r.Length
	}
	return total
}

// isComplete returns true if the ranges cover all bytes from 0 to totalSize.
func isComplete(ranges []Range, totalSize int64) bool {
	return totalCoverage(ranges) == totalSize && len(findGaps(ranges, totalSize)) == 0
}

// containsRange returns true if the given offset and length are fully covered
// by the existing ranges.
func containsRange(ranges []Range, offset, length int64) bool {
	if len(ranges) == 0 {
		return false
	}

	merged := mergeRanges(ranges)
	end := offset + length

	for _, r := range merged {
		if r.Offset <= offset && r.End() >= end {
			return true
		}
	}
	return false
}
