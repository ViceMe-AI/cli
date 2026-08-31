package commerceartifact

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const signaturePath = "commerce-signature.json"

type Expected struct {
	ArtifactDigest        string
	ArtifactType          string
	WorkID                string
	BindingType           string
	ProductID             string
	StableName            string
	SkillReleaseID        string
	ReleaseVersion        int
	SigningKeyID          string
	EnvelopeDigest        string
	Signature             string
	ManifestDigest        string
	Manifest              Manifest
	SignedEnvelope        Envelope
	MinimumRuntimeVersion string
}

type File struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"sizeBytes"`
}

type Envelope struct {
	SchemaVersion          int    `json:"schemaVersion"`
	ArtifactType           string `json:"artifactType"`
	WorkID                 string `json:"workId"`
	BindingType            string `json:"bindingType"`
	ProductID              string `json:"productId"`
	StableName             string `json:"stableName"`
	SkillReleaseID         string `json:"skillReleaseId"`
	ReleaseVersion         int    `json:"releaseVersion"`
	TemplateVersion        int    `json:"templateVersion"`
	RuntimeProtocolVersion int    `json:"runtimeProtocolVersion"`
	MinimumRuntimeVersion  string `json:"minimumRuntimeVersion"`
	Files                  []File `json:"files"`
}

type Manifest struct {
	SchemaVersion          int      `json:"schemaVersion"`
	PurchaseSkillWorkID    string   `json:"purchaseSkillWorkId"`
	StableName             string   `json:"stableName"`
	SkillReleaseID         string   `json:"skillReleaseId"`
	ReleaseVersion         int      `json:"releaseVersion"`
	CommerceAPIProfile     string   `json:"commerceApiProfile"`
	RuntimeProtocolVersion int      `json:"runtimeProtocolVersion"`
	MinimumRuntimeVersion  string   `json:"minimumRuntimeVersion"`
	GeneratedAt            string   `json:"generatedAt"`
	BindingType            string   `json:"bindingType"`
	ProductID              string   `json:"productId"`
	AllowedProductIDs      []string `json:"allowedProductIds"`
}

type SignatureFile struct {
	SchemaVersion  int      `json:"schemaVersion"`
	Algorithm      string   `json:"algorithm"`
	KeyID          string   `json:"keyId"`
	Envelope       Envelope `json:"envelope"`
	EnvelopeDigest string   `json:"envelopeDigest"`
	Signature      string   `json:"signature"`
}

type Verified struct {
	Signature SignatureFile
	Manifest  Manifest
	Files     map[string][]byte
}

func Verify(artifact []byte, trustedPublicKey string, expected Expected) (Verified, error) {
	if err := validateExpectedDescriptor(expected); err != nil {
		return Verified{}, err
	}
	if len(artifact) == 0 || len(artifact) > 32<<20 {
		return Verified{}, errors.New("COMMERCE_SKILL_ARTIFACT_SIZE_INVALID")
	}
	artifactHash := sha256.Sum256(artifact)
	if hex.EncodeToString(artifactHash[:]) != expected.ArtifactDigest {
		return Verified{}, errors.New("COMMERCE_SKILL_ARTIFACT_DIGEST_INVALID")
	}
	reader, err := zip.NewReader(bytes.NewReader(artifact), int64(len(artifact)))
	if err != nil {
		return Verified{}, errors.New("COMMERCE_SKILL_ARTIFACT_ZIP_INVALID")
	}
	entries := make(map[string]*zip.File, len(reader.File))
	for _, entry := range reader.File {
		if !safePath(entry.Name) || entry.FileInfo().IsDir() || !entry.Mode().IsRegular() {
			return Verified{}, errors.New("COMMERCE_SKILL_ARTIFACT_PATH_INVALID")
		}
		if _, exists := entries[entry.Name]; exists {
			return Verified{}, errors.New("COMMERCE_SKILL_ARTIFACT_FILE_SET_INVALID")
		}
		entries[entry.Name] = entry
	}
	signatureEntry := entries[signaturePath]
	if signatureEntry == nil || signatureEntry.UncompressedSize64 > 256<<10 {
		return Verified{}, errors.New("COMMERCE_SKILL_SIGNATURE_MISSING")
	}
	signatureBody, err := readEntry(signatureEntry, int64(signatureEntry.UncompressedSize64))
	if err != nil {
		return Verified{}, errors.New("COMMERCE_SKILL_SIGNATURE_INVALID")
	}
	var signature SignatureFile
	decoder := json.NewDecoder(bytes.NewReader(signatureBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&signature); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return Verified{}, errors.New("COMMERCE_SKILL_SIGNATURE_INVALID")
	}
	if err := validateSignatureShape(signature); err != nil {
		return Verified{}, err
	}
	canonical, err := canonicalJSON(signature.Envelope)
	if err != nil {
		return Verified{}, errors.New("COMMERCE_SKILL_ENVELOPE_INVALID")
	}
	envelopeHash := sha256.Sum256(canonical)
	if hex.EncodeToString(envelopeHash[:]) != signature.EnvelopeDigest ||
		signature.EnvelopeDigest != expected.EnvelopeDigest {
		return Verified{}, errors.New("COMMERCE_SKILL_ENVELOPE_DIGEST_INVALID")
	}
	if signature.KeyID != expected.SigningKeyID || signature.Signature != expected.Signature {
		return Verified{}, errors.New("COMMERCE_SKILL_SIGNATURE_IDENTITY_MISMATCH")
	}
	publicKey, err := parseEd25519PublicKey(trustedPublicKey)
	if err != nil {
		return Verified{}, errors.New("COMMERCE_SKILL_TRUSTED_KEY_INVALID")
	}
	signatureBytes, err := base64.RawURLEncoding.DecodeString(signature.Signature)
	if err != nil || !ed25519.Verify(publicKey, canonical, signatureBytes) {
		return Verified{}, errors.New("COMMERCE_SKILL_SIGNATURE_INVALID")
	}
	if err := validateIdentity(signature.Envelope, expected); err != nil {
		return Verified{}, err
	}
	descriptorEnvelope, err := canonicalJSON(expected.SignedEnvelope)
	if err != nil || !bytes.Equal(canonical, descriptorEnvelope) {
		return Verified{}, errors.New("COMMERCE_SKILL_IDENTITY_MISMATCH")
	}
	expectedPaths := map[string]struct{}{signaturePath: {}}
	files := make(map[string][]byte, len(signature.Envelope.Files)+1)
	files[signaturePath] = signatureBody
	var total int64
	for _, expectedFile := range signature.Envelope.Files {
		if _, duplicate := expectedPaths[expectedFile.Path]; duplicate {
			return Verified{}, errors.New("COMMERCE_SKILL_ARTIFACT_FILE_SET_INVALID")
		}
		expectedPaths[expectedFile.Path] = struct{}{}
		total += expectedFile.SizeBytes
		if expectedFile.SizeBytes < 0 || total > 32<<20 {
			return Verified{}, errors.New("COMMERCE_SKILL_ARTIFACT_SIZE_INVALID")
		}
		entry := entries[expectedFile.Path]
		if entry == nil || entry.UncompressedSize64 != uint64(expectedFile.SizeBytes) {
			return Verified{}, errors.New("COMMERCE_SKILL_ARTIFACT_FILE_MISSING")
		}
		body, err := readEntry(entry, expectedFile.SizeBytes)
		if err != nil {
			return Verified{}, errors.New("COMMERCE_SKILL_ARTIFACT_FILE_DIGEST_INVALID")
		}
		digest := sha256.Sum256(body)
		if hex.EncodeToString(digest[:]) != expectedFile.SHA256 {
			return Verified{}, errors.New("COMMERCE_SKILL_ARTIFACT_FILE_DIGEST_INVALID")
		}
		files[expectedFile.Path] = body
	}
	if len(entries) != len(expectedPaths) {
		return Verified{}, errors.New("COMMERCE_SKILL_ARTIFACT_FILE_SET_INVALID")
	}
	for name := range entries {
		if _, ok := expectedPaths[name]; !ok {
			return Verified{}, errors.New("COMMERCE_SKILL_ARTIFACT_FILE_SET_INVALID")
		}
	}
	manifest, err := ParseManifest(files["commerce-manifest.json"])
	if err != nil {
		return Verified{}, err
	}
	if err := validateManifestIdentity(manifest, signature.Envelope, expected); err != nil {
		return Verified{}, err
	}
	embeddedManifest, err := canonicalJSON(manifest)
	if err != nil {
		return Verified{}, errors.New("COMMERCE_SKILL_MANIFEST_INVALID")
	}
	descriptorManifest, err := canonicalJSON(expected.Manifest)
	if err != nil || !bytes.Equal(embeddedManifest, descriptorManifest) {
		return Verified{}, errors.New("COMMERCE_SKILL_IDENTITY_MISMATCH")
	}
	return Verified{Signature: signature, Manifest: manifest, Files: files}, nil
}

// ParseManifest validates the exact server-bound Product identity carried by
// commerce-manifest.json. The manifest is independently checked even though
// its file digest is signed: a valid signature must not authorize a manifest
// that disagrees with the descriptor or signed envelope.
func ParseManifest(body []byte) (Manifest, error) {
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return Manifest{}, errors.New("COMMERCE_SKILL_MANIFEST_INVALID")
	}
	if err := validateManifestShape(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// ParseEnvelope strictly decodes the descriptor's signed envelope before any
// artifact is downloaded or installed.
func ParseEnvelope(body []byte) (Envelope, error) {
	var envelope Envelope
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return Envelope{}, errors.New("COMMERCE_SKILL_ENVELOPE_INVALID")
	}
	if err := validateEnvelopeShape(envelope); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

// ParseTrustRing validates the exact public-key representation embedded in an
// official CLI binary. Release automation and runtime key resolution share
// this parser so a value cannot pass the release gate and fail only when a
// buyer installs a signed Commerce Skill.
func ParseTrustRing(encoded string) (map[string]string, error) {
	keys := make(map[string]string)
	if encoded == "" {
		return keys, nil
	}
	for _, item := range strings.Split(encoded, ",") {
		parts := strings.SplitN(item, ":", 2)
		if len(parts) != 2 || !validTrustKeyID(parts[0]) || parts[1] == "" {
			return nil, errors.New("COMMERCE_SKILL_TRUST_RING_INVALID")
		}
		if _, duplicate := keys[parts[0]]; duplicate {
			return nil, errors.New("COMMERCE_SKILL_TRUST_RING_INVALID")
		}
		if _, err := parseEd25519PublicKey(parts[1]); err != nil {
			return nil, errors.New("COMMERCE_SKILL_TRUST_RING_INVALID")
		}
		keys[parts[0]] = parts[1]
	}
	return keys, nil
}

func parseEd25519PublicKey(encoded string) (ed25519.PublicKey, error) {
	publicDER, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || base64.RawURLEncoding.EncodeToString(publicDER) != encoded {
		return nil, errors.New("invalid base64url public key")
	}
	parsed, err := x509.ParsePKIXPublicKey(publicDER)
	if err != nil {
		return nil, err
	}
	publicKey, ok := parsed.(ed25519.PublicKey)
	if !ok || len(publicKey) != ed25519.PublicKeySize {
		return nil, errors.New("public key is not Ed25519")
	}
	return publicKey, nil
}

func validTrustKeyID(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || strings.ContainsRune("._-", character) {
			continue
		}
		return false
	}
	return true
}

func validateSignatureShape(signature SignatureFile) error {
	if signature.SchemaVersion != 1 || signature.Algorithm != "Ed25519" ||
		!validTrustKeyID(signature.KeyID) || !validSHA256(signature.EnvelopeDigest) || signature.Signature == "" {
		return errors.New("COMMERCE_SKILL_SIGNATURE_INVALID")
	}
	return validateEnvelopeShape(signature.Envelope)
}

func validateEnvelopeShape(envelope Envelope) error {
	if envelope.SchemaVersion != 2 || envelope.WorkID == "" || envelope.StableName == "" ||
		envelope.SkillReleaseID == "" || envelope.ReleaseVersion < 1 || envelope.TemplateVersion < 1 ||
		envelope.RuntimeProtocolVersion < 1 || envelope.MinimumRuntimeVersion == "" ||
		len(envelope.MinimumRuntimeVersion) > 32 || len(envelope.Files) < 2 || len(envelope.Files) > 32 ||
		!validUUID(envelope.WorkID) || !validUUID(envelope.ProductID) ||
		!validUUID(envelope.SkillReleaseID) || !validStableName(envelope.StableName) {
		return errors.New("COMMERCE_SKILL_ENVELOPE_INVALID")
	}
	previous := ""
	hasSkill, hasManifest := false, false
	for _, file := range envelope.Files {
		if !safePath(file.Path) || len(file.SHA256) != 64 || file.SizeBytes < 0 ||
			(previous != "" && !commercePathLess(previous, file.Path)) {
			return errors.New("COMMERCE_SKILL_ENVELOPE_INVALID")
		}
		if _, err := hex.DecodeString(file.SHA256); err != nil {
			return errors.New("COMMERCE_SKILL_ENVELOPE_INVALID")
		}
		previous = file.Path
		hasSkill = hasSkill || file.Path == "SKILL.md"
		hasManifest = hasManifest || file.Path == "commerce-manifest.json"
	}
	if !hasSkill || !hasManifest {
		return errors.New("COMMERCE_SKILL_ENVELOPE_INVALID")
	}
	if envelope.ArtifactType != "PRODUCT_PURCHASE" || envelope.BindingType != "DIRECT_PRODUCT" || envelope.ProductID == "" {
		return errors.New("COMMERCE_SKILL_ENVELOPE_INVALID")
	}
	return nil
}

func validateManifestShape(manifest Manifest) error {
	if manifest.SchemaVersion != 2 ||
		!validUUID(manifest.PurchaseSkillWorkID) ||
		!validStableName(manifest.StableName) ||
		!validUUID(manifest.SkillReleaseID) ||
		manifest.ReleaseVersion < 1 ||
		manifest.CommerceAPIProfile != "viceme-commerce-v1" ||
		manifest.RuntimeProtocolVersion < 1 ||
		manifest.MinimumRuntimeVersion == "" || len(manifest.MinimumRuntimeVersion) > 32 ||
		manifest.BindingType != "DIRECT_PRODUCT" || !validUUID(manifest.ProductID) ||
		len(manifest.AllowedProductIDs) != 1 || manifest.AllowedProductIDs[0] != manifest.ProductID {
		return errors.New("COMMERCE_SKILL_MANIFEST_INVALID")
	}
	if _, err := time.Parse(time.RFC3339Nano, manifest.GeneratedAt); err != nil {
		return errors.New("COMMERCE_SKILL_MANIFEST_INVALID")
	}
	return nil
}

func validateExpectedDescriptor(expected Expected) error {
	if !validSHA256(expected.ArtifactDigest) || !validSHA256(expected.ManifestDigest) ||
		!validSHA256(expected.EnvelopeDigest) || expected.Signature == "" ||
		!validTrustKeyID(expected.SigningKeyID) || expected.MinimumRuntimeVersion == "" {
		return errors.New("COMMERCE_SKILL_DESCRIPTOR_INVALID")
	}
	if err := validateEnvelopeShape(expected.SignedEnvelope); err != nil {
		return err
	}
	if err := validateManifestShape(expected.Manifest); err != nil {
		return err
	}
	if err := validateIdentity(expected.SignedEnvelope, expected); err != nil {
		return err
	}
	if err := validateManifestIdentity(expected.Manifest, expected.SignedEnvelope, expected); err != nil {
		return err
	}
	manifestCanonical, err := canonicalJSON(expected.Manifest)
	if err != nil {
		return errors.New("COMMERCE_SKILL_MANIFEST_INVALID")
	}
	manifestHash := sha256.Sum256(manifestCanonical)
	if hex.EncodeToString(manifestHash[:]) != expected.ManifestDigest {
		return errors.New("COMMERCE_SKILL_MANIFEST_DIGEST_INVALID")
	}
	envelopeCanonical, err := canonicalJSON(expected.SignedEnvelope)
	if err != nil {
		return errors.New("COMMERCE_SKILL_ENVELOPE_INVALID")
	}
	envelopeHash := sha256.Sum256(envelopeCanonical)
	if hex.EncodeToString(envelopeHash[:]) != expected.EnvelopeDigest {
		return errors.New("COMMERCE_SKILL_ENVELOPE_DIGEST_INVALID")
	}
	return nil
}

func validateManifestIdentity(manifest Manifest, envelope Envelope, expected Expected) error {
	if manifest.PurchaseSkillWorkID != expected.WorkID || manifest.PurchaseSkillWorkID != envelope.WorkID ||
		manifest.StableName != expected.StableName || manifest.StableName != envelope.StableName ||
		manifest.SkillReleaseID != expected.SkillReleaseID || manifest.SkillReleaseID != envelope.SkillReleaseID ||
		manifest.ReleaseVersion != expected.ReleaseVersion || manifest.ReleaseVersion != envelope.ReleaseVersion ||
		manifest.BindingType != expected.BindingType || manifest.BindingType != envelope.BindingType ||
		manifest.ProductID != expected.ProductID || manifest.ProductID != envelope.ProductID ||
		manifest.RuntimeProtocolVersion != envelope.RuntimeProtocolVersion ||
		manifest.MinimumRuntimeVersion != expected.MinimumRuntimeVersion ||
		manifest.MinimumRuntimeVersion != envelope.MinimumRuntimeVersion {
		return errors.New("COMMERCE_SKILL_IDENTITY_MISMATCH")
	}
	return nil
}

// The API canonicalizes artifact paths with JavaScript localeCompare. Paths
// are restricted to ASCII, so case-folded lexical order reproduces the only
// meaningful difference from Go byte order (notably SKILL.md).
func commercePathLess(left, right string) bool {
	leftFolded, rightFolded := strings.ToLower(left), strings.ToLower(right)
	if leftFolded != rightFolded {
		return leftFolded < rightFolded
	}
	return left < right
}

func validateIdentity(envelope Envelope, expected Expected) error {
	if envelope.ArtifactType != expected.ArtifactType || envelope.WorkID != expected.WorkID ||
		envelope.StableName != expected.StableName ||
		envelope.SkillReleaseID != expected.SkillReleaseID || envelope.ReleaseVersion != expected.ReleaseVersion {
		return errors.New("COMMERCE_SKILL_IDENTITY_MISMATCH")
	}
	if expected.BindingType != "DIRECT_PRODUCT" || envelope.BindingType != expected.BindingType ||
		envelope.ProductID != expected.ProductID {
		return errors.New("COMMERCE_SKILL_IDENTITY_MISMATCH")
	}
	return nil
}

var stableNamePattern = regexp.MustCompile(`^viceme-[a-z0-9]+(?:-[a-z0-9]+)*$`)

func validStableName(value string) bool {
	return len(value) >= 8 && len(value) <= 160 && stableNamePattern.MatchString(value)
}

func validSHA256(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	if len(compact) != 32 {
		return false
	}
	if _, err := hex.DecodeString(compact); err != nil {
		return false
	}
	version := value[14]
	variant := strings.ToLower(value[19:20])[0]
	return version >= '1' && version <= '8' && strings.ContainsRune("89ab", rune(variant))
}

func readEntry(entry *zip.File, size int64) ([]byte, error) {
	reader, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, size+1))
	if err != nil || int64(len(data)) != size {
		return nil, errors.New("entry size mismatch")
	}
	return data, nil
}

func safePath(name string) bool {
	if name == "" || len(name) > 240 || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || path.Clean(name) != name {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	for _, character := range name {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || strings.ContainsRune("._/-", character) {
			continue
		}
		return false
	}
	return true
}

func canonicalJSON(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var generic any
	if err := decoder.Decode(&generic); err != nil {
		return nil, err
	}
	var result bytes.Buffer
	if err := writeCanonical(&result, generic); err != nil {
		return nil, err
	}
	return result.Bytes(), nil
}

func writeCanonical(writer *bytes.Buffer, value any) error {
	switch typed := value.(type) {
	case nil:
		writer.WriteString("null")
	case bool:
		writer.WriteString(strconv.FormatBool(typed))
	case string:
		encoded, _ := json.Marshal(typed)
		writer.Write(encoded)
	case json.Number:
		writer.WriteString(string(typed))
	case []any:
		writer.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				writer.WriteByte(',')
			}
			if err := writeCanonical(writer, item); err != nil {
				return err
			}
		}
		writer.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		writer.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				writer.WriteByte(',')
			}
			encoded, _ := json.Marshal(key)
			writer.Write(encoded)
			writer.WriteByte(':')
			if err := writeCanonical(writer, typed[key]); err != nil {
				return err
			}
		}
		writer.WriteByte('}')
	default:
		return fmt.Errorf("unsupported canonical JSON type %T", value)
	}
	return nil
}
