//go:build integration

// Package integration contains end-to-end integration tests for blobber v2.
//
// Tests in this package require Docker and are excluded from normal test runs.
// Run with: go test -tags=integration ./integration/...
package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/meigma/blobber/v2/internal/testutils"
)

// registry is the shared registry container for all integration tests.
var registry *testutils.Registry

// TestMain manages the lifecycle of the shared registry container.
func TestMain(m *testing.M) {
	// Create a context with timeout for container startup.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Start the registry container.
	var cleanup func()
	var err error
	registry, cleanup, err = testutils.StartRegistry(ctx)
	if err != nil {
		os.Stderr.WriteString("failed to start registry: " + err.Error() + "\n")
		os.Exit(1)
	}

	// Run all tests.
	code := m.Run()

	// Clean up the container.
	cleanup()

	os.Exit(code)
}
