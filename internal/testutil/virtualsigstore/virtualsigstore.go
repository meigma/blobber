// Package virtualsigstore provides test helpers for signing/verification without network.
// It wraps sigstore-go's VirtualSigstore to generate valid bundles and trusted roots
// that work offline for integration testing.
package virtualsigstore

import (
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"unsafe"

	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	protocommon "github.com/sigstore/protobuf-specs/gen/pb-go/common/v1"
	protorekor "github.com/sigstore/protobuf-specs/gen/pb-go/rekor/v1"
	prototrustroot "github.com/sigstore/protobuf-specs/gen/pb-go/trustroot/v1"
	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/testing/ca"
	"github.com/sigstore/sigstore-go/pkg/tlog"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

// VirtualSigstore wraps ca.VirtualSigstore for testing.
type VirtualSigstore struct {
	vs *ca.VirtualSigstore
}

// New creates a new VirtualSigstore for testing.
func New() (*VirtualSigstore, error) {
	vs, err := ca.NewVirtualSigstore()
	if err != nil {
		return nil, fmt.Errorf("create virtual sigstore: %w", err)
	}
	return &VirtualSigstore{vs: vs}, nil
}

// SignedArtifact contains a signed artifact and its bundle.
type SignedArtifact struct {
	Artifact   []byte
	BundleJSON []byte
	Identity   string
	Issuer     string
}

// Sign creates a signature bundle for the given artifact.
func (v *VirtualSigstore) Sign(identity, issuer string, artifact []byte) (*SignedArtifact, error) {
	entity, err := v.vs.Sign(identity, issuer, artifact)
	if err != nil {
		return nil, fmt.Errorf("sign artifact: %w", err)
	}

	// Convert TestEntity to protobuf bundle
	pbBundle, err := testEntityToBundle(entity)
	if err != nil {
		return nil, fmt.Errorf("convert to bundle: %w", err)
	}

	bundleJSON, err := protojson.Marshal(pbBundle)
	if err != nil {
		return nil, fmt.Errorf("marshal bundle: %w", err)
	}

	return &SignedArtifact{
		Artifact:   artifact,
		BundleJSON: bundleJSON,
		Identity:   identity,
		Issuer:     issuer,
	}, nil
}

// TrustedRootJSON returns the trusted root as JSON.
func (v *VirtualSigstore) TrustedRootJSON() ([]byte, error) {
	tr, err := v.buildTrustedRoot()
	if err != nil {
		return nil, fmt.Errorf("build trusted root: %w", err)
	}
	return protojson.Marshal(tr)
}

// TrustedMaterial returns the VirtualSigstore as a TrustedMaterial for verification.
func (v *VirtualSigstore) TrustedMaterial() root.TrustedMaterial {
	return v.vs
}

// buildTrustedRoot constructs a TrustedRoot protobuf from VirtualSigstore.
func (v *VirtualSigstore) buildTrustedRoot() (*prototrustroot.TrustedRoot, error) {
	tr := &prototrustroot.TrustedRoot{
		MediaType: "application/vnd.dev.sigstore.trustedroot+json;version=0.1",
	}

	if err := v.addTlogs(tr); err != nil {
		return nil, err
	}
	if err := v.addCtlogs(tr); err != nil {
		return nil, err
	}
	if err := v.addFulcioCAs(tr); err != nil {
		return nil, err
	}
	if err := v.addTSAs(tr); err != nil {
		return nil, err
	}

	return tr, nil
}

func (v *VirtualSigstore) addTlogs(tr *prototrustroot.TrustedRoot) error {
	for logID, tlog := range v.vs.RekorLogs() {
		pkBytes, err := x509.MarshalPKIXPublicKey(tlog.PublicKey)
		if err != nil {
			return fmt.Errorf("marshal rekor public key: %w", err)
		}
		logIDBytes, err := hex.DecodeString(logID)
		if err != nil {
			return fmt.Errorf("decode log ID: %w", err)
		}
		tr.Tlogs = append(tr.Tlogs, &prototrustroot.TransparencyLogInstance{
			BaseUrl:       tlog.BaseURL,
			HashAlgorithm: protocommon.HashAlgorithm_SHA2_256,
			PublicKey: &protocommon.PublicKey{
				RawBytes:   pkBytes,
				KeyDetails: protocommon.PublicKeyDetails_PKIX_ECDSA_P256_SHA_256,
				ValidFor: &protocommon.TimeRange{
					Start: timestamppb.New(tlog.ValidityPeriodStart),
					End:   timestamppb.New(tlog.ValidityPeriodEnd),
				},
			},
			LogId: &protocommon.LogId{KeyId: logIDBytes},
		})
	}
	return nil
}

func (v *VirtualSigstore) addCtlogs(tr *prototrustroot.TrustedRoot) error {
	for logID, ctlog := range v.vs.CTLogs() {
		pkBytes, err := x509.MarshalPKIXPublicKey(ctlog.PublicKey)
		if err != nil {
			return fmt.Errorf("marshal ctlog public key: %w", err)
		}
		logIDBytes, err := hex.DecodeString(logID)
		if err != nil {
			return fmt.Errorf("decode ct log ID: %w", err)
		}
		tr.Ctlogs = append(tr.Ctlogs, &prototrustroot.TransparencyLogInstance{
			BaseUrl:       ctlog.BaseURL,
			HashAlgorithm: protocommon.HashAlgorithm_SHA2_256,
			PublicKey: &protocommon.PublicKey{
				RawBytes:   pkBytes,
				KeyDetails: protocommon.PublicKeyDetails_PKIX_ECDSA_P256_SHA_256,
				ValidFor: &protocommon.TimeRange{
					Start: timestamppb.New(ctlog.ValidityPeriodStart),
					End:   timestamppb.New(ctlog.ValidityPeriodEnd),
				},
			},
			LogId: &protocommon.LogId{KeyId: logIDBytes},
		})
	}
	return nil
}

func (v *VirtualSigstore) addFulcioCAs(tr *prototrustroot.TrustedRoot) error {
	for _, fca := range v.vs.FulcioCertificateAuthorities() {
		fulcioCA, ok := fca.(*root.FulcioCertificateAuthority)
		if !ok {
			return fmt.Errorf("unexpected Fulcio CA type: %T", fca)
		}
		var certs []*protocommon.X509Certificate
		for _, inter := range fulcioCA.Intermediates {
			certs = append(certs, &protocommon.X509Certificate{RawBytes: inter.Raw})
		}
		certs = append(certs, &protocommon.X509Certificate{RawBytes: fulcioCA.Root.Raw})
		tr.CertificateAuthorities = append(tr.CertificateAuthorities, &prototrustroot.CertificateAuthority{
			Uri: fulcioCA.URI,
			Subject: &protocommon.DistinguishedName{
				Organization: fulcioCA.Root.Subject.Organization[0],
				CommonName:   fulcioCA.Root.Subject.CommonName,
			},
			ValidFor: &protocommon.TimeRange{
				Start: timestamppb.New(fulcioCA.ValidityPeriodStart),
				End:   timestamppb.New(fulcioCA.ValidityPeriodEnd),
			},
			CertChain: &protocommon.X509CertificateChain{Certificates: certs},
		})
	}
	return nil
}

func (v *VirtualSigstore) addTSAs(tr *prototrustroot.TrustedRoot) error {
	for _, tsa := range v.vs.TimestampingAuthorities() {
		tsaCA, ok := tsa.(*root.SigstoreTimestampingAuthority)
		if !ok {
			return fmt.Errorf("unexpected TSA type: %T", tsa)
		}
		var certs []*protocommon.X509Certificate
		if tsaCA.Leaf != nil {
			certs = append(certs, &protocommon.X509Certificate{RawBytes: tsaCA.Leaf.Raw})
		}
		for _, inter := range tsaCA.Intermediates {
			certs = append(certs, &protocommon.X509Certificate{RawBytes: inter.Raw})
		}
		certs = append(certs, &protocommon.X509Certificate{RawBytes: tsaCA.Root.Raw})
		tr.TimestampAuthorities = append(tr.TimestampAuthorities, &prototrustroot.CertificateAuthority{
			Uri: tsaCA.URI,
			Subject: &protocommon.DistinguishedName{
				Organization: tsaCA.Root.Subject.Organization[0],
				CommonName:   tsaCA.Root.Subject.CommonName,
			},
			ValidFor: &protocommon.TimeRange{
				Start: timestamppb.New(tsaCA.ValidityPeriodStart),
				End:   timestamppb.New(tsaCA.ValidityPeriodEnd),
			},
			CertChain: &protocommon.X509CertificateChain{Certificates: certs},
		})
	}
	return nil
}

// testEntityToBundle converts a TestEntity to a protobuf bundle.
// Uses bundle v0.1 format which only requires inclusion promise (SET),
// not inclusion proof (which VirtualSigstore doesn't generate by default).
func testEntityToBundle(entity *ca.TestEntity) (*protobundle.Bundle, error) {
	pb := &protobundle.Bundle{
		MediaType: "application/vnd.dev.sigstore.bundle+json;version=0.1",
	}

	// Get verification content (certificate)
	vc, err := entity.VerificationContent()
	if err != nil {
		return nil, fmt.Errorf("get verification content: %w", err)
	}

	cert, ok := vc.(*bundle.Certificate)
	if !ok {
		return nil, fmt.Errorf("unexpected verification content type: %T", vc)
	}
	pb.VerificationMaterial = &protobundle.VerificationMaterial{
		Content: &protobundle.VerificationMaterial_Certificate{
			Certificate: &protocommon.X509Certificate{
				RawBytes: cert.Certificate().Raw,
			},
		},
	}

	// Add tlog entries
	tlogEntries, err := entity.TlogEntries()
	if err != nil {
		return nil, fmt.Errorf("get tlog entries: %w", err)
	}

	for _, entry := range tlogEntries {
		// Clone the protobuf entry and add KindVersion (missing from VirtualSigstore's NewEntry)
		tle, tleOK := proto.Clone(entry.TransparencyLogEntry()).(*protorekor.TransparencyLogEntry)
		if !tleOK {
			return nil, errors.New("unexpected transparency log entry type")
		}
		if tle.KindVersion == nil {
			// VirtualSigstore uses hashedrekord for Sign()
			tle.KindVersion = &protorekor.KindVersion{
				Kind:    "hashedrekord",
				Version: "0.0.1",
			}
		}

		// VirtualSigstore creates SETs but stores them in a private field that doesn't
		// get populated in TransparencyLogEntry(). Extract using reflection and add
		// to InclusionPromise for bundle v0.1 compatibility.
		if entry.HasInclusionPromise() && tle.InclusionPromise == nil {
			set := getSignedEntryTimestamp(entry)
			if len(set) > 0 {
				tle.InclusionPromise = &protorekor.InclusionPromise{
					SignedEntryTimestamp: set,
				}
			}
		}

		pb.VerificationMaterial.TlogEntries = append(pb.VerificationMaterial.TlogEntries, tle)
	}

	// Add timestamps
	timestamps, err := entity.Timestamps()
	if err != nil {
		return nil, fmt.Errorf("get timestamps: %w", err)
	}

	if len(timestamps) > 0 {
		pb.VerificationMaterial.TimestampVerificationData = &protobundle.TimestampVerificationData{}
		for _, ts := range timestamps {
			pb.VerificationMaterial.TimestampVerificationData.Rfc3161Timestamps = append(
				pb.VerificationMaterial.TimestampVerificationData.Rfc3161Timestamps,
				&protocommon.RFC3161SignedTimestamp{SignedTimestamp: ts},
			)
		}
	}

	// Get signature content
	sigContent, err := entity.SignatureContent()
	if err != nil {
		return nil, fmt.Errorf("get signature content: %w", err)
	}

	msgSig, ok := sigContent.(*bundle.MessageSignature)
	if !ok {
		return nil, fmt.Errorf("unexpected signature content type: %T", sigContent)
	}
	pb.Content = &protobundle.Bundle_MessageSignature{
		MessageSignature: &protocommon.MessageSignature{
			MessageDigest: &protocommon.HashOutput{
				Algorithm: protocommon.HashAlgorithm_SHA2_256,
				Digest:    msgSig.Digest(),
			},
			Signature: msgSig.Signature(),
		},
	}

	return pb, nil
}

// getSignedEntryTimestamp extracts the private signedEntryTimestamp field from a tlog.Entry
// using reflection. This is necessary because VirtualSigstore stores the SET in a private
// field that isn't exposed through TransparencyLogEntry().
func getSignedEntryTimestamp(entry *tlog.Entry) []byte {
	v := reflect.ValueOf(entry).Elem()
	f := v.FieldByName("signedEntryTimestamp")
	if !f.IsValid() || f.IsZero() {
		return nil
	}
	// Use unsafe to access unexported field
	ptr := unsafe.Pointer(f.UnsafeAddr())
	return *(*[]byte)(ptr)
}
