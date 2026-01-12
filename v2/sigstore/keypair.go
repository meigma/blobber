package sigstore

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"

	protocommon "github.com/sigstore/protobuf-specs/gen/pb-go/common/v1"
	"github.com/sigstore/sigstore-go/pkg/sign"
	"github.com/sigstore/sigstore/pkg/cryptoutils"
	"github.com/sigstore/sigstore/pkg/signature"
)

// StaticKeypair wraps an existing crypto.Signer to implement the sign.Keypair interface.
// Use NewStaticKeypair to create instances.
type StaticKeypair struct {
	privKey    crypto.Signer
	algDetails signature.AlgorithmDetails
	hint       []byte
}

// NewStaticKeypair creates a Keypair from an existing crypto.Signer.
// The key type and algorithm are automatically detected.
func NewStaticKeypair(key crypto.Signer) (*StaticKeypair, error) {
	if key == nil {
		return nil, errors.New("sigstore: nil key")
	}

	algo, err := detectAlgorithm(key)
	if err != nil {
		return nil, err
	}

	algDetails, err := signature.GetAlgorithmDetails(algo)
	if err != nil {
		return nil, fmt.Errorf("sigstore: get algorithm details: %w", err)
	}

	// Generate hint from public key (same as EphemeralKeypair)
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(key.Public())
	if err != nil {
		return nil, fmt.Errorf("sigstore: marshal public key: %w", err)
	}
	hashedBytes := sha256.Sum256(pubKeyBytes)
	hint := []byte(base64.StdEncoding.EncodeToString(hashedBytes[:]))

	return &StaticKeypair{
		privKey:    key,
		algDetails: algDetails,
		hint:       hint,
	}, nil
}

// detectAlgorithm determines the PublicKeyDetails from a crypto.Signer.
func detectAlgorithm(key crypto.Signer) (protocommon.PublicKeyDetails, error) {
	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		return detectECDSAAlgorithm(k)
	case *rsa.PrivateKey:
		return detectRSAAlgorithm(k)
	case ed25519.PrivateKey:
		return protocommon.PublicKeyDetails_PKIX_ED25519, nil
	default:
		return protocommon.PublicKeyDetails_PUBLIC_KEY_DETAILS_UNSPECIFIED,
			fmt.Errorf("sigstore: unsupported key type: %T", key)
	}
}

func detectECDSAAlgorithm(key *ecdsa.PrivateKey) (protocommon.PublicKeyDetails, error) {
	switch key.Curve {
	case elliptic.P256():
		return protocommon.PublicKeyDetails_PKIX_ECDSA_P256_SHA_256, nil
	case elliptic.P384():
		return protocommon.PublicKeyDetails_PKIX_ECDSA_P384_SHA_384, nil
	case elliptic.P521():
		return protocommon.PublicKeyDetails_PKIX_ECDSA_P521_SHA_512, nil
	default:
		return protocommon.PublicKeyDetails_PUBLIC_KEY_DETAILS_UNSPECIFIED,
			fmt.Errorf("sigstore: unsupported ECDSA curve: %s", key.Curve.Params().Name)
	}
}

func detectRSAAlgorithm(key *rsa.PrivateKey) (protocommon.PublicKeyDetails, error) {
	bits := key.N.BitLen()
	switch {
	case bits >= 4096:
		return protocommon.PublicKeyDetails_PKIX_RSA_PKCS1V15_4096_SHA256, nil
	case bits >= 3072:
		return protocommon.PublicKeyDetails_PKIX_RSA_PKCS1V15_3072_SHA256, nil
	case bits >= 2048:
		return protocommon.PublicKeyDetails_PKIX_RSA_PKCS1V15_2048_SHA256, nil
	default:
		return protocommon.PublicKeyDetails_PUBLIC_KEY_DETAILS_UNSPECIFIED,
			fmt.Errorf("sigstore: RSA key size %d bits is too small (minimum 2048)", bits)
	}
}

// ParsePrivateKeyPEM parses a PEM-encoded private key.
// Supports PKCS8, PKCS1 (RSA), and SEC1 (EC) formats.
// Pass nil password for unencrypted keys.
func ParsePrivateKeyPEM(pemData, password []byte) (crypto.Signer, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, errors.New("sigstore: failed to decode PEM block")
	}

	var keyBytes []byte
	var err error

	// Handle encrypted PEM
	//nolint:staticcheck // x509.IsEncryptedPEMBlock is deprecated but still widely used
	if x509.IsEncryptedPEMBlock(block) {
		if password == nil {
			return nil, errors.New("sigstore: encrypted key requires password")
		}
		//nolint:staticcheck // x509.DecryptPEMBlock is deprecated but still widely used
		keyBytes, err = x509.DecryptPEMBlock(block, password)
		if err != nil {
			return nil, fmt.Errorf("sigstore: decrypt PEM block: %w", err)
		}
	} else {
		keyBytes = block.Bytes
	}

	// Parse based on PEM block type
	switch block.Type {
	case "PRIVATE KEY": // PKCS8
		key, parseErr := x509.ParsePKCS8PrivateKey(keyBytes)
		if parseErr != nil {
			return nil, fmt.Errorf("sigstore: parse PKCS8 private key: %w", parseErr)
		}
		signer, ok := key.(crypto.Signer)
		if !ok {
			return nil, fmt.Errorf("sigstore: key type %T does not implement crypto.Signer", key)
		}
		return signer, nil

	case "RSA PRIVATE KEY": // PKCS1
		key, parseErr := x509.ParsePKCS1PrivateKey(keyBytes)
		if parseErr != nil {
			return nil, fmt.Errorf("sigstore: parse PKCS1 private key: %w", parseErr)
		}
		return key, nil

	case "EC PRIVATE KEY": // SEC1
		key, parseErr := x509.ParseECPrivateKey(keyBytes)
		if parseErr != nil {
			return nil, fmt.Errorf("sigstore: parse EC private key: %w", parseErr)
		}
		return key, nil

	default:
		return nil, fmt.Errorf("sigstore: unsupported PEM block type: %s", block.Type)
	}
}

// GetHashAlgorithm returns the hash algorithm to compute the digest to sign.
func (s *StaticKeypair) GetHashAlgorithm() protocommon.HashAlgorithm {
	return s.algDetails.GetProtoHashType()
}

// GetSigningAlgorithm returns the signing algorithm of the key.
func (s *StaticKeypair) GetSigningAlgorithm() protocommon.PublicKeyDetails {
	return s.algDetails.GetSignatureAlgorithm()
}

// GetHint returns the fingerprint of the public key.
func (s *StaticKeypair) GetHint() []byte {
	return s.hint
}

// GetKeyAlgorithm returns the top-level key algorithm.
func (s *StaticKeypair) GetKeyAlgorithm() string {
	switch s.algDetails.GetKeyType() {
	case signature.ECDSA:
		return "ECDSA"
	case signature.RSA:
		return "RSA"
	case signature.ED25519:
		return "ED25519"
	default:
		return ""
	}
}

// GetPublicKey returns the public key.
func (s *StaticKeypair) GetPublicKey() crypto.PublicKey {
	return s.privKey.Public()
}

// GetPublicKeyPem returns the public key in PEM format.
func (s *StaticKeypair) GetPublicKeyPem() (string, error) {
	pubKeyBytes, err := cryptoutils.MarshalPublicKeyToPEM(s.privKey.Public())
	if err != nil {
		return "", err
	}
	return string(pubKeyBytes), nil
}

// SignData returns the signature and the data that was signed.
// For RSA and ECDSA, data is hashed before signing.
// For Ed25519, the raw data is passed to the signer.
func (s *StaticKeypair) SignData(_ context.Context, data []byte) ([]byte, []byte, error) {
	hf := s.algDetails.GetHashType()
	dataToSign := data

	// RSA, ECDSA, and Ed25519ph sign a digest; pure Ed25519 hashes during signing
	if hf != crypto.Hash(0) {
		hasher := hf.New()
		hasher.Write(data)
		dataToSign = hasher.Sum(nil)
	}

	sig, err := s.privKey.Sign(rand.Reader, dataToSign, hf)
	if err != nil {
		return nil, nil, err
	}

	return sig, dataToSign, nil
}

// Ensure StaticKeypair implements sign.Keypair.
var _ sign.Keypair = (*StaticKeypair)(nil)
