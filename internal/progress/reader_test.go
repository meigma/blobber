package progress

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReader_TracksProgress(t *testing.T) {
	t.Parallel()

	data := []byte("hello world")
	r := bytes.NewReader(data)

	var events []struct {
		transferred int64
		total       int64
	}
	pr := NewReader(r, int64(len(data)), func(transferred, total int64) {
		events = append(events, struct {
			transferred int64
			total       int64
		}{transferred, total})
	})

	buf := make([]byte, 5)
	n, err := pr.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Len(t, events, 1)
	assert.Equal(t, int64(5), events[0].transferred)
	assert.Equal(t, int64(11), events[0].total)

	// Read remaining
	_, err = io.ReadAll(pr)
	require.NoError(t, err)
	assert.Equal(t, int64(11), events[len(events)-1].transferred)
}

func TestReader_NilCallback(t *testing.T) {
	t.Parallel()

	data := []byte("hello")
	r := bytes.NewReader(data)

	pr := NewReader(r, int64(len(data)), nil)

	buf, err := io.ReadAll(pr)
	require.NoError(t, err)
	assert.Equal(t, data, buf)
}

func TestReader_CloseClosesUnderlying(t *testing.T) {
	t.Parallel()

	closed := false
	r := &mockCloser{
		Reader: bytes.NewReader([]byte("test")),
		onClose: func() error {
			closed = true
			return nil
		},
	}

	pr := NewReader(r, 4, nil)
	err := pr.Close()
	require.NoError(t, err)
	assert.True(t, closed)
}

func TestReader_CloseNonCloser(t *testing.T) {
	t.Parallel()

	// bytes.Reader doesn't implement io.Closer
	r := bytes.NewReader([]byte("test"))

	pr := NewReader(r, 4, nil)
	err := pr.Close()
	require.NoError(t, err) // Should not error
}

type mockCloser struct {
	io.Reader
	onClose func() error
}

func (m *mockCloser) Close() error {
	return m.onClose()
}
