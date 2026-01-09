package sigstore

import (
	"crypto"
	"log/slog"

	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/sign"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

// SignerOption configures a Signer.
type SignerOption func(*Signer) error

// VerifierOption configures a Verifier.
type VerifierOption func(*Verifier) error

// WithEphemeralKey generates a new ephemeral keypair for signing.
// This is the recommended approach for keyless signing with Fulcio.
func WithEphemeralKey() SignerOption {
	return func(s *Signer) error {
		kp, err := sign.NewEphemeralKeypair(nil)
		if err != nil {
			return err
		}
		s.keypair = kp
		return nil
	}
}

// WithFulcio enables certificate issuance via Fulcio CA for keyless signing.
// The baseURL should be the Fulcio server URL (e.g., "https://fulcio.sigstore.dev").
func WithFulcio(baseURL string) SignerOption {
	return func(s *Signer) error {
		s.opts.CertificateProvider = sign.NewFulcio(&sign.FulcioOptions{
			BaseURL: baseURL,
		})
		return nil
	}
}

// WithRekor enables transparency log recording via Rekor.
// The baseURL should be the Rekor server URL (e.g., "https://rekor.sigstore.dev").
func WithRekor(baseURL string) SignerOption {
	return func(s *Signer) error {
		s.opts.TransparencyLogs = append(s.opts.TransparencyLogs,
			sign.NewRekor(&sign.RekorOptions{BaseURL: baseURL}))
		return nil
	}
}

// WithPrivateKey uses the provided crypto.Signer for signing.
// This is the core option for key-based (non-keyless) signing.
// The key type is automatically detected (ECDSA, RSA, or Ed25519).
func WithPrivateKey(key crypto.Signer) SignerOption {
	return func(s *Signer) error {
		kp, err := NewStaticKeypair(key)
		if err != nil {
			return err
		}
		s.keypair = kp
		return nil
	}
}

// WithPrivateKeyPEM parses a PEM-encoded private key and uses it for signing.
// Pass nil password for unencrypted keys.
// Supports PKCS8, PKCS1 (RSA), and SEC1 (EC) formats.
// This is a convenience wrapper around WithPrivateKey.
func WithPrivateKeyPEM(pemData, password []byte) SignerOption {
	return func(s *Signer) error {
		key, err := ParsePrivateKeyPEM(pemData, password)
		if err != nil {
			return err
		}
		kp, err := NewStaticKeypair(key)
		if err != nil {
			return err
		}
		s.keypair = kp
		return nil
	}
}

// WithTrustedRoot sets a custom trusted root for verification.
func WithTrustedRoot(tr root.TrustedMaterial) VerifierOption {
	return func(v *Verifier) error {
		v.trustedRoot = tr
		return nil
	}
}

// WithTrustedRootFile loads a trusted root from a JSON file.
func WithTrustedRootFile(path string) VerifierOption {
	return func(v *Verifier) error {
		tr, err := root.NewTrustedRootFromPath(path)
		if err != nil {
			return err
		}
		v.trustedRoot = tr
		return nil
	}
}

// WithIdentity requires signatures from a specific OIDC identity.
// The issuer is the OIDC provider URL (e.g., "https://accounts.google.com").
// The subject is the expected identity (e.g., "user@example.com").
func WithIdentity(issuer, subject string) VerifierOption {
	return func(v *Verifier) error {
		id, err := verify.NewShortCertificateIdentity(issuer, "", subject, "")
		if err != nil {
			return err
		}
		v.identity = &id
		return nil
	}
}

// WithLogger sets a custom logger for the verifier.
// This enables logging of warnings (e.g., when no identity is configured).
func WithLogger(logger *slog.Logger) VerifierOption {
	return func(v *Verifier) error {
		v.logger = logger
		return nil
	}
}
