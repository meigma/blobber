package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeRanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    []Range
		expected []Range
	}{
		{
			name:     "empty",
			input:    nil,
			expected: nil,
		},
		{
			name:     "single range",
			input:    []Range{{Offset: 0, Length: 100}},
			expected: []Range{{Offset: 0, Length: 100}},
		},
		{
			name: "non-overlapping",
			input: []Range{
				{Offset: 0, Length: 100},
				{Offset: 200, Length: 100},
			},
			expected: []Range{
				{Offset: 0, Length: 100},
				{Offset: 200, Length: 100},
			},
		},
		{
			name: "overlapping",
			input: []Range{
				{Offset: 0, Length: 100},
				{Offset: 50, Length: 100},
			},
			expected: []Range{{Offset: 0, Length: 150}},
		},
		{
			name: "adjacent",
			input: []Range{
				{Offset: 0, Length: 100},
				{Offset: 100, Length: 100},
			},
			expected: []Range{{Offset: 0, Length: 200}},
		},
		{
			name: "unsorted",
			input: []Range{
				{Offset: 200, Length: 100},
				{Offset: 0, Length: 100},
				{Offset: 100, Length: 100},
			},
			expected: []Range{{Offset: 0, Length: 300}},
		},
		{
			name: "contained range",
			input: []Range{
				{Offset: 0, Length: 200},
				{Offset: 50, Length: 50},
			},
			expected: []Range{{Offset: 0, Length: 200}},
		},
		{
			name: "multiple merges",
			input: []Range{
				{Offset: 0, Length: 50},
				{Offset: 40, Length: 50},
				{Offset: 200, Length: 50},
				{Offset: 240, Length: 50},
			},
			expected: []Range{
				{Offset: 0, Length: 90},
				{Offset: 200, Length: 90},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := mergeRanges(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAddRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		existing []Range
		newRange Range
		expected []Range
	}{
		{
			name:     "add to empty",
			existing: nil,
			newRange: Range{Offset: 0, Length: 100},
			expected: []Range{{Offset: 0, Length: 100}},
		},
		{
			name:     "add non-overlapping",
			existing: []Range{{Offset: 0, Length: 100}},
			newRange: Range{Offset: 200, Length: 100},
			expected: []Range{
				{Offset: 0, Length: 100},
				{Offset: 200, Length: 100},
			},
		},
		{
			name:     "add overlapping",
			existing: []Range{{Offset: 0, Length: 100}},
			newRange: Range{Offset: 50, Length: 100},
			expected: []Range{{Offset: 0, Length: 150}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := addRange(tt.existing, tt.newRange)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindGaps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ranges    []Range
		totalSize int64
		expected  []Range
	}{
		{
			name:      "no ranges - full gap",
			ranges:    nil,
			totalSize: 1000,
			expected:  []Range{{Offset: 0, Length: 1000}},
		},
		{
			name:      "complete - no gaps",
			ranges:    []Range{{Offset: 0, Length: 1000}},
			totalSize: 1000,
			expected:  nil,
		},
		{
			name:      "gap at start",
			ranges:    []Range{{Offset: 500, Length: 500}},
			totalSize: 1000,
			expected:  []Range{{Offset: 0, Length: 500}},
		},
		{
			name:      "gap at end",
			ranges:    []Range{{Offset: 0, Length: 500}},
			totalSize: 1000,
			expected:  []Range{{Offset: 500, Length: 500}},
		},
		{
			name:      "gap in middle",
			ranges:    []Range{{Offset: 0, Length: 300}, {Offset: 700, Length: 300}},
			totalSize: 1000,
			expected:  []Range{{Offset: 300, Length: 400}},
		},
		{
			name: "multiple gaps",
			ranges: []Range{
				{Offset: 100, Length: 100},
				{Offset: 400, Length: 100},
				{Offset: 700, Length: 100},
			},
			totalSize: 1000,
			expected: []Range{
				{Offset: 0, Length: 100},
				{Offset: 200, Length: 200},
				{Offset: 500, Length: 200},
				{Offset: 800, Length: 200},
			},
		},
		{
			name:      "zero total size",
			ranges:    []Range{{Offset: 0, Length: 100}},
			totalSize: 0,
			expected:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := findGaps(tt.ranges, tt.totalSize)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTotalCoverage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ranges   []Range
		expected int64
	}{
		{
			name:     "empty",
			ranges:   nil,
			expected: 0,
		},
		{
			name:     "single range",
			ranges:   []Range{{Offset: 0, Length: 100}},
			expected: 100,
		},
		{
			name: "non-overlapping",
			ranges: []Range{
				{Offset: 0, Length: 100},
				{Offset: 200, Length: 100},
			},
			expected: 200,
		},
		{
			name: "overlapping - merged before counting",
			ranges: []Range{
				{Offset: 0, Length: 100},
				{Offset: 50, Length: 100},
			},
			expected: 150,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := totalCoverage(tt.ranges)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsComplete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ranges    []Range
		totalSize int64
		expected  bool
	}{
		{
			name:      "empty ranges",
			ranges:    nil,
			totalSize: 1000,
			expected:  false,
		},
		{
			name:      "single complete range",
			ranges:    []Range{{Offset: 0, Length: 1000}},
			totalSize: 1000,
			expected:  true,
		},
		{
			name: "multiple ranges covering all",
			ranges: []Range{
				{Offset: 0, Length: 500},
				{Offset: 500, Length: 500},
			},
			totalSize: 1000,
			expected:  true,
		},
		{
			name:      "partial coverage",
			ranges:    []Range{{Offset: 0, Length: 500}},
			totalSize: 1000,
			expected:  false,
		},
		{
			name:      "zero total size",
			ranges:    nil,
			totalSize: 0,
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := isComplete(tt.ranges, tt.totalSize)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContainsRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ranges   []Range
		offset   int64
		length   int64
		expected bool
	}{
		{
			name:     "empty ranges",
			ranges:   nil,
			offset:   0,
			length:   100,
			expected: false,
		},
		{
			name:     "exact match",
			ranges:   []Range{{Offset: 0, Length: 100}},
			offset:   0,
			length:   100,
			expected: true,
		},
		{
			name:     "contained within",
			ranges:   []Range{{Offset: 0, Length: 200}},
			offset:   50,
			length:   50,
			expected: true,
		},
		{
			name:     "extends beyond",
			ranges:   []Range{{Offset: 0, Length: 100}},
			offset:   50,
			length:   100,
			expected: false,
		},
		{
			name:     "completely outside",
			ranges:   []Range{{Offset: 0, Length: 100}},
			offset:   200,
			length:   100,
			expected: false,
		},
		{
			name: "spans multiple non-adjacent",
			ranges: []Range{
				{Offset: 0, Length: 100},
				{Offset: 200, Length: 100},
			},
			offset:   50,
			length:   200,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := containsRange(tt.ranges, tt.offset, tt.length)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRange_End(t *testing.T) {
	t.Parallel()

	r := Range{Offset: 100, Length: 50}
	assert.Equal(t, int64(150), r.End())
}
