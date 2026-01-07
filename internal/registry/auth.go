package registry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

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
	return &dockerHubFallbackStore{store: store}, nil
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

type dockerHubFallbackStore struct {
	store credentials.Store
}

func (s *dockerHubFallbackStore) Get(ctx context.Context, serverAddress string) (auth.Credential, error) {
	cred, err := s.store.Get(ctx, serverAddress)
	if err == nil && !isEmptyCredential(cred) {
		return cred, nil
	}
	for _, alt := range dockerHubFallbacks(serverAddress) {
		if alt == serverAddress {
			continue
		}
		fallbackCred, fallbackErr := s.store.Get(ctx, alt)
		if fallbackErr == nil && !isEmptyCredential(fallbackCred) {
			return fallbackCred, nil
		}
	}
	if err != nil {
		return cred, err
	}
	return cred, nil
}

func (s *dockerHubFallbackStore) Put(ctx context.Context, serverAddress string, cred auth.Credential) error {
	return s.store.Put(ctx, serverAddress, cred)
}

func (s *dockerHubFallbackStore) Delete(ctx context.Context, serverAddress string) error {
	return s.store.Delete(ctx, serverAddress)
}

func dockerHubFallbacks(serverAddress string) []string {
	host := normalizeServerAddress(serverAddress)
	if !isDockerHubHost(host) {
		return nil
	}
	return []string{
		"https://index.docker.io/v1/",
		"index.docker.io",
		"registry-1.docker.io",
		"docker.io",
	}
}

func isDockerHubHost(host string) bool {
	switch host {
	case "docker.io", "registry-1.docker.io", "index.docker.io":
		return true
	default:
		return false
	}
}

func normalizeServerAddress(addr string) string {
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimPrefix(addr, "https://")
	addr, _, _ = strings.Cut(addr, "/")
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

func isEmptyCredential(cred auth.Credential) bool {
	return cred == auth.EmptyCredential ||
		(cred.Username == "" && cred.Password == "" && cred.AccessToken == "" && cred.RefreshToken == "")
}
