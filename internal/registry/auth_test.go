package registry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/registry/remote/auth"
)

func TestStaticCredentials(t *testing.T) {
	t.Parallel()

	store := StaticCredentials("ghcr.io", "testuser", "testpass")
	require.NotNil(t, store)

	t.Run("returns credentials for matching registry", func(t *testing.T) {
		t.Parallel()

		cred, err := store.Get(context.Background(), "ghcr.io")
		require.NoError(t, err)
		assert.Equal(t, "testuser", cred.Username)
		assert.Equal(t, "testpass", cred.Password)
	})

	t.Run("returns empty credentials for non-matching registry", func(t *testing.T) {
		t.Parallel()

		cred, err := store.Get(context.Background(), "docker.io")
		require.NoError(t, err)
		assert.Equal(t, auth.EmptyCredential, cred)
	})

	t.Run("Put returns error", func(t *testing.T) {
		t.Parallel()

		err := store.Put(context.Background(), "ghcr.io", auth.Credential{
			Username: "other",
			Password: "other",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "read-only")
	})

	t.Run("Delete returns error", func(t *testing.T) {
		t.Parallel()

		err := store.Delete(context.Background(), "ghcr.io")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "read-only")
	})
}

func TestDefaultCredentialStore(t *testing.T) {
	t.Parallel()

	// DefaultCredentialStore reads from Docker config, which may or may not exist.
	// We just verify it doesn't panic and returns a valid store or error.
	store, err := DefaultCredentialStore()

	// Either succeeds with a store, or fails with an error - both are valid
	if err != nil {
		assert.Nil(t, store)
	} else {
		assert.NotNil(t, store)
	}
}
