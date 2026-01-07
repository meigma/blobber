package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateContentRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		header         string
		expectedOffset int64
		expectedLength int64
		wantErr        bool
		errContains    string
	}{
		{
			name:           "valid with total",
			header:         "bytes 0-99/1000",
			expectedOffset: 0,
			expectedLength: 100,
			wantErr:        false,
		},
		{
			name:           "valid with unknown total",
			header:         "bytes 100-199/*",
			expectedOffset: 100,
			expectedLength: 100,
			wantErr:        false,
		},
		{
			name:           "empty header (accepted)",
			header:         "",
			expectedOffset: 0,
			expectedLength: 100,
			wantErr:        false,
		},
		{
			name:           "offset mismatch",
			header:         "bytes 50-149/1000",
			expectedOffset: 0,
			expectedLength: 100,
			wantErr:        true,
			errContains:    "start offset mismatch",
		},
		{
			name:           "length mismatch",
			header:         "bytes 0-49/1000",
			expectedOffset: 0,
			expectedLength: 100,
			wantErr:        true,
			errContains:    "length mismatch",
		},
		{
			name:           "malformed header",
			header:         "invalid",
			expectedOffset: 0,
			expectedLength: 100,
			wantErr:        true,
			errContains:    "malformed",
		},
		{
			name:           "missing bytes prefix",
			header:         "0-99/1000",
			expectedOffset: 0,
			expectedLength: 100,
			wantErr:        true,
			errContains:    "malformed",
		},
		{
			name:           "large offset and length",
			header:         "bytes 1000000-1999999/5000000",
			expectedOffset: 1000000,
			expectedLength: 1000000,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateContentRange(tt.header, tt.expectedOffset, tt.expectedLength)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
