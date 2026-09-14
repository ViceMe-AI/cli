package command

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"github.com/ViceMe-AI/cli/internal/api"
	"strings"
	"testing"
)

func TestReplicaRedistributionSignedLicenseRoundTrip(t *testing.T) {
	signer := newReplicaTestSigner(t, "redistribution-test")
	trustReplicaTestSigner(t, signer)
	id := "11111111-1111-4111-8111-111111111111"
	versionID := "22222222-2222-4222-8222-222222222222"
	digest := strings.Repeat("a", 64)
	license := signedReplicaTestLicense(t, signer, id, versionID, 1, "VMO-REDISTRIBUTION", digest)
	license.Claims.Redistribution = &api.WebsiteReplicaRedistributionLicense{Allowed: true, Scope: "VICEME", UnauthorizedRedistributionProhibited: true}
	var canonical map[string]any
	raw := mustJSON(t, license.Claims)
	if err := json.Unmarshal(raw, &canonical); err != nil {
		t.Fatal(err)
	}
	encoded := mustJSON(t, canonical)
	license.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(signer.privateKey, encoded))
	download := api.WebsiteReplicaDownload{ReplicaID: id, VersionID: versionID, Version: 1, ArtifactDigest: digest, License: mustJSON(t, license)}
	claims, err := verifiedReplicaLicenseClaims(context.Background(), &Runtime{}, download, "VMO-REDISTRIBUTION")
	if err != nil || claims.Redistribution == nil || !claims.Redistribution.Allowed {
		t.Fatalf("signed grant rejected: %v", err)
	}
	license.Claims.Redistribution.Allowed = false
	download.License = mustJSON(t, license)
	if _, err := verifiedReplicaLicenseClaims(context.Background(), &Runtime{}, download, "VMO-REDISTRIBUTION"); err == nil {
		t.Fatal("tampered grant signature accepted")
	}
}

func TestReplicaRedistributionSourceCannotChangeConfirmation(t *testing.T) {
	id := "10101010-1010-4010-8010-101010101010"
	request := api.CreateWebsiteReplicaPublicationRequest{SourceEntitlementID: id}
	confirmation := api.WebsiteReplicaPublicationConfirmationChallenge{Review: api.WebsiteReplicaPublicationReview{SourceEntitlementID: id}}
	if !replicaConfirmationMatchesRequest(confirmation, request) {
		t.Fatal("matching source was rejected")
	}
	confirmation.Review.SourceEntitlementID = "20202020-2020-4020-8020-202020202020"
	if replicaConfirmationMatchesRequest(confirmation, request) {
		t.Fatal("changed source was accepted")
	}
	confirmation.Review.SourceEntitlementID = ""
	if replicaConfirmationMatchesRequest(confirmation, request) {
		t.Fatal("dropped source was accepted")
	}
}

func TestReplicaRedistributionInvalidSourceStopsBeforeNetwork(t *testing.T) {
	if validateReplicaPublishOptions(replicaPublishOptions{SourceEntitlementID: "not-a-uuid"}) == nil {
		t.Fatal("invalid source accepted")
	}
}
