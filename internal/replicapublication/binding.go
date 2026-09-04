package replicapublication

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/ViceMe-AI/cli/internal/privatepath"
	"github.com/ViceMe-AI/cli/internal/replicacontent"
)

type Binding struct {
	SchemaVersion      int                                 `json:"schemaVersion"`
	Kind               string                              `json:"kind"`
	EndpointOrigin     string                              `json:"endpointOrigin"`
	Market             string                              `json:"market"`
	ProjectFingerprint string                              `json:"projectFingerprint"`
	Publication        BindingPublication                  `json:"publication"`
	Merchant           BindingMerchant                     `json:"merchant"`
	FrozenSource       replicacontent.SourceArchiveSummary `json:"frozenSource"`
	Work               *BindingWork                        `json:"work"`
	Replica            *BindingReplica                     `json:"replica"`
	Product            *BindingProduct                     `json:"product"`
	Version            *BindingVersion                     `json:"version"`
	UpdatedAt          time.Time                           `json:"updatedAt"`
}

type BindingPublication struct {
	ID              string `json:"id"`
	ClientRequestID string `json:"clientRequestId"`
	Status          string `json:"status"`
	StatusURL       string `json:"statusUrl"`
}

type BindingMerchant struct {
	ID string `json:"id"`
}

type BindingWork struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type BindingReplica struct {
	ID          string `json:"id"`
	ShortCode   string `json:"shortCode"`
	Instruction string `json:"instruction"`
}

type BindingProduct struct {
	ID         string `json:"id"`
	SKUID      string `json:"skuId"`
	Currency   string `json:"currency"`
	PriceCents int    `json:"priceCents"`
}

type BindingVersion struct {
	ID          string    `json:"id"`
	Number      int       `json:"number"`
	PublishedAt time.Time `json:"publishedAt"`
}

type BindingStore struct {
	EndpointOrigin string
	Market         string
	Now            func() time.Time
}

func (store BindingStore) Load(projectPath string) (Binding, bool, error) {
	filename := bindingPath(projectPath)
	info, err := os.Lstat(filename)
	if errors.Is(err, fs.ErrNotExist) {
		return Binding{}, false, nil
	}
	if err != nil {
		return Binding{}, false, bindingError("REPLICA_BINDING_READ_FAILED", "could not inspect the local Website Replica binding", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return Binding{}, false, bindingSafetyError(errors.New("Website Replica binding is not a real regular file"))
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return Binding{}, false, bindingError("REPLICA_BINDING_READ_FAILED", "could not read the local Website Replica binding", err)
	}
	var binding Binding
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&binding); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return Binding{}, false, output.Validation("REPLICA_BINDING_INVALID", "local Website Replica binding is invalid").WithCause(err)
	}
	if err := store.validate(binding); err != nil {
		return Binding{}, false, err
	}
	return binding, true, nil
}

func (store BindingStore) Save(projectPath string, pending Pending, publication api.WebsiteReplicaPublication) error {
	if pending.Publication == nil || pending.ProjectPath != projectPath || publication.ID == "" || publication.ID != pending.Publication.ID ||
		publication.ClientRequestID != pending.ClientRequestID || publication.MerchantAccountID == "" {
		return output.Validation("REPLICA_BINDING_INVALID", "Website Replica Publication cannot be bound to this project")
	}
	existing, found, err := store.Load(projectPath)
	if err != nil {
		return err
	}
	binding := existing
	if !found {
		binding = Binding{
			SchemaVersion:      bindingSchemaVersion,
			Kind:               "WebsiteReplica",
			EndpointOrigin:     store.EndpointOrigin,
			Market:             store.Market,
			ProjectFingerprint: pending.ProjectFingerprint,
			FrozenSource:       pending.SourceArchive,
		}
	}
	if binding.ProjectFingerprint != pending.ProjectFingerprint {
		return output.Validation("REPLICA_BINDING_PROJECT_MISMATCH", "local Website Replica binding belongs to another project")
	}
	binding.Publication = BindingPublication{
		ID: publication.ID, ClientRequestID: publication.ClientRequestID,
		Status: publication.Status, StatusURL: publication.StatusURL,
	}
	binding.Merchant = BindingMerchant{ID: publication.MerchantAccountID}
	binding.FrozenSource = pending.SourceArchive
	if publication.Status == "PUBLISHED" || publication.Status == "PUBLISHED_DEGRADED" {
		if publication.Result == nil {
			return output.Validation("REPLICA_BINDING_INVALID", "published Website Replica response is missing its stable result")
		}
		publishedAt, parseErr := parsePublicationDatetime(publication.Result.PublishedAt)
		if parseErr != nil {
			return output.Validation("REPLICA_BINDING_INVALID", "published Website Replica time is invalid").WithCause(parseErr)
		}
		binding.Work = &BindingWork{ID: publication.WorkID, URL: publication.Result.WorkURL}
		binding.Replica = &BindingReplica{
			ID: publication.ReplicaID, ShortCode: publication.Result.ShortCode,
			Instruction: publication.Result.Instruction,
		}
		binding.Product = &BindingProduct{
			ID: publication.Result.Product.ID, SKUID: publication.Result.Product.SKUID,
			Currency: publication.Result.Product.Currency, PriceCents: publication.Result.Product.PriceCents,
		}
		binding.Version = &BindingVersion{ID: publication.Result.VersionID, Number: publication.Result.Version, PublishedAt: publishedAt}
	}
	if store.Now == nil {
		store.Now = time.Now
	}
	binding.UpdatedAt = store.Now().UTC()
	if err := store.validate(binding); err != nil {
		return err
	}
	filename := bindingPath(projectPath)
	if err := ensureBindingDirectory(filepath.Dir(filename)); err != nil {
		return bindingSafetyError(err)
	}
	data, err := json.MarshalIndent(binding, "", "  ")
	if err != nil {
		return output.Internal("REPLICA_BINDING_SAVE_FAILED", "could not encode the local Website Replica binding", err)
	}
	data = append(data, '\n')
	if err := privatefile.WriteAtomic(filename, data, ".website-replica-*.tmp"); err != nil {
		return bindingError("REPLICA_BINDING_SAVE_FAILED", "could not atomically save the local Website Replica binding", err)
	}
	return nil
}

func (store BindingStore) validate(binding Binding) error {
	if binding.SchemaVersion != bindingSchemaVersion || binding.Kind != "WebsiteReplica" || binding.EndpointOrigin != store.EndpointOrigin ||
		binding.Market != store.Market || !digestPattern.MatchString(binding.ProjectFingerprint) ||
		!uuidPattern.MatchString(binding.Publication.ID) || !uuidPattern.MatchString(binding.Publication.ClientRequestID) ||
		!validPublicationStatus(binding.Publication.Status) || binding.Publication.StatusURL == "" || !uuidPattern.MatchString(binding.Merchant.ID) ||
		!digestPattern.MatchString(binding.FrozenSource.Digest) || binding.FrozenSource.SizeBytes < 1 || binding.UpdatedAt.IsZero() {
		return output.Validation("REPLICA_BINDING_INVALID", "local Website Replica binding is invalid")
	}
	stableFields := []bool{binding.Work != nil, binding.Replica != nil, binding.Product != nil, binding.Version != nil}
	for _, present := range stableFields[1:] {
		if present != stableFields[0] {
			return output.Validation("REPLICA_BINDING_INVALID", "local Website Replica stable binding is incomplete")
		}
	}
	if binding.Work != nil {
		if !uuidPattern.MatchString(binding.Work.ID) || binding.Work.URL == "" || !uuidPattern.MatchString(binding.Replica.ID) ||
			binding.Replica.ShortCode == "" || binding.Replica.Instruction != "VICEME-REPLICA:"+binding.Replica.ShortCode ||
			!uuidPattern.MatchString(binding.Product.ID) || !uuidPattern.MatchString(binding.Product.SKUID) ||
			(binding.Product.Currency != "CNY" && binding.Product.Currency != "USD") || binding.Product.PriceCents < 0 ||
			!uuidPattern.MatchString(binding.Version.ID) || binding.Version.Number < 1 || binding.Version.PublishedAt.IsZero() {
			return output.Validation("REPLICA_BINDING_INVALID", "local Website Replica stable binding is invalid")
		}
	}
	return nil
}

func bindingPath(projectPath string) string {
	root := projectPath
	if info, err := os.Lstat(projectPath); err == nil {
		if info.Mode().IsRegular() {
			root = filepath.Dir(projectPath)
		}
	} else if strings.EqualFold(filepath.Ext(projectPath), ".zip") {
		root = filepath.Dir(projectPath)
	}
	return filepath.Join(root, ".viceme", bindingFilename)
}

func bindingError(code, message string, err error) *output.Error {
	if errors.Is(err, fs.ErrPermission) || privatefile.IsPermissionDenial(err) {
		return bindingSafetyError(err)
	}
	return output.Internal(code, message, err)
}

func bindingSafetyError(err error) *output.Error {
	return output.Policy("REPLICA_BINDING_PERMISSION_REQUIRED", "ViceMe cannot safely read or atomically write the local Website Replica binding").WithCause(err)
}

func ensureBindingDirectory(directory string) error {
	_, err := privatepath.EnsureDirectory(directory)
	return err
}

func parsePublicationDatetime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return parsed, nil
	}
	return time.Parse("2006-01-02T15:04Z", value)
}
