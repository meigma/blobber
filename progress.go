package blobber

// ProgressEvent represents a progress update during push/pull operations.
type ProgressEvent struct {
	// Operation identifies the operation type ("push" or "pull").
	Operation string
	// BytesTransferred is the cumulative bytes transferred so far.
	BytesTransferred int64
	// TotalBytes is the total expected size.
	TotalBytes int64
}

// ProgressCallback is called during push/pull operations to report progress.
// Implementations should be efficient as this may be called frequently.
type ProgressCallback func(event ProgressEvent)
