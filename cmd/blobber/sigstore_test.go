//go:build integration

package main_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/rogpeppe/go-internal/testscript"
	"oras.land/oras-go/v2/registry/remote/credentials"

	"github.com/meigma/blobber"
	"github.com/meigma/blobber/core"
	"github.com/meigma/blobber/internal/registry"
	"github.com/meigma/blobber/internal/testutil/virtualsigstore"
)

// sigstoreTestEnv holds the VirtualSigstore instance for all tests.
// This is created once per test run and reused to ensure consistent trusted root.
var sigstoreTestEnv *virtualsigstore.VirtualSigstore

// initSigstoreEnv initializes the VirtualSigstore environment.
func initSigstoreEnv() error {
	var err error
	sigstoreTestEnv, err = virtualsigstore.New()
	return err
}

// cmdSigstoreGenTrustedRoot generates a trusted root JSON file.
// Usage: sigstore-gen-trusted-root <output_file>
func cmdSigstoreGenTrustedRoot(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("sigstore-gen-trusted-root does not support negation")
	}
	if len(args) != 1 {
		ts.Fatalf("usage: sigstore-gen-trusted-root <output_file>")
	}

	if sigstoreTestEnv == nil {
		ts.Fatalf("sigstore environment not initialized")
	}

	trustedRootJSON, err := sigstoreTestEnv.TrustedRootJSON()
	if err != nil {
		ts.Fatalf("generate trusted root: %v", err)
	}

	// Resolve path relative to work directory
	outputPath := filepath.Join(ts.Getenv("WORK"), args[0])
	ts.Check(ts.Exec("mkdir", "-p", filepath.Dir(outputPath)))

	if err := writeFile(outputPath, trustedRootJSON); err != nil {
		ts.Fatalf("write trusted root: %v", err)
	}
}

// cmdSigstorePushSigned pushes an artifact and signs it with VirtualSigstore.
// Usage: sigstore-push-signed <directory> <reference> <identity> <issuer>
func cmdSigstorePushSigned(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("sigstore-push-signed does not support negation")
	}
	if len(args) != 4 {
		ts.Fatalf("usage: sigstore-push-signed <directory> <reference> <identity> <issuer>")
	}

	if sigstoreTestEnv == nil {
		ts.Fatalf("sigstore environment not initialized")
	}

	dir := filepath.Join(ts.Getenv("WORK"), args[0])
	ref := args[1]
	identity := args[2]
	issuer := args[3]

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create client with insecure for local registry
	client, err := blobber.NewClient(blobber.WithInsecure(true))
	if err != nil {
		ts.Fatalf("create client: %v", err)
	}

	// Push the artifact
	manifestDigest, err := client.Push(ctx, ref, dirFS(dir))
	if err != nil {
		ts.Fatalf("push: %v", err)
	}

	// Fetch manifest bytes for signing
	reg := registry.New(
		registry.WithPlainHTTP(true),
		registry.WithCredentialStore(credentials.NewMemoryStore()),
	)
	manifestBytes, _, err := reg.FetchManifest(ctx, ref)
	if err != nil {
		ts.Fatalf("fetch manifest: %v", err)
	}

	// Sign the manifest using VirtualSigstore
	signedArtifact, err := sigstoreTestEnv.Sign(identity, issuer, manifestBytes)
	if err != nil {
		ts.Fatalf("sign: %v", err)
	}

	// Push the signature as a referrer
	_, err = reg.PushReferrer(ctx, ref, manifestDigest, signedArtifact.BundleJSON, &core.ReferrerPushOptions{
		ArtifactType: blobber.SignatureArtifactType,
		Annotations: map[string]string{
			"org.opencontainers.image.created": time.Now().UTC().Format(time.RFC3339),
		},
	})
	if err != nil {
		ts.Fatalf("push signature: %v", err)
	}

	// Write to testscript stdout
	fmt.Fprintln(ts.Stdout(), manifestDigest)
}

// cmdSigstorePushInvalidSig pushes an artifact with an invalid signature.
// Usage: sigstore-push-invalid-sig <directory> <reference>
func cmdSigstorePushInvalidSig(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("sigstore-push-invalid-sig does not support negation")
	}
	if len(args) != 2 {
		ts.Fatalf("usage: sigstore-push-invalid-sig <directory> <reference>")
	}

	dir := filepath.Join(ts.Getenv("WORK"), args[0])
	ref := args[1]

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create client with insecure for local registry
	client, err := blobber.NewClient(blobber.WithInsecure(true))
	if err != nil {
		ts.Fatalf("create client: %v", err)
	}

	// Push the artifact
	manifestDigest, err := client.Push(ctx, ref, dirFS(dir))
	if err != nil {
		ts.Fatalf("push: %v", err)
	}

	// Push an invalid signature (not a valid bundle)
	reg := registry.New(
		registry.WithPlainHTTP(true),
		registry.WithCredentialStore(credentials.NewMemoryStore()),
	)
	_, err = reg.PushReferrer(ctx, ref, manifestDigest, []byte(`{"invalid": "signature"}`), &core.ReferrerPushOptions{
		ArtifactType: blobber.SignatureArtifactType,
		Annotations: map[string]string{
			"org.opencontainers.image.created": time.Now().UTC().Format(time.RFC3339),
		},
	})
	if err != nil {
		ts.Fatalf("push invalid signature: %v", err)
	}

	// Write to testscript stdout
	fmt.Fprintln(ts.Stdout(), manifestDigest)
}

// cmdSigstorePushWrongIdentity pushes an artifact signed with a different identity.
// Usage: sigstore-push-wrong-identity <directory> <reference>
func cmdSigstorePushWrongIdentity(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("sigstore-push-wrong-identity does not support negation")
	}
	if len(args) != 2 {
		ts.Fatalf("usage: sigstore-push-wrong-identity <directory> <reference>")
	}

	if sigstoreTestEnv == nil {
		ts.Fatalf("sigstore environment not initialized")
	}

	dir := filepath.Join(ts.Getenv("WORK"), args[0])
	ref := args[1]

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create client with insecure for local registry
	client, err := blobber.NewClient(blobber.WithInsecure(true))
	if err != nil {
		ts.Fatalf("create client: %v", err)
	}

	// Push the artifact
	manifestDigest, err := client.Push(ctx, ref, dirFS(dir))
	if err != nil {
		ts.Fatalf("push: %v", err)
	}

	// Fetch manifest bytes for signing
	reg := registry.New(
		registry.WithPlainHTTP(true),
		registry.WithCredentialStore(credentials.NewMemoryStore()),
	)
	manifestBytes, _, err := reg.FetchManifest(ctx, ref)
	if err != nil {
		ts.Fatalf("fetch manifest: %v", err)
	}

	// Sign with a different identity than what will be verified
	signedArtifact, err := sigstoreTestEnv.Sign("different@example.com", "https://different.example.com", manifestBytes)
	if err != nil {
		ts.Fatalf("sign: %v", err)
	}

	// Push the signature as a referrer
	_, err = reg.PushReferrer(ctx, ref, manifestDigest, signedArtifact.BundleJSON, &core.ReferrerPushOptions{
		ArtifactType: blobber.SignatureArtifactType,
		Annotations: map[string]string{
			"org.opencontainers.image.created": time.Now().UTC().Format(time.RFC3339),
		},
	})
	if err != nil {
		ts.Fatalf("push signature: %v", err)
	}

	// Write to testscript stdout
	fmt.Fprintln(ts.Stdout(), manifestDigest)
}

// cmdSigstorePushTampered pushes an artifact, then signs different content.
// Usage: sigstore-push-tampered <directory> <reference> <identity> <issuer>
func cmdSigstorePushTampered(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("sigstore-push-tampered does not support negation")
	}
	if len(args) != 4 {
		ts.Fatalf("usage: sigstore-push-tampered <directory> <reference> <identity> <issuer>")
	}

	if sigstoreTestEnv == nil {
		ts.Fatalf("sigstore environment not initialized")
	}

	dir := filepath.Join(ts.Getenv("WORK"), args[0])
	ref := args[1]
	identity := args[2]
	issuer := args[3]

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create client with insecure for local registry
	client, err := blobber.NewClient(blobber.WithInsecure(true))
	if err != nil {
		ts.Fatalf("create client: %v", err)
	}

	// Push the artifact
	manifestDigest, err := client.Push(ctx, ref, dirFS(dir))
	if err != nil {
		ts.Fatalf("push: %v", err)
	}

	// Sign DIFFERENT content (not the actual manifest)
	tamperedContent := []byte(`{"tampered": "manifest"}`)
	signedArtifact, err := sigstoreTestEnv.Sign(identity, issuer, tamperedContent)
	if err != nil {
		ts.Fatalf("sign: %v", err)
	}

	// Push the signature as a referrer
	reg := registry.New(
		registry.WithPlainHTTP(true),
		registry.WithCredentialStore(credentials.NewMemoryStore()),
	)
	_, err = reg.PushReferrer(ctx, ref, manifestDigest, signedArtifact.BundleJSON, &core.ReferrerPushOptions{
		ArtifactType: blobber.SignatureArtifactType,
		Annotations: map[string]string{
			"org.opencontainers.image.created": time.Now().UTC().Format(time.RFC3339),
		},
	})
	if err != nil {
		ts.Fatalf("push signature: %v", err)
	}

	// Write to testscript stdout
	fmt.Fprintln(ts.Stdout(), manifestDigest)
}

// cmdSigstoreGenKey generates an ECDSA P-256 private key file.
// Usage: sigstore-gen-key <output_file>
func cmdSigstoreGenKey(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("sigstore-gen-key does not support negation")
	}
	if len(args) != 1 {
		ts.Fatalf("usage: sigstore-gen-key <output_file>")
	}

	// Generate ECDSA P-256 key
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		ts.Fatalf("generate key: %v", err)
	}

	// Encode as PKCS8 PEM
	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		ts.Fatalf("marshal key: %v", err)
	}

	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyBytes,
	})

	// Resolve path relative to work directory
	outputPath := filepath.Join(ts.Getenv("WORK"), args[0])
	ts.Check(ts.Exec("mkdir", "-p", filepath.Dir(outputPath)))

	if err := writeFile(outputPath, pemData); err != nil {
		ts.Fatalf("write key file: %v", err)
	}
}

// dirFS returns an fs.FS for the given directory path.
func dirFS(dir string) fs.FS {
	return os.DirFS(dir)
}

// writeFile writes data to a file, creating parent directories if needed.
func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}
