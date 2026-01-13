// Package testutils provides testing utilities for blobber v2 integration tests.
package testutils

import (
	"context"
	"testing"
	"time"
)

// DefaultTimeout is the default timeout for integration test operations.
const DefaultTimeout = 2 * time.Minute

// TestContext returns a context with timeout for test operations.
// The context is cancelled when the test completes.
func TestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	t.Cleanup(cancel)
	return ctx
}

// TestContextWithTimeout returns a context with a custom timeout.
// The context is cancelled when the test completes.
func TestContextWithTimeout(t *testing.T, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}
