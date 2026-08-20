package command

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/publication"
	"github.com/spf13/cobra"
)

const websiteBindingName = ".viceme/website.json"

var websiteUUIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var websiteWorkKeyPattern = regexp.MustCompile(`^wrk_[A-Za-z0-9_-]{4,124}$`)

type websiteBinding struct {
	SchemaVersion    int               `json:"schemaVersion"`
	ClientWorkID     string            `json:"clientWorkId"`
	WorkID           string            `json:"workId,omitempty"`
	WorkKey          string            `json:"workKey,omitempty"`
	Region           string            `json:"region"`
	DisplayName      string            `json:"displayName"`
	SourceURL        string            `json:"sourceUrl"`
	DescriptionZhCN  string            `json:"descriptionZhCn,omitempty"`
	DescriptionEnUS  string            `json:"descriptionEnUs,omitempty"`
	Cover            *api.WebsiteCover `json:"cover,omitempty"`
	LastSourceDigest string            `json:"lastSourceDigest,omitempty"`
	ReleaseVersion   int               `json:"releaseVersion,omitempty"`
}

func newWebsiteCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "website", Short: "Publish creator websites"}
	command.AddCommand(newWebsitePublishCommand(runtime))
	return command
}

func newWebsitePublishCommand(runtime *Runtime) *cobra.Command {
	var sourcePath string
	var displayName string
	var creatorDisplayName string
	var sourceURL string
	var descriptionZhCN string
	var descriptionEnUS string
	var coverPath string
	command := &cobra.Command{
		Use:   "publish",
		Short: "Publish or update the website work in the current source directory",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			root, bindingPath, err := resolveWebsiteBindingPath(sourcePath)
			if err != nil {
				return err
			}
			binding, found, err := loadWebsiteBinding(bindingPath)
			if err != nil {
				return err
			}
			if strings.TrimSpace(displayName) == "" {
				return output.Validation("WEBSITE_NAME_REQUIRED", "--name is required")
			}
			if err := validateWebsiteURL(sourceURL); err != nil {
				return err
			}
			if found && binding.Region != string(runtime.region) {
				return output.Validation("WEBSITE_REGION_MISMATCH", "website binding region does not match the active CLI profile")
			}
			if !found {
				binding = websiteBinding{
					SchemaVersion: 1,
					ClientWorkID:  randomUUID(),
					Region:        string(runtime.region),
				}
			}
			sourceDigest, err := digestWebsiteDirectory(root)
			if err != nil {
				return err
			}
			binding.DisplayName = strings.TrimSpace(displayName)
			if normalizedSourceURL := strings.TrimSpace(sourceURL); normalizedSourceURL != "" {
				binding.SourceURL = normalizedSourceURL
			}
			if value := strings.TrimSpace(descriptionZhCN); value != "" {
				binding.DescriptionZhCN = value
			}
			if value := strings.TrimSpace(descriptionEnUS); value != "" {
				binding.DescriptionEnUS = value
			}
			var coverCandidate *publication.Candidate
			if strings.TrimSpace(coverPath) != "" {
				candidate, err := publication.ReadCandidate(coverPath)
				if err != nil {
					return err
				}
				coverCandidate = &candidate
				binding.Cover = &api.WebsiteCover{
					Digest: candidate.Digest, SizeBytes: candidate.SizeBytes,
					FileName: candidate.FileName, ContentType: candidate.ContentType,
				}
			}
			// Persist the source identity before the network call. If the call succeeds
			// but the process is interrupted, the next publish still reuses this work.
			if err := writeWebsiteBinding(bindingPath, binding); err != nil {
				return err
			}
			client := runtime.client()
			if coverCandidate != nil {
				authorization, err := client.AuthorizeWebsiteCoverUpload(command.Context(), api.AuthorizeWebsiteCoverUploadRequest{
					ClientWorkID: binding.ClientWorkID,
					WebsiteCover: *binding.Cover,
				})
				if err != nil {
					return err
				}
				progress(runtime, "Uploading website cover")
				if err := client.PutUpload(command.Context(), authorization, bytes.NewReader(coverCandidate.Bytes), coverCandidate.SizeBytes); err != nil {
					return err
				}
			}
			work, err := client.PublishCreatorWebsite(command.Context(), api.PublishCreatorWebsiteRequest{
				ClientRequestID: randomUUID(), ClientWorkID: binding.ClientWorkID, SourceDigest: sourceDigest,
				DisplayName: binding.DisplayName, CreatorDisplayName: strings.TrimSpace(creatorDisplayName), SourceURL: binding.SourceURL,
				DescriptionZhCN: binding.DescriptionZhCN, DescriptionEnUS: binding.DescriptionEnUS, Cover: binding.Cover,
			})
			if err != nil {
				return err
			}
			if binding.WorkKey != "" && binding.WorkKey != work.WorkKey {
				return output.Validation("WEBSITE_IDENTITY_CONFLICT", "repeat publication returned a different workKey")
			}
			if binding.WorkID != "" && binding.WorkID != work.CreatorWorkID {
				return output.Validation("WEBSITE_IDENTITY_CONFLICT", "repeat publication returned a different workId")
			}
			if work.Publication != nil && work.Publication.ClientWorkID != binding.ClientWorkID {
				return output.Validation("WEBSITE_IDENTITY_CONFLICT", "published work clientWorkId does not match the local binding")
			}
			binding.WorkID = work.CreatorWorkID
			binding.WorkKey = work.WorkKey
			if work.Publication != nil {
				binding.LastSourceDigest = work.Publication.SourceDigest
				binding.ReleaseVersion = work.Publication.Version
			}
			if err := writeWebsiteBinding(bindingPath, binding); err != nil {
				return output.Internal("WEBSITE_BINDING_WRITE_FAILED", "website was published but the local work binding could not be updated", err).WithDetails(map[string]any{"workKey": work.WorkKey, "clientWorkId": binding.ClientWorkID})
			}
			return runtime.business(map[string]any{"work": work, "bindingPath": bindingPath})
		},
	}
	command.Flags().StringVar(&sourcePath, "path", ".", "website source directory")
	command.Flags().StringVar(&displayName, "name", "", "website display name")
	command.Flags().StringVar(&creatorDisplayName, "creator-display-name", "", "creator display name used when the account has none")
	command.Flags().StringVar(&sourceURL, "url", "", "optional published website URL")
	command.Flags().StringVar(&descriptionZhCN, "description-zh-cn", "", "optional Chinese website description")
	command.Flags().StringVar(&descriptionEnUS, "description-en-us", "", "optional English website description")
	command.Flags().StringVar(&coverPath, "cover", "", "optional local website cover image")
	return command
}

func resolveWebsiteBindingPath(sourcePath string) (string, string, error) {
	root, err := filepath.Abs(sourcePath)
	if err != nil {
		return "", "", output.Validation("WEBSITE_PATH_INVALID", "could not resolve website path")
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", "", output.Validation("WEBSITE_PATH_INVALID", "website path must be a directory")
	}
	return root, filepath.Join(root, filepath.FromSlash(websiteBindingName)), nil
}

func loadWebsiteBinding(filename string) (websiteBinding, bool, error) {
	data, err := os.ReadFile(filename)
	if errors.Is(err, os.ErrNotExist) {
		return websiteBinding{}, false, nil
	}
	if err != nil {
		return websiteBinding{}, false, output.Validation("WEBSITE_BINDING_READ_FAILED", "could not read website binding")
	}
	var binding websiteBinding
	if err := json.Unmarshal(data, &binding); err != nil || binding.SchemaVersion != 1 || !websiteUUIDPattern.MatchString(binding.ClientWorkID) || (binding.WorkKey != "" && !websiteWorkKeyPattern.MatchString(binding.WorkKey)) {
		return websiteBinding{}, false, output.Validation("WEBSITE_BINDING_INVALID", "website binding is invalid")
	}
	return binding, true, nil
}

func requirePublishedWebsiteBinding(sourcePath string) (websiteBinding, string, error) {
	_, filename, err := resolveWebsiteBindingPath(sourcePath)
	if err != nil {
		return websiteBinding{}, "", err
	}
	binding, found, err := loadWebsiteBinding(filename)
	if err != nil {
		return websiteBinding{}, "", err
	}
	if !found || binding.WorkKey == "" {
		return websiteBinding{}, "", output.Validation("WEBSITE_PUBLICATION_REQUIRED", "publish this website before configuring access").WithHint("run 'viceme website publish --path <dir> --name <name>'")
	}
	return binding, filename, nil
}

func writeWebsiteBinding(filename string, binding websiteBinding) error {
	if binding.SchemaVersion != 1 || !websiteUUIDPattern.MatchString(binding.ClientWorkID) {
		return output.Validation("WEBSITE_BINDING_INVALID", "website binding is invalid")
	}
	data, err := json.MarshalIndent(binding, "", "  ")
	if err != nil {
		return output.Internal("WEBSITE_BINDING_WRITE_FAILED", "could not encode website binding", err)
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return output.Internal("WEBSITE_BINDING_WRITE_FAILED", "could not create website binding directory", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(filename), ".website-*.json")
	if err != nil {
		return output.Internal("WEBSITE_BINDING_WRITE_FAILED", "could not create website binding", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		_ = temporary.Close()
		return output.Internal("WEBSITE_BINDING_WRITE_FAILED", "could not write website binding", err)
	}
	if err := temporary.Close(); err != nil {
		return output.Internal("WEBSITE_BINDING_WRITE_FAILED", "could not close website binding", err)
	}
	if err := os.Rename(temporaryName, filename); err != nil {
		return output.Internal("WEBSITE_BINDING_WRITE_FAILED", "could not replace website binding", err)
	}
	return nil
}

func validateWebsiteURL(raw string) error {
	return validateOptionalWebsiteURL("--url", raw)
}

func validateOptionalWebsiteURL(flag, raw string) error {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return nil
	}
	parsed, err := url.ParseRequestURI(normalized)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return output.Validation("WEBSITE_URL_INVALID", flag+" must be an absolute http(s) URL")
	}
	return nil
}

func digestWebsiteDirectory(root string) (string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		first := strings.Split(filepath.ToSlash(relative), "/")[0]
		if entry.IsDir() && (first == ".git" || first == ".viceme" || first == "node_modules") {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return output.Validation("WEBSITE_SYMLINK_UNSUPPORTED", fmt.Sprintf("website source contains symlink %q", filepath.ToSlash(relative)))
		}
		if entry.Type().IsRegular() {
			files = append(files, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		var cliError *output.Error
		if errors.As(err, &cliError) {
			return "", cliError
		}
		return "", output.Validation("WEBSITE_DIGEST_FAILED", "could not read website source")
	}
	sort.Strings(files)
	hash := sha256.New()
	for _, relative := range files {
		file, err := os.Open(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			return "", output.Validation("WEBSITE_DIGEST_FAILED", "could not read website source")
		}
		info, err := file.Stat()
		if err == nil {
			_, _ = fmt.Fprintf(hash, "%d:%s:%d:", len(relative), relative, info.Size())
			_, err = io.Copy(hash, file)
		}
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			return "", output.Validation("WEBSITE_DIGEST_FAILED", "could not read website source")
		}
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
