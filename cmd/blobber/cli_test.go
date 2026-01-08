//go:build integration

package main_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rogpeppe/go-internal/testscript"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/meigma/blobber/cmd/blobber/cli"
)

// registryHost holds the registry URL for all tests (set once in TestMain).
var registryHost string

func TestMain(m *testing.M) {
	// Start registry container before running tests
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	container, host, err := startRegistry(ctx)
	if err != nil {
		panic("failed to start registry: " + err.Error())
	}
	registryHost = host

	// Run tests with blobber command available
	exitCode := testscript.RunMain(m, map[string]func() int{
		"blobber": func() int {
			if err := cli.Execute(); err != nil {
				return 1
			}
			return 0
		},
	})

	// Cleanup container
	if container != nil {
		_ = container.Terminate(context.Background())
	}

	os.Exit(exitCode)
}

func TestCLI(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata/script",
		Setup: func(env *testscript.Env) error {
			env.Setenv("REGISTRY", registryHost)
			// Set XDG paths to the work directory so cache/config
			// operations work (testscript sets HOME=/no-home which is read-only)
			env.Setenv("XDG_CACHE_HOME", env.WorkDir+"/.cache")
			env.Setenv("XDG_CONFIG_HOME", env.WorkDir+"/.config")
			return nil
		},
	})
}

// startRegistry starts a distribution/registry container and returns the host:port.
func startRegistry(ctx context.Context) (testcontainers.Container, string, error) {
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
		return nil, "", err
	}

	host, err := container.Host(ctx)
	if err != nil {
		return container, "", err
	}

	port, err := container.MappedPort(ctx, "5000")
	if err != nil {
		return container, "", err
	}

	return container, host + ":" + port.Port(), nil
}
