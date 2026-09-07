package command

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/replicapreview"
	"github.com/ViceMe-AI/cli/internal/replicapublication"
)

func TestCreatorPreviewMissingInputsNeverReachPublication(t *testing.T) {
	for _, scenario := range []struct {
		name, code, action string
		url                bool
	}{
		{"approval missing without URL", "REPLICA_PREVIEW_REVIEW_REQUIRED", "CONFIRM_CREATOR_PREVIEW", false},
		{"missing creator approval", "REPLICA_PREVIEW_REVIEW_REQUIRED", "CONFIRM_CREATOR_PREVIEW", true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			project := t.TempDir()
			if err := os.WriteFile(filepath.Join(project, "my-reading-corner.html"), []byte("<h1>Reading corner</h1>"), 0600); err != nil {
				t.Fatal(err)
			}
			remoteCalls, pageCalls, opened := 0, 0, 0
			apiServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { remoteCalls++ }))
			defer apiServer.Close()
			page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				pageCalls++
				if r.URL.RequestURI() != "/my-reading-corner.html?year=2026" {
					t.Errorf("wrong entry: %s", r.URL.RequestURI())
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer page.Close()
			root := t.TempDir()
			deps := replicaPublicationTestDependencies(t, root, apiServer, time.Now().UTC())
			deps.Store = &replicaPreviewCredentialStore{}
			deps.StartReplicaPreview = replicapreview.Start
			deps.OpenURL = func(context.Context, string) error { opened++; return nil }
			deps.NewID = func() string { t.Fatal("input-only step created a publication identity"); return "" }
			var stdout bytes.Buffer
			deps.Out = &stdout
			args := []string{"replica", "publish", "--path", project, "--slug", "reading-corner", "--title", "Reading corner", "--price-cents", "1"}
			if scenario.url {
				args = append(args, "--preview-url", page.URL+"/my-reading-corner.html?year=2026")
			}
			if exit := Execute(args, deps); exit != output.ExitValidation || !strings.Contains(stdout.String(), scenario.code) || !strings.Contains(stdout.String(), scenario.action) {
				t.Fatalf("unexpected result: exit=%d %s", exit, stdout.String())
			}
			if scenario.url {
				var envelope struct {
					Error struct {
						Details struct {
							ReviewRequiredBy   string `json:"reviewRequiredBy"`
							PreviewURL         string `json:"previewUrl"`
							PresentationTarget string `json:"presentationTarget"`
						} `json:"details"`
					} `json:"error"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				details := envelope.Error.Details
				if details.ReviewRequiredBy != "CREATOR" || details.PreviewURL != page.URL+"/my-reading-corner.html?year=2026" {
					t.Fatalf("missing creator presentation handoff: %+v", details)
				}
			}
			if remoteCalls != 0 || deps.Store.(*replicaPreviewCredentialStore).reads.Load() != 0 {
				t.Fatal("input step crossed remote/auth boundary")
			}
			if scenario.url && (pageCalls != 0 || opened != 0) {
				t.Fatal("approval must not probe the page or open a browser")
			}
			if !scenario.url && (pageCalls != 0 || opened != 0) {
				t.Fatal("missing URL guessed a page")
			}
			entries, _ := os.ReadDir(project)
			if len(entries) != 1 {
				t.Fatal("source tree modified")
			}
		})
	}
}

func TestCreatorPreviewReviewedPageReachesFrozenReview(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "my-reading-corner.html"), []byte("<h1>Reading corner</h1>"), 0600); err != nil {
		t.Fatal(err)
	}
	page := httptest.NewServer(http.FileServer(http.Dir(project)))
	page.Close() // A creator-approved URL may already be offline when publication starts.
	// Reuse the publication fixture to exercise freezing, review and command recovery.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input map[string]any
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
			return
		}
		writeJSONResponse(w, replicaConfirmationRequiredResponse(now, input, "wrv1-"+strings.Repeat("b", 64)))
	}))
	defer server.Close()
	deps := replicaPublicationTestDependencies(t, t.TempDir(), server, now)
	deps.StartReplicaPreview = replicapreview.Start
	deps.OpenURL = func(context.Context, string) error { t.Fatal("publish reopened the approved preview"); return nil }
	var stdout bytes.Buffer
	deps.Out = &stdout
	args := append(replicaPublicationTestArguments(project, "reading-corner"), "--preview-url", page.URL+"/my-reading-corner.html")
	if exit := Execute(args, deps); exit != output.ExitConfirmation || !strings.Contains(stdout.String(), "REPLICA_PUBLICATION_CONFIRMATION_REQUIRED") || !strings.Contains(stdout.String(), `"reviewedBy": "CREATOR"`) || !strings.Contains(stdout.String(), "--preview-reviewed") {
		t.Fatalf("single HTML did not reach reviewed freeze: exit=%d %s", exit, stdout.String())
	}
	for _, change := range [][]string{
		{"--preview-url", page.URL + "/different.html"},
		{"--preview-reviewed=false"},
	} {
		stdout.Reset()
		changed := append(append([]string{}, args...), "--confirm", "wrv1-"+strings.Repeat("b", 64))
		changed = append(changed, change...)
		if exit := Execute(changed, deps); exit != output.ExitConfirmation || !strings.Contains(stdout.String(), "REPLICA_PUBLICATION_CONFIRMATION_CHANGED") {
			t.Fatalf("changed preview crossed confirmation: exit=%d %s", exit, stdout.String())
		}
	}
}

func TestReplicaPreviewConfirmationPreservesCreatorAndLegacyReviews(t *testing.T) {
	for _, reviewer := range []string{"CREATOR", "AGENT"} {
		t.Run(reviewer, func(t *testing.T) {
			pending := replicapublication.Pending{
				ProjectPath:  "/tmp/site",
				Preview:      replicapublication.Preview{Verified: true, ReviewedBy: reviewer, Reused: true, TargetURL: "http://127.0.0.1:4173/"},
				Confirmation: &api.WebsiteReplicaPublicationConfirmationChallenge{Version: "review-version"},
			}
			command := replicaPublishResumeCommand(pending)
			if !strings.Contains(command, "--preview-reviewed") || !strings.Contains(command, pending.Preview.TargetURL) {
				t.Fatalf("resume lost approved preview: %s", command)
			}
			for _, options := range []replicaPublishOptions{
				{ConfirmationVersion: "review-version", PreviewURL: pending.Preview.TargetURL},
				{ConfirmationVersion: "review-version", PreviewReviewed: true, PreviewURL: "http://127.0.0.1:4173/changed"},
				{ConfirmationVersion: "review-version", PreviewReviewed: true, PreviewURL: pending.Preview.TargetURL, ConfirmUnverifiedPreview: true},
			} {
				err := validateConfirmedReplicaRequest(options, pending, replicapublication.Binding{}, false)
				if err == nil || !strings.Contains(err.Error(), "approved preview changed") {
					t.Fatalf("changed preview must invalidate %s approval: %v", reviewer, err)
				}
			}
		})
	}
}
