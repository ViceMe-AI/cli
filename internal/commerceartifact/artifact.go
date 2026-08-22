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
	"sort"
	"strconv"
	"strings"
)

const signaturePath = "commerce-signature.json"

type Expected struct {
	ArtifactDigest     string
	ArtifactType       string
	BindingType        string
	ProductID          string
	ProductBlueprintID string
	StableName         string
	SkillReleaseID     string
	ReleaseVersion     int
	SigningKeyID       string
	EnvelopeDigest     string
	Signature          string
}

type File struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"sizeBytes"`
}

type Envelope struct {
	SchemaVersion          int     `json:"schemaVersion"`
	ArtifactType           string  `json:"artifactType"`
	WorkID                 string  `json:"workId"`
	BindingType            *string `json:"bindingType"`
	ProductID              *string `json:"productId"`
	ProductBlueprintID     *string `json:"productBlueprintId"`
	StableName             string  `json:"stableName"`
	SkillReleaseID         string  `json:"skillReleaseId"`
	ReleaseVersion         int     `json:"releaseVersion"`
	TemplateVersion        int     `json:"templateVersion"`
	RuntimeProtocolVersion int     `json:"runtimeProtocolVersion"`
	MinimumRuntimeVersion  string  `json:"minimumRuntimeVersion"`
	Files                  []File  `json:"files"`
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
	Files     map[string][]byte
}

func Verify(artifact []byte, trustedPublicKey string, expected Expected) (Verified, error) {
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
	return Verified{Signature: signature, Files: files}, nil
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
		signature.KeyID == "" || signature.EnvelopeDigest == "" || signature.Signature == "" {
		return errors.New("COMMERCE_SKILL_SIGNATURE_INVALID")
	}
	envelope := signature.Envelope
	if envelope.SchemaVersion != 2 || envelope.WorkID == "" || envelope.StableName == "" ||
		envelope.SkillReleaseID == "" || envelope.ReleaseVersion < 1 || envelope.TemplateVersion < 1 ||
		envelope.RuntimeProtocolVersion < 1 || envelope.MinimumRuntimeVersion == "" ||
		len(envelope.Files) < 2 || len(envelope.Files) > 32 {
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
	if envelope.ArtifactType == "PRODUCT_PURCHASE" {
		if envelope.BindingType == nil {
			return errors.New("COMMERCE_SKILL_ENVELOPE_INVALID")
		}
		switch *envelope.BindingType {
		case "DIRECT_PRODUCT":
			if envelope.ProductID == nil || envelope.ProductBlueprintID != nil {
				return errors.New("COMMERCE_SKILL_ENVELOPE_INVALID")
			}
		case "PRODUCT_BLUEPRINT":
			if envelope.ProductBlueprintID == nil || envelope.ProductID != nil {
				return errors.New("COMMERCE_SKILL_ENVELOPE_INVALID")
			}
		default:
			return errors.New("COMMERCE_SKILL_ENVELOPE_INVALID")
		}
	} else if envelope.BindingType != nil || envelope.ProductID != nil || envelope.ProductBlueprintID != nil {
		return errors.New("COMMERCE_SKILL_ENVELOPE_INVALID")
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
	if envelope.ArtifactType != expected.ArtifactType || envelope.StableName != expected.StableName ||
		envelope.SkillReleaseID != expected.SkillReleaseID || envelope.ReleaseVersion != expected.ReleaseVersion {
		return errors.New("COMMERCE_SKILL_IDENTITY_MISMATCH")
	}
	if expected.BindingType == "DIRECT_PRODUCT" {
		if envelope.BindingType == nil || *envelope.BindingType != expected.BindingType ||
			envelope.ProductID == nil || *envelope.ProductID != expected.ProductID ||
			envelope.ProductBlueprintID != nil {
			return errors.New("COMMERCE_SKILL_IDENTITY_MISMATCH")
		}
	} else if expected.BindingType == "PRODUCT_BLUEPRINT" {
		if envelope.BindingType == nil || *envelope.BindingType != expected.BindingType ||
			envelope.ProductBlueprintID == nil || *envelope.ProductBlueprintID != expected.ProductBlueprintID ||
			envelope.ProductID != nil {
			return errors.New("COMMERCE_SKILL_IDENTITY_MISMATCH")
		}
	} else if envelope.BindingType != nil || envelope.ProductID != nil || envelope.ProductBlueprintID != nil {
		return errors.New("COMMERCE_SKILL_IDENTITY_MISMATCH")
	}
	return nil
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
