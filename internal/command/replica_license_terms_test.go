package command

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
)

func TestReplicaLicenseTermsCompatibility(t *testing.T) {
	signer := newReplicaTestSigner(t, "terms-test")
	trustReplicaTestSigner(t, signer)
	const id = "11111111-1111-4111-8111-111111111111"
	const version = "22222222-2222-4222-8222-222222222222"
	digest := strings.Repeat("a", 64)
	for _, test := range []struct {
		name, terms, mutation, code string
		redistribute                bool
	}{
		{name: "legacy", terms: "website-replica-license/v1"},
		{name: "traceable", terms: "website-replica-license/v3"},
		{name: "traceable redistribution", terms: "website-replica-license/v3", redistribute: true},
		{name: "unknown terms", terms: "website-replica-license/v99", code: "REPLICA_LICENSE_TERMS_UNSUPPORTED"},
		{name: "wrong order", terms: "website-replica-license/v3", mutation: "order", code: "REPLICA_LICENSE_IDENTITY_MISMATCH"},
		{name: "wrong digest", terms: "website-replica-license/v3", mutation: "digest", code: "REPLICA_LICENSE_IDENTITY_MISMATCH"},
		{name: "tampered claims", terms: "website-replica-license/v3", mutation: "signature", code: "REPLICA_LICENSE_SIGNATURE_INVALID"},
	} {
		t.Run(test.name, func(t *testing.T) {
			license := signedReplicaTestLicense(t, signer, id, version, 1, "VMO-TERMS", digest)
			license.Claims.LicenseTermsVersion = test.terms
			if test.terms == "website-replica-license/v3" {
				license.Claims.BaseArtifactDigest = strings.Repeat("b", 64)
				license.Claims.Traceability = &api.WebsiteReplicaTraceabilityLicense{Enabled: true, Purpose: "COPYRIGHT_ORDER_ATTRIBUTION", RuntimeReporting: false, RemovalOrEvasionProhibited: true, ResaleProhibited: !test.redistribute}
			}
			if test.redistribute {
				license.Claims.Redistribution = &api.WebsiteReplicaRedistributionLicense{Allowed: true, Scope: "VICEME", UnauthorizedRedistributionProhibited: true}
			}
			switch test.mutation {
			case "order":
				license.Claims.OrderNo = "VMO-OTHER"
			case "digest":
				license.Claims.ArtifactDigest = strings.Repeat("c", 64)
			}
			var canonical map[string]any
			if err := json.Unmarshal(mustJSON(t, license.Claims), &canonical); err != nil {
				t.Fatal(err)
			}
			license.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(signer.privateKey, mustJSON(t, canonical)))
			if test.mutation == "signature" {
				license.Claims.IssuedAt = "2026-09-02T00:00:00.000Z"
			}
			download := api.WebsiteReplicaDownload{ReplicaID: id, VersionID: version, Version: 1, ArtifactDigest: digest, License: mustJSON(t, license)}
			_, err := verifiedReplicaLicenseClaims(context.Background(), &Runtime{}, download, "VMO-TERMS")
			if test.code == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || output.AsError(err).Subtype != test.code {
				t.Fatalf("expected %s, got %v", test.code, err)
			}
		})
	}
}
