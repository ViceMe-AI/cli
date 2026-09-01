package publication

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/gofrs/flock"
)

const BindingAPIVersion = "binding.viceme.ai/v1"

type SkillBinding struct {
	APIVersion        string `json:"apiVersion"`
	Kind              string `json:"kind"`
	ListingID         string `json:"listingId"`
	ClientWorkID      string `json:"clientWorkId"`
	Market            string `json:"market"`
	EndpointOrigin    string `json:"endpointOrigin"`
	BindingReceipt    string `json:"bindingReceipt"`
	LastPackageDigest string `json:"lastPackageDigest"`
}

type SourceIntent struct {
	SourceType      string `json:"sourceType"`
	SourcePath      string `json:"sourcePath"`
	PackageDigest   string `json:"packageDigest"`
	ClientWorkID    string `json:"clientWorkId"`
	ClientRequestID string `json:"clientRequestId"`
	Resolution      string `json:"resolution,omitempty"`
}

type ResolvedSourceIdentity struct {
	SourceType      string
	SourcePath      string
	ClientWorkID    string
	ClientRequestID string
	Binding         *SkillBinding
}

type bindingIndexEntry struct {
	SourceType string       `json:"sourceType"`
	SourcePath string       `json:"sourcePath"`
	Binding    SkillBinding `json:"binding"`
}

type bindingIndex struct {
	SchemaVersion  int                 `json:"schemaVersion"`
	EndpointOrigin string              `json:"endpointOrigin"`
	Bindings       []bindingIndexEntry `json:"bindings"`
	Intents        []SourceIntent      `json:"intents"`
}

type BindingStore struct {
	Directory      string
	EndpointOrigin string
	Now            func() time.Time
}

func SourceType(sourcePath string) (string, string, error) {
	abs, err := filepath.Abs(sourcePath)
	if err != nil {
		return "", "", output.Validation("SKILL_PATH_INVALID", "could not resolve the Skill path").WithCause(err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return "", "", output.Validation("SKILL_PATH_NOT_FOUND", "Skill path does not exist").WithCause(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", "", output.Validation("SKILL_SYMLINK_REJECTED", "Skill source cannot be a symbolic link")
	}
	if info.IsDir() {
		manifest, err := os.Lstat(filepath.Join(abs, "SKILL.md"))
		if err != nil || !manifest.Mode().IsRegular() || manifest.Mode()&os.ModeSymlink != 0 {
			return "", "", output.Validation("SKILL_MANIFEST_MISSING", "Skill workspace must contain a regular SKILL.md at its root")
		}
		return "WORKSPACE", abs, nil
	}
	if info.Mode().IsRegular() && strings.EqualFold(filepath.Ext(abs), ".zip") {
		return "ZIP", abs, nil
	}
	return "", "", output.Validation("SKILL_SOURCE_UNSUPPORTED", "Skill source must be a directory or ZIP file")
}

func (s BindingStore) ResolveOrCreate(sourcePath, sourceType, packageDigest, resolution string, newID func() string) (ResolvedSourceIdentity, error) {
	if err := s.validateScope(); err != nil {
		return ResolvedSourceIdentity{}, err
	}
	if !isHexDigest(packageDigest) {
		return ResolvedSourceIdentity{}, output.Validation("SKILL_BINDING_DIGEST_INVALID", "canonical package digest is invalid")
	}
	if resolution == "" && !isLogicalRemoteSource(sourcePath) {
		binding, found, err := s.loadSidecar(sourcePath, sourceType)
		if err != nil {
			return ResolvedSourceIdentity{}, err
		}
		if found {
			return ResolvedSourceIdentity{SourceType: sourceType, SourcePath: sourcePath, ClientWorkID: binding.ClientWorkID, ClientRequestID: newID(), Binding: &binding}, nil
		}
	}
	if err := os.MkdirAll(s.Directory, 0o700); err != nil {
		return ResolvedSourceIdentity{}, bindingWriteError(s.Directory, "SKILL_BINDING_INDEX_SAVE_FAILED", "could not create the local Skill binding index", err)
	}
	lock := flock.New(s.lockFilename())
	if err := lock.Lock(); err != nil {
		return ResolvedSourceIdentity{}, bindingWriteError(s.Directory, "SKILL_BINDING_INDEX_LOCK_FAILED", "could not lock the local Skill binding index", err)
	}
	defer lock.Unlock()
	index, err := s.loadIndex()
	if err != nil {
		return ResolvedSourceIdentity{}, err
	}
	if resolution == "" {
		if binding := resolveIndexedBinding(index.Bindings, sourcePath, sourceType, packageDigest); binding != nil {
			return ResolvedSourceIdentity{SourceType: sourceType, SourcePath: sourcePath, ClientWorkID: binding.ClientWorkID, ClientRequestID: newID(), Binding: binding}, nil
		}
	}
	if intent := resolveIntent(index.Intents, sourcePath, sourceType, packageDigest, resolution); intent != nil {
		return ResolvedSourceIdentity{SourceType: sourceType, SourcePath: sourcePath, ClientWorkID: intent.ClientWorkID, ClientRequestID: intent.ClientRequestID}, nil
	}
	intent := SourceIntent{SourceType: sourceType, SourcePath: sourcePath, PackageDigest: packageDigest, ClientWorkID: newID(), ClientRequestID: newID(), Resolution: resolution}
	if !safeID(intent.ClientWorkID) || !safeID(intent.ClientRequestID) {
		return ResolvedSourceIdentity{}, output.Internal("SKILL_BINDING_ID_FAILED", "could not allocate stable Skill source identity", nil)
	}
	index.Intents = append(index.Intents, intent)
	if err := s.saveIndex(index); err != nil {
		return ResolvedSourceIdentity{}, err
	}
	return ResolvedSourceIdentity{SourceType: sourceType, SourcePath: sourcePath, ClientWorkID: intent.ClientWorkID, ClientRequestID: intent.ClientRequestID}, nil
}

func (s BindingStore) Save(sourcePath, sourceType string, binding SkillBinding) error {
	if err := s.validateBinding(binding); err != nil {
		return err
	}
	if !isLogicalRemoteSource(sourcePath) {
		if err := writeBindingJSON(sidecarFilename(sourcePath, sourceType), binding); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(s.Directory, 0o700); err != nil {
		return bindingWriteError(s.Directory, "SKILL_BINDING_INDEX_SAVE_FAILED", "could not create the local Skill binding index", err)
	}
	lock := flock.New(s.lockFilename())
	if err := lock.Lock(); err != nil {
		return bindingWriteError(s.Directory, "SKILL_BINDING_INDEX_LOCK_FAILED", "could not lock the local Skill binding index", err)
	}
	defer lock.Unlock()
	index, err := s.loadIndex()
	if err != nil {
		return err
	}
	next := make([]bindingIndexEntry, 0, len(index.Bindings)+1)
	for _, entry := range index.Bindings {
		if entry.Binding.ClientWorkID == binding.ClientWorkID || (entry.SourceType == sourceType && entry.SourcePath == sourcePath) {
			continue
		}
		next = append(next, entry)
	}
	index.Bindings = append(next, bindingIndexEntry{SourceType: sourceType, SourcePath: sourcePath, Binding: binding})
	remaining := index.Intents[:0]
	for _, intent := range index.Intents {
		if intent.ClientWorkID != binding.ClientWorkID {
			remaining = append(remaining, intent)
		}
	}
	index.Intents = remaining
	return s.saveIndex(index)
}

func (s BindingStore) loadSidecar(sourcePath, sourceType string) (SkillBinding, bool, error) {
	data, err := os.ReadFile(sidecarFilename(sourcePath, sourceType))
	if errors.Is(err, fs.ErrNotExist) {
		return SkillBinding{}, false, nil
	}
	if err != nil {
		return SkillBinding{}, false, output.Validation("SKILL_BINDING_READ_FAILED", "could not read the local Skill binding").WithCause(err)
	}
	var binding SkillBinding
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&binding); err != nil {
		return SkillBinding{}, false, output.Validation("SKILL_BINDING_INVALID", "local Skill binding is invalid").WithCause(err)
	}
	if err := s.validateBinding(binding); err != nil {
		return SkillBinding{}, false, err
	}
	return binding, true, nil
}

func (s BindingStore) validateScope() error {
	parsed, err := url.Parse(s.EndpointOrigin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return output.Internal("SKILL_BINDING_SCOPE_INVALID", "Skill binding endpoint scope is invalid", err)
	}
	return nil
}

func (s BindingStore) validateBinding(binding SkillBinding) error {
	if binding.APIVersion != BindingAPIVersion || binding.Kind != "SkillListing" || !safeID(binding.ListingID) || !safeID(binding.ClientWorkID) || !isHexDigest(binding.LastPackageDigest) || binding.BindingReceipt == "" {
		return output.Validation("SKILL_BINDING_INVALID", "local Skill binding is invalid")
	}
	if (binding.Market != "CN" && binding.Market != "GLOBAL") || binding.EndpointOrigin != s.EndpointOrigin {
		return output.Validation("SKILL_BINDING_SCOPE_MISMATCH", "local Skill binding belongs to another API endpoint").WithHint("select the original profile endpoint, or use --new-listing only when you explicitly want a separate work")
	}
	return nil
}

func (s BindingStore) loadIndex() (bindingIndex, error) {
	data, err := os.ReadFile(s.indexFilename())
	if errors.Is(err, fs.ErrNotExist) {
		return bindingIndex{SchemaVersion: 1, EndpointOrigin: s.EndpointOrigin}, nil
	}
	if err != nil {
		return bindingIndex{}, bindingWriteError(s.Directory, "SKILL_BINDING_INDEX_READ_FAILED", "could not read the local Skill binding index", err)
	}
	var index bindingIndex
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&index); err != nil || index.SchemaVersion != 1 || index.EndpointOrigin != s.EndpointOrigin {
		return bindingIndex{}, output.Validation("SKILL_BINDING_INDEX_INVALID", "local Skill binding index is invalid")
	}
	return index, nil
}

func (s BindingStore) saveIndex(index bindingIndex) error {
	index.SchemaVersion = 1
	index.EndpointOrigin = s.EndpointOrigin
	return writeBindingJSON(s.indexFilename(), index)
}

func (s BindingStore) indexFilename() string {
	return filepath.Join(s.Directory, endpointKey(s.EndpointOrigin)+".json")
}

func (s BindingStore) lockFilename() string {
	return filepath.Join(s.Directory, endpointKey(s.EndpointOrigin)+".lock")
}

func sidecarFilename(sourcePath, sourceType string) string {
	if sourceType == "WORKSPACE" {
		return filepath.Join(sourcePath, ".viceme", "skill.json")
	}
	return sourcePath + ".viceme.json"
}

func resolveIndexedBinding(entries []bindingIndexEntry, sourcePath, sourceType, digest string) *SkillBinding {
	for _, entry := range entries {
		if entry.SourceType == sourceType && entry.SourcePath == sourcePath {
			value := entry.Binding
			return &value
		}
	}
	if isLogicalRemoteSource(sourcePath) {
		return nil
	}
	var match *SkillBinding
	for _, entry := range entries {
		if entry.SourceType == sourceType && entry.Binding.LastPackageDigest == digest {
			if match != nil && match.ListingID != entry.Binding.ListingID {
				return nil
			}
			value := entry.Binding
			match = &value
		}
	}
	return match
}

func resolveIntent(intents []SourceIntent, sourcePath, sourceType, digest, resolution string) *SourceIntent {
	for _, intent := range intents {
		if intent.Resolution == resolution && intent.SourceType == sourceType && intent.SourcePath == sourcePath && intent.PackageDigest == digest {
			value := intent
			return &value
		}
	}
	if isLogicalRemoteSource(sourcePath) {
		return nil
	}
	var match *SourceIntent
	for _, intent := range intents {
		if intent.Resolution == resolution && intent.SourceType == sourceType && intent.PackageDigest == digest {
			if match != nil && match.ClientWorkID != intent.ClientWorkID {
				return nil
			}
			value := intent
			match = &value
		}
	}
	return match
}

func isLogicalRemoteSource(sourcePath string) bool {
	return strings.HasPrefix(sourcePath, "remote:")
}

func writeBindingJSON(filename string, value any) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		return bindingWriteError(filepath.Dir(filename), "SKILL_BINDING_SAVE_FAILED", "could not create the Skill binding directory", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return output.Internal("SKILL_BINDING_SAVE_FAILED", "could not encode the Skill binding", err)
	}
	data = append(data, '\n')
	if err := privatefile.Write(filename, data, ".binding-*.tmp"); err != nil {
		return bindingWriteError(filepath.Dir(filename), "SKILL_BINDING_SAVE_FAILED", "could not write the Skill binding", err)
	}
	return nil
}

func bindingWriteError(directory, code, message string, err error) error {
	if errors.Is(err, fs.ErrPermission) {
		return output.Policy("SKILL_BINDING_PERMISSION_REQUIRED", "ViceMe cannot write the stable Skill binding from this process").WithCause(err).WithDetails(map[string]any{"directory": directory}).WithHint("allow this process to write the reported directory, then retry the exact same command")
	}
	return output.Internal(code, message, err)
}

func endpointKey(origin string) string {
	digest := sha256.Sum256([]byte(origin))
	return hex.EncodeToString(digest[:16])
}
