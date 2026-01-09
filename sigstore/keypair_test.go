package sigstore

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	protocommon "github.com/sigstore/protobuf-specs/gen/pb-go/common/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStaticKeypair_ECDSA_P256(t *testing.T) {
	t.Parallel()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	kp, err := NewStaticKeypair(key)
	require.NoError(t, err)

	assert.Equal(t, "ECDSA", kp.GetKeyAlgorithm())
	assert.Equal(t, protocommon.PublicKeyDetails_PKIX_ECDSA_P256_SHA_256, kp.GetSigningAlgorithm())
	assert.Equal(t, protocommon.HashAlgorithm_SHA2_256, kp.GetHashAlgorithm())
	assert.NotNil(t, kp.GetPublicKey())
	assert.NotEmpty(t, kp.GetHint())

	pubPEM, err := kp.GetPublicKeyPem()
	require.NoError(t, err)
	assert.Contains(t, pubPEM, "BEGIN PUBLIC KEY")
}

func TestNewStaticKeypair_ECDSA_P384(t *testing.T) {
	t.Parallel()

	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	require.NoError(t, err)

	kp, err := NewStaticKeypair(key)
	require.NoError(t, err)

	assert.Equal(t, "ECDSA", kp.GetKeyAlgorithm())
	assert.Equal(t, protocommon.PublicKeyDetails_PKIX_ECDSA_P384_SHA_384, kp.GetSigningAlgorithm())
}

func TestNewStaticKeypair_ECDSA_P521(t *testing.T) {
	t.Parallel()

	key, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	require.NoError(t, err)

	kp, err := NewStaticKeypair(key)
	require.NoError(t, err)

	assert.Equal(t, "ECDSA", kp.GetKeyAlgorithm())
	assert.Equal(t, protocommon.PublicKeyDetails_PKIX_ECDSA_P521_SHA_512, kp.GetSigningAlgorithm())
}

func TestNewStaticKeypair_RSA_2048(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	kp, err := NewStaticKeypair(key)
	require.NoError(t, err)

	assert.Equal(t, "RSA", kp.GetKeyAlgorithm())
	assert.Equal(t, protocommon.PublicKeyDetails_PKIX_RSA_PKCS1V15_2048_SHA256, kp.GetSigningAlgorithm())
}

func TestNewStaticKeypair_RSA_4096(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 4096)
	require.NoError(t, err)

	kp, err := NewStaticKeypair(key)
	require.NoError(t, err)

	assert.Equal(t, "RSA", kp.GetKeyAlgorithm())
	assert.Equal(t, protocommon.PublicKeyDetails_PKIX_RSA_PKCS1V15_4096_SHA256, kp.GetSigningAlgorithm())
}

func TestNewStaticKeypair_Ed25519(t *testing.T) {
	t.Parallel()

	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	kp, err := NewStaticKeypair(key)
	require.NoError(t, err)

	assert.Equal(t, "ED25519", kp.GetKeyAlgorithm())
	assert.Equal(t, protocommon.PublicKeyDetails_PKIX_ED25519, kp.GetSigningAlgorithm())
}

func TestNewStaticKeypair_NilKey(t *testing.T) {
	t.Parallel()

	_, err := NewStaticKeypair(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil key")
}

func TestStaticKeypair_SignData(t *testing.T) {
	t.Parallel()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	kp, err := NewStaticKeypair(key)
	require.NoError(t, err)

	data := []byte("test data to sign")
	sig, signedData, err := kp.SignData(context.Background(), data)
	require.NoError(t, err)

	assert.NotEmpty(t, sig)
	assert.NotEmpty(t, signedData)
	// signedData should be a hash (32 bytes for SHA-256)
	assert.Len(t, signedData, 32)
}

func TestParsePrivateKeyPEM_PKCS8_ECDSA(t *testing.T) {
	t.Parallel()

	// Generate a key and encode as PKCS8 PEM
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)

	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyBytes,
	})

	parsedKey, err := ParsePrivateKeyPEM(pemData, nil)
	require.NoError(t, err)

	ecKey, ok := parsedKey.(*ecdsa.PrivateKey)
	require.True(t, ok)
	assert.True(t, key.Equal(ecKey))
}

func TestParsePrivateKeyPEM_PKCS8_RSA(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)

	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyBytes,
	})

	parsedKey, err := ParsePrivateKeyPEM(pemData, nil)
	require.NoError(t, err)

	rsaKey, ok := parsedKey.(*rsa.PrivateKey)
	require.True(t, ok)
	assert.True(t, key.Equal(rsaKey))
}

func TestParsePrivateKeyPEM_PKCS8_Ed25519(t *testing.T) {
	t.Parallel()

	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)

	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyBytes,
	})

	parsedKey, err := ParsePrivateKeyPEM(pemData, nil)
	require.NoError(t, err)

	edKey, ok := parsedKey.(ed25519.PrivateKey)
	require.True(t, ok)
	assert.True(t, key.Equal(edKey))
}

func TestParsePrivateKeyPEM_PKCS1_RSA(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	keyBytes := x509.MarshalPKCS1PrivateKey(key)
	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: keyBytes,
	})

	parsedKey, err := ParsePrivateKeyPEM(pemData, nil)
	require.NoError(t, err)

	rsaKey, ok := parsedKey.(*rsa.PrivateKey)
	require.True(t, ok)
	assert.True(t, key.Equal(rsaKey))
}

func TestParsePrivateKeyPEM_SEC1_EC(t *testing.T) {
	t.Parallel()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	keyBytes, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})

	parsedKey, err := ParsePrivateKeyPEM(pemData, nil)
	require.NoError(t, err)

	ecKey, ok := parsedKey.(*ecdsa.PrivateKey)
	require.True(t, ok)
	assert.True(t, key.Equal(ecKey))
}

func TestParsePrivateKeyPEM_InvalidPEM(t *testing.T) {
	t.Parallel()

	_, err := ParsePrivateKeyPEM([]byte("not a pem"), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode PEM")
}

func TestParsePrivateKeyPEM_UnsupportedType(t *testing.T) {
	t.Parallel()

	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: []byte("fake cert"),
	})

	_, err := ParsePrivateKeyPEM(pemData, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported PEM block type")
}

func TestWithPrivateKey(t *testing.T) {
	t.Parallel()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	signer, err := NewSigner(WithPrivateKey(key))
	require.NoError(t, err)
	assert.NotNil(t, signer)
}

func TestWithPrivateKeyPEM(t *testing.T) {
	t.Parallel()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)

	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyBytes,
	})

	signer, err := NewSigner(WithPrivateKeyPEM(pemData, nil))
	require.NoError(t, err)
	assert.NotNil(t, signer)
}

func TestWithPrivateKeyPEM_InvalidPEM(t *testing.T) {
	t.Parallel()

	_, err := NewSigner(WithPrivateKeyPEM([]byte("not a pem"), nil))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode PEM")
}
