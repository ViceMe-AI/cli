package interactionartifact

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
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const signaturePath = "interaction-signature.json"

type File struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"sizeBytes"`
}

type Envelope struct {
	SchemaVersion          int    `json:"schemaVersion"`
	ArtifactType           string `json:"artifactType"`
	WorkID                 string `json:"workId"`
	DefinitionVersionID    string `json:"definitionVersionId"`
	StableName             string `json:"stableName"`
	SkillReleaseID         string `json:"skillReleaseId"`
	ReleaseVersion         int    `json:"releaseVersion"`
	TemplateVersion        int    `json:"templateVersion"`
	RuntimeProtocolVersion int    `json:"runtimeProtocolVersion"`
	MinimumRuntimeVersion  string `json:"minimumRuntimeVersion"`
	Files                  []File `json:"files"`
}

type SignatureFile struct {
	SchemaVersion  int      `json:"schemaVersion"`
	Algorithm      string   `json:"algorithm"`
	KeyID          string   `json:"keyId"`
	Envelope       Envelope `json:"envelope"`
	EnvelopeDigest string   `json:"envelopeDigest"`
	Signature      string   `json:"signature"`
}

type Manifest struct {
	SchemaVersion          int             `json:"schemaVersion"`
	WorkID                 string          `json:"workId"`
	DefinitionVersionID    string          `json:"definitionVersionId"`
	StableName             string          `json:"stableName"`
	SkillReleaseID         string          `json:"skillReleaseId"`
	ReleaseVersion         int             `json:"releaseVersion"`
	InteractionAPIProfile  string          `json:"interactionApiProfile"`
	RuntimeProtocolVersion int             `json:"runtimeProtocolVersion"`
	MinimumRuntimeVersion  string          `json:"minimumRuntimeVersion"`
	GeneratedAt            string          `json:"generatedAt"`
	EntryModes             []string        `json:"entryModes"`
	InitialInput           json.RawMessage `json:"initialInput"`
}

type Expected struct {
	ArtifactDigest string
	ManifestDigest string
	EnvelopeDigest string
	SigningKeyID   string
	Signature      string
	Manifest       Manifest
	Envelope       Envelope
}

type Verified struct{ Files map[string][]byte }

func ParseManifest(raw json.RawMessage) (Manifest, error) {
	var value Manifest
	if err := strictJSON(raw, &value); err != nil || !validManifest(value) {
		return Manifest{}, errors.New("INTERACTION_SKILL_MANIFEST_INVALID")
	}
	return value, nil
}

func ParseEnvelope(raw json.RawMessage) (Envelope, error) {
	var value Envelope
	if err := strictJSON(raw, &value); err != nil || !validEnvelope(value) {
		return Envelope{}, errors.New("INTERACTION_SKILL_ENVELOPE_INVALID")
	}
	return value, nil
}

func Verify(artifact []byte, trustedPublicKey string, expected Expected) (Verified, error) {
	if len(artifact) == 0 || len(artifact) > 32<<20 || !validManifest(expected.Manifest) || !validEnvelope(expected.Envelope) {
		return Verified{}, errors.New("INTERACTION_SKILL_DESCRIPTOR_INVALID")
	}
	if digest(artifact) != expected.ArtifactDigest || digestCanonical(expected.Manifest) != expected.ManifestDigest || digestCanonical(expected.Envelope) != expected.EnvelopeDigest {
		return Verified{}, errors.New("INTERACTION_SKILL_DESCRIPTOR_INVALID")
	}
	reader, err := zip.NewReader(bytes.NewReader(artifact), int64(len(artifact)))
	if err != nil {
		return Verified{}, errors.New("INTERACTION_SKILL_ARTIFACT_ZIP_INVALID")
	}
	entries := map[string]*zip.File{}
	for _, entry := range reader.File {
		if !safePath(entry.Name) || entry.FileInfo().IsDir() || !entry.Mode().IsRegular() || entries[entry.Name] != nil {
			return Verified{}, errors.New("INTERACTION_SKILL_ARTIFACT_PATH_INVALID")
		}
		entries[entry.Name] = entry
	}
	sigEntry := entries[signaturePath]
	if sigEntry == nil || sigEntry.UncompressedSize64 > 256<<10 {
		return Verified{}, errors.New("INTERACTION_SKILL_SIGNATURE_MISSING")
	}
	sigRaw, err := readEntry(sigEntry, int64(sigEntry.UncompressedSize64))
	if err != nil {
		return Verified{}, errors.New("INTERACTION_SKILL_SIGNATURE_INVALID")
	}
	var signature SignatureFile
	if err := strictJSON(sigRaw, &signature); err != nil || signature.SchemaVersion != 1 || signature.Algorithm != "Ed25519" || signature.KeyID != expected.SigningKeyID || signature.Signature != expected.Signature || signature.EnvelopeDigest != expected.EnvelopeDigest || !validEnvelope(signature.Envelope) {
		return Verified{}, errors.New("INTERACTION_SKILL_SIGNATURE_INVALID")
	}
	canonical := canonicalJSON(signature.Envelope)
	if digest(canonical) != signature.EnvelopeDigest || !bytes.Equal(canonical, canonicalJSON(expected.Envelope)) {
		return Verified{}, errors.New("INTERACTION_SKILL_IDENTITY_MISMATCH")
	}
	publicDER, err := base64.RawURLEncoding.DecodeString(trustedPublicKey)
	if err != nil {
		return Verified{}, errors.New("INTERACTION_SKILL_TRUSTED_KEY_INVALID")
	}
	parsed, err := x509.ParsePKIXPublicKey(publicDER)
	publicKey, ok := parsed.(ed25519.PublicKey)
	signatureBytes, decodeErr := base64.RawURLEncoding.DecodeString(signature.Signature)
	if err != nil || !ok || decodeErr != nil || !ed25519.Verify(publicKey, canonical, signatureBytes) {
		return Verified{}, errors.New("INTERACTION_SKILL_SIGNATURE_INVALID")
	}
	expectedPaths := map[string]bool{signaturePath: true}
	files := map[string][]byte{signaturePath: sigRaw}
	for _, file := range signature.Envelope.Files {
		entry := entries[file.Path]
		if expectedPaths[file.Path] || entry == nil || entry.UncompressedSize64 != uint64(file.SizeBytes) {
			return Verified{}, errors.New("INTERACTION_SKILL_ARTIFACT_FILE_SET_INVALID")
		}
		body, err := readEntry(entry, file.SizeBytes)
		if err != nil || digest(body) != file.SHA256 {
			return Verified{}, errors.New("INTERACTION_SKILL_ARTIFACT_FILE_DIGEST_INVALID")
		}
		expectedPaths[file.Path], files[file.Path] = true, body
	}
	if len(entries) != len(expectedPaths) {
		return Verified{}, errors.New("INTERACTION_SKILL_ARTIFACT_FILE_SET_INVALID")
	}
	manifest, err := ParseManifest(files["interaction-manifest.json"])
	if err != nil || !bytes.Equal(canonicalJSON(manifest), canonicalJSON(expected.Manifest)) || !sameIdentity(manifest, signature.Envelope) {
		return Verified{}, errors.New("INTERACTION_SKILL_IDENTITY_MISMATCH")
	}
	return Verified{Files: files}, nil
}

func validEnvelope(v Envelope) bool {
	if v.SchemaVersion != 1 || v.ArtifactType != "WORK_INTERACTION" || !uuid(v.WorkID) || !uuid(v.DefinitionVersionID) || !uuid(v.SkillReleaseID) || !stableName(v.StableName) || v.ReleaseVersion < 1 || v.TemplateVersion < 1 || v.RuntimeProtocolVersion < 1 || v.MinimumRuntimeVersion == "" || len(v.Files) < 2 {
		return false
	}
	previous, skill, manifest := "", false, false
	for _, file := range v.Files {
		if !safePath(file.Path) || len(file.SHA256) != 64 || file.SizeBytes < 0 || (previous != "" && !pathLess(previous, file.Path)) {
			return false
		}
		if _, err := hex.DecodeString(file.SHA256); err != nil {
			return false
		}
		previous, skill, manifest = file.Path, skill || file.Path == "SKILL.md", manifest || file.Path == "interaction-manifest.json"
	}
	return skill && manifest
}

func validManifest(v Manifest) bool {
	if v.SchemaVersion != 1 || !uuid(v.WorkID) || !uuid(v.DefinitionVersionID) || !uuid(v.SkillReleaseID) || !stableName(v.StableName) || v.ReleaseVersion < 1 || v.InteractionAPIProfile != "viceme-interaction-v1" || v.RuntimeProtocolVersion < 1 || v.MinimumRuntimeVersion == "" || len(v.EntryModes) == 0 || len(v.InitialInput) == 0 {
		return false
	}
	for _, mode := range v.EntryModes {
		if mode == "DIRECT" {
			return true
		}
	}
	return false
}

func sameIdentity(m Manifest, e Envelope) bool {
	return m.WorkID == e.WorkID && m.DefinitionVersionID == e.DefinitionVersionID && m.StableName == e.StableName && m.SkillReleaseID == e.SkillReleaseID && m.ReleaseVersion == e.ReleaseVersion && m.RuntimeProtocolVersion == e.RuntimeProtocolVersion && m.MinimumRuntimeVersion == e.MinimumRuntimeVersion
}
func digest(value []byte) string       { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }
func digestCanonical(value any) string { return digest(canonicalJSON(value)) }
func strictJSON(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}
func readEntry(entry *zip.File, size int64) ([]byte, error) {
	reader, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	body, err := io.ReadAll(io.LimitReader(reader, size+1))
	if err != nil || int64(len(body)) != size {
		return nil, errors.New("size")
	}
	return body, nil
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
	return true
}
func pathLess(left, right string) bool {
	lf, rf := strings.ToLower(left), strings.ToLower(right)
	if lf != rf {
		return lf < rf
	}
	return left < right
}

var stableNamePattern = regexp.MustCompile(`^viceme-[a-z0-9]+(?:-[a-z0-9]+)*$`)

func stableName(value string) bool {
	return len(value) >= 8 && len(value) <= 160 && stableNamePattern.MatchString(value)
}
func uuid(value string) bool {
	compact := strings.ReplaceAll(value, "-", "")
	if len(value) != 36 || len(compact) != 32 {
		return false
	}
	_, err := hex.DecodeString(compact)
	return err == nil
}
func canonicalJSON(value any) []byte {
	raw, _ := json.Marshal(value)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var generic any
	_ = decoder.Decode(&generic)
	var out bytes.Buffer
	writeCanonical(&out, generic)
	return out.Bytes()
}
func writeCanonical(out *bytes.Buffer, value any) {
	switch typed := value.(type) {
	case nil:
		out.WriteString("null")
	case bool:
		out.WriteString(strconv.FormatBool(typed))
	case string:
		raw, _ := json.Marshal(typed)
		out.Write(raw)
	case json.Number:
		out.WriteString(string(typed))
	case []any:
		out.WriteByte('[')
		for i, item := range typed {
			if i > 0 {
				out.WriteByte(',')
			}
			writeCanonical(out, item)
		}
		out.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			raw, _ := json.Marshal(key)
			out.Write(raw)
			out.WriteByte(':')
			writeCanonical(out, typed[key])
		}
		out.WriteByte('}')
	}
}
