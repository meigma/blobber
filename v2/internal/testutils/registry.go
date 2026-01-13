package testutils

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Registry holds the running registry container and connection details.
type Registry struct {
	container testcontainers.Container
	Host      string
}

// refCounter provides unique suffixes for test refs within a test run.
var refCounter atomic.Uint64

// StartRegistry starts a distribution/registry:2 container.
// Call this from TestMain. The returned cleanup function terminates the container.
func StartRegistry(ctx context.Context) (*Registry, func(), error) {
	container, err := testcontainers.Run(ctx,
		"registry:2",
		testcontainers.WithExposedPorts("5000/tcp"),
		testcontainers.WithEnv(map[string]string{
			"REGISTRY_STORAGE_DELETE_ENABLED": "true",
		}),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/v2/").
				WithPort("5000/tcp").
				WithStatusCodeMatcher(func(status int) bool {
					return status == 200
				}).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("start registry container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, nil, fmt.Errorf("get container host: %w", err)
	}

	port, err := container.MappedPort(ctx, "5000")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, nil, fmt.Errorf("get container port: %w", err)
	}

	reg := &Registry{
		container: container,
		Host:      host + ":" + port.Port(),
	}

	cleanup := func() {
		_ = container.Terminate(context.Background())
	}

	return reg, cleanup, nil
}

// TestRef generates a unique registry reference for a test.
// The reference includes the test name to aid debugging.
// Example: localhost:32768/TestPush_Success-1/blob:v1
func (r *Registry) TestRef(t *testing.T, suffix string) string {
	t.Helper()

	// Sanitize test name for use in registry path.
	// OCI references must be lowercase and can't contain certain characters.
	name := strings.ToLower(t.Name())
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, " ", "-")

	// Add counter to ensure uniqueness even if test runs multiple times.
	counter := refCounter.Add(1)

	if suffix == "" {
		suffix = "blob"
	}

	return fmt.Sprintf("%s/%s-%d/%s:v1", r.Host, name, counter, suffix)
}
