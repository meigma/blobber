package registry

import (
	"context"
	"errors"
	"fmt"

	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
)

// DefaultCredentialStore returns a credential store that reads from
// Docker config (~/.docker/config.json) and credential helpers.
func DefaultCredentialStore() (credentials.Store, error) {
	store, err := credentials.NewStoreFromDocker(credentials.StoreOptions{})
	if err != nil {
		return nil, fmt.Errorf("create docker credential store: %w", err)
	}
	return store, nil
}

// StaticCredentials returns a credential store with a single static credential
// for the specified registry.
func StaticCredentials(registry, username, password string) credentials.Store {
	return &staticStore{
		registry: registry,
		cred: auth.Credential{
			Username: username,
			Password: password,
		},
	}
}

// staticStore implements credentials.Store for a single static credential.
type staticStore struct {
	registry string
	cred     auth.Credential
}

// Get retrieves credentials for the given server address.
func (s *staticStore) Get(_ context.Context, serverAddress string) (auth.Credential, error) {
	if serverAddress == s.registry {
		return s.cred, nil
	}
	return auth.EmptyCredential, nil
}

// Put is not supported for static credentials.
func (s *staticStore) Put(_ context.Context, _ string, _ auth.Credential) error {
	return errors.New("static credential store is read-only")
}

// Delete is not supported for static credentials.
func (s *staticStore) Delete(_ context.Context, _ string) error {
	return errors.New("static credential store is read-only")
}
