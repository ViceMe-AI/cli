package commerceartifact

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestVerifyChecksSignatureIdentityAndExactFileSet(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{
		SchemaVersion: 2, PurchaseSkillWorkID: "11111111-1111-4111-8111-111111111111",
		StableName: "viceme-buy-photo", SkillReleaseID: "33333333-3333-4333-8333-333333333333",
		ReleaseVersion: 1, CommerceAPIProfile: "viceme-commerce-v1", RuntimeProtocolVersion: 1,
		MinimumRuntimeVersion: "1.0.0", GeneratedAt: "2026-08-25T00:00:00.000Z",
		BindingType: "DIRECT_PRODUCT", ProductID: "22222222-2222-4222-8222-222222222222",
		AllowedProductIDs: []string{"22222222-2222-4222-8222-222222222222"},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"SKILL.md":               []byte("---\nname: viceme-buy-photo\ndescription: \"Buy\"\n---\n"),
		"commerce-manifest.json": append(manifestJSON, '\n'),
	}
	envelope := Envelope{
		SchemaVersion: 2, ArtifactType: "PRODUCT_PURCHASE", BindingType: "DIRECT_PRODUCT",
		WorkID:     "11111111-1111-4111-8111-111111111111",
		ProductID:  "22222222-2222-4222-8222-222222222222",
		StableName: "viceme-buy-photo", SkillReleaseID: "33333333-3333-4333-8333-333333333333",
		ReleaseVersion: 1, TemplateVersion: 1, RuntimeProtocolVersion: 1,
		MinimumRuntimeVersion: "1.0.0",
	}
	for _, name := range []string{"SKILL.md", "commerce-manifest.json"} {
		digest := sha256.Sum256(files[name])
		envelope.Files = append(envelope.Files, File{Path: name, SHA256: hex.EncodeToString(digest[:]), SizeBytes: int64(len(files[name]))})
	}
	// Platform canonical ordering is lexical, not insertion ordering.
	envelope.Files[0], envelope.Files[1] = envelope.Files[1], envelope.Files[0]
	canonical, err := canonicalJSON(envelope)
	if err != nil {
		t.Fatal(err)
	}
	envelopeDigest := sha256.Sum256(canonical)
	signature := ed25519.Sign(privateKey, canonical)
	signatureFile := SignatureFile{
		SchemaVersion: 1, Algorithm: "Ed25519", KeyID: "test-v1", Envelope: envelope,
		EnvelopeDigest: hex.EncodeToString(envelopeDigest[:]),
		Signature:      base64.RawURLEncoding.EncodeToString(signature),
	}
	artifact := buildTestArtifact(t, files, signatureFile, nil)
	artifactDigest := sha256.Sum256(artifact)
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	expected := Expected{
		ArtifactDigest: hex.EncodeToString(artifactDigest[:]), ArtifactType: "PRODUCT_PURCHASE",
		WorkID: envelope.WorkID, BindingType: envelope.BindingType, ProductID: envelope.ProductID, StableName: envelope.StableName,
		SkillReleaseID: envelope.SkillReleaseID, ReleaseVersion: envelope.ReleaseVersion,
		SigningKeyID: signatureFile.KeyID, EnvelopeDigest: signatureFile.EnvelopeDigest,
		Signature: signatureFile.Signature, Manifest: manifest, SignedEnvelope: envelope,
		MinimumRuntimeVersion: manifest.MinimumRuntimeVersion,
	}
	manifestCanonical, err := canonicalJSON(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest := sha256.Sum256(manifestCanonical)
	expected.ManifestDigest = hex.EncodeToString(manifestDigest[:])
	verified, err := Verify(artifact, base64.RawURLEncoding.EncodeToString(publicDER), expected)
	if err != nil {
		t.Fatalf("valid signed artifact failed: %v", err)
	}
	if string(verified.Files["SKILL.md"]) != string(files["SKILL.md"]) {
		t.Fatal("verified payload changed")
	}

	wrongWork := expected
	wrongWork.WorkID = "44444444-4444-4444-8444-444444444444"
	if _, err := Verify(artifact, base64.RawURLEncoding.EncodeToString(publicDER), wrongWork); err == nil || err.Error() != "COMMERCE_SKILL_IDENTITY_MISMATCH" {
		t.Fatalf("cross-Work signed artifact was accepted: %v", err)
	}

	tamperedFiles := cloneFiles(files)
	tamperedFiles["SKILL.md"] = []byte("tampered")
	tampered := buildTestArtifact(t, tamperedFiles, signatureFile, nil)
	tamperedDigest := sha256.Sum256(tampered)
	tamperedExpected := expected
	tamperedExpected.ArtifactDigest = hex.EncodeToString(tamperedDigest[:])
	if _, err := Verify(tampered, base64.RawURLEncoding.EncodeToString(publicDER), tamperedExpected); err == nil {
		t.Fatal("tampered signed file was accepted")
	}

	extra := buildTestArtifact(t, files, signatureFile, map[string][]byte{"extra.md": []byte("unsigned")})
	extraDigest := sha256.Sum256(extra)
	extraExpected := expected
	extraExpected.ArtifactDigest = hex.EncodeToString(extraDigest[:])
	if _, err := Verify(extra, base64.RawURLEncoding.EncodeToString(publicDER), extraExpected); err == nil {
		t.Fatal("unsigned extra file was accepted")
	}

	// Even a freshly signed envelope must not authorize a manifest whose
	// server-bound Product identity differs from the descriptor/envelope.
	wrongManifest := manifest
	wrongManifest.ProductID = "44444444-4444-4444-8444-444444444444"
	wrongManifest.AllowedProductIDs = []string{wrongManifest.ProductID}
	wrongManifestJSON, err := json.Marshal(wrongManifest)
	if err != nil {
		t.Fatal(err)
	}
	resignedFiles := cloneFiles(files)
	resignedFiles["commerce-manifest.json"] = append(wrongManifestJSON, '\n')
	resignedEnvelope := envelope
	resignedEnvelope.Files = append([]File(nil), envelope.Files...)
	for index := range resignedEnvelope.Files {
		if resignedEnvelope.Files[index].Path == "commerce-manifest.json" {
			digest := sha256.Sum256(resignedFiles["commerce-manifest.json"])
			resignedEnvelope.Files[index].SHA256 = hex.EncodeToString(digest[:])
			resignedEnvelope.Files[index].SizeBytes = int64(len(resignedFiles["commerce-manifest.json"]))
		}
	}
	resignedCanonical, err := canonicalJSON(resignedEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	resignedEnvelopeDigest := sha256.Sum256(resignedCanonical)
	resignedSignature := SignatureFile{
		SchemaVersion: 1, Algorithm: "Ed25519", KeyID: signatureFile.KeyID,
		Envelope: resignedEnvelope, EnvelopeDigest: hex.EncodeToString(resignedEnvelopeDigest[:]),
		Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, resignedCanonical)),
	}
	resignedArtifact := buildTestArtifact(t, resignedFiles, resignedSignature, nil)
	resignedArtifactDigest := sha256.Sum256(resignedArtifact)
	resignedExpected := expected
	resignedExpected.ArtifactDigest = hex.EncodeToString(resignedArtifactDigest[:])
	resignedExpected.SignedEnvelope = resignedEnvelope
	resignedExpected.EnvelopeDigest = resignedSignature.EnvelopeDigest
	resignedExpected.Signature = resignedSignature.Signature
	if _, err := Verify(resignedArtifact, base64.RawURLEncoding.EncodeToString(publicDER), resignedExpected); err == nil || err.Error() != "COMMERCE_SKILL_IDENTITY_MISMATCH" {
		t.Fatalf("signed cross-Product manifest was accepted: %v", err)
	}
}

func TestManifestAndEnvelopeParsingAreStrict(t *testing.T) {
	validManifest := []byte(`{"schemaVersion":2,"purchaseSkillWorkId":"11111111-1111-4111-8111-111111111111","stableName":"viceme-buy-photo","skillReleaseId":"33333333-3333-4333-8333-333333333333","releaseVersion":1,"commerceApiProfile":"viceme-commerce-v1","runtimeProtocolVersion":1,"minimumRuntimeVersion":"1.0.0","generatedAt":"2026-08-25T00:00:00.000Z","bindingType":"DIRECT_PRODUCT","productId":"22222222-2222-4222-8222-222222222222","allowedProductIds":["22222222-2222-4222-8222-222222222222"]}`)
	if _, err := ParseManifest(validManifest); err != nil {
		t.Fatalf("valid manifest was rejected: %v", err)
	}
	for _, invalid := range [][]byte{
		append(bytes.Clone(validManifest[:len(validManifest)-1]), []byte(`,"unknown":true}`)...),
		append(bytes.Clone(validManifest), []byte(` {}`)...),
	} {
		if _, err := ParseManifest(invalid); err == nil {
			t.Fatalf("non-strict manifest was accepted: %s", invalid)
		}
	}

	validEnvelope := Envelope{
		SchemaVersion: 2, ArtifactType: "PRODUCT_PURCHASE", WorkID: "11111111-1111-4111-8111-111111111111",
		BindingType: "DIRECT_PRODUCT", ProductID: "22222222-2222-4222-8222-222222222222",
		StableName: "viceme-buy-photo", SkillReleaseID: "33333333-3333-4333-8333-333333333333",
		ReleaseVersion: 1, TemplateVersion: 1, RuntimeProtocolVersion: 1, MinimumRuntimeVersion: "1.0.0",
		Files: []File{
			{Path: "commerce-manifest.json", SHA256: strings.Repeat("a", 64), SizeBytes: 1},
			{Path: "SKILL.md", SHA256: strings.Repeat("b", 64), SizeBytes: 1},
		},
	}
	envelopeJSON, err := json.Marshal(validEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseEnvelope(envelopeJSON); err != nil {
		t.Fatalf("valid envelope was rejected: %v", err)
	}
	withUnknown := append(bytes.Clone(envelopeJSON[:len(envelopeJSON)-1]), []byte(`,"unknown":true}`)...)
	if _, err := ParseEnvelope(withUnknown); err == nil {
		t.Fatal("envelope with an unknown field was accepted")
	}
}

func TestParseTrustRingRequiresUniqueEd25519SPKIKeys(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(publicDER)
	keys, err := ParseTrustRing("release-v1:" + encoded)
	if err != nil || keys["release-v1"] != encoded {
		t.Fatalf("valid Ed25519 trust ring failed: keys=%v err=%v", keys, err)
	}

	invalid := []string{
		"release-v1:A",
		"release-v1:" + base64.RawURLEncoding.EncodeToString([]byte("not-der")),
		"release-v1:" + encoded + ",release-v1:" + encoded,
		"bad key:" + encoded,
	}
	for _, ring := range invalid {
		if _, err := ParseTrustRing(ring); err == nil {
			t.Fatalf("invalid trust ring was accepted: %q", ring)
		}
	}
}

func buildTestArtifact(t *testing.T, files map[string][]byte, signature SignatureFile, extra map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	write := func(name string, body []byte) {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"SKILL.md", "commerce-manifest.json"} {
		write(name, files[name])
	}
	for name, body := range extra {
		write(name, body)
	}
	signatureJSON, err := json.Marshal(signature)
	if err != nil {
		t.Fatal(err)
	}
	write(signaturePath, signatureJSON)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func cloneFiles(input map[string][]byte) map[string][]byte {
	result := make(map[string][]byte, len(input))
	for name, body := range input {
		result[name] = bytes.Clone(body)
	}
	return result
}
