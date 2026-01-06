package registry

// DefaultCredentialStore returns a credential store that reads from
// Docker config (~/.docker/config.json) and credential helpers.
func DefaultCredentialStore() (any, error) {
	panic("not implemented")
}

// StaticCredentials returns a credential store with a single static credential.
func StaticCredentials(registry, username, password string) any {
	panic("not implemented")
}
