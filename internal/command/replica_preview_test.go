package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/replicapreview"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	updatepkg "github.com/ViceMe-AI/cli/internal/update"
)

type replicaPreviewSessionStub struct {
	result   replicapreview.Result
	closed   atomic.Bool
	closeErr error
}

func (session *replicaPreviewSessionStub) Result() replicapreview.Result { return session.result }
func (session *replicaPreviewSessionStub) Wait(context.Context) error    { return nil }
func (session *replicaPreviewSessionStub) Close() error {
	session.closed.Store(true)
	return session.closeErr
}

type replicaPreviewCredentialStore struct{ reads atomic.Int32 }

func (store *replicaPreviewCredentialStore) Get(string) (string, error) {
	store.reads.Add(1)
	return "", errors.New("preview must not read login state")
}
func (*replicaPreviewCredentialStore) Set(string, string) error { return nil }
func (*replicaPreviewCredentialStore) Delete(string) error      { return nil }

type replicaPreviewUpdater struct{}

func (*replicaPreviewUpdater) RecoverActivationWhileLocked(context.Context) error { return nil }
func (*replicaPreviewUpdater) EnsureLauncher(context.Context) (updatepkg.TargetResult, error) {
	return updatepkg.TargetResult{}, nil
}
func (*replicaPreviewUpdater) Check(context.Context) (updatepkg.CheckResult, error) {
	return updatepkg.CheckResult{}, nil
}
func (*replicaPreviewUpdater) Apply(context.Context, updatepkg.CheckResult, updatepkg.ApplyOptions) (updatepkg.ApplyResult, error) {
	return updatepkg.ApplyResult{}, nil
}

type replicaPreviewRoundTripper struct{ calls atomic.Int32 }

func (transport *replicaPreviewRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	transport.calls.Add(1)
	return nil, errors.New("preview must not call a remote API")
}

func TestReplicaPreviewOpensTheAnonymousShellWithoutAuthUploadOrSourceWrites(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "invalid-token-that-must-not-be-read")
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0o700); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(project, "index.html")
	const source = "<h1>unchanged</h1>"
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	store := &replicaPreviewCredentialStore{}
	transport := &replicaPreviewRoundTripper{}
	session := &replicaPreviewSessionStub{result: replicapreview.Result{
		TargetURL:    "http://127.0.0.1:4173/",
		Reused:       true,
		ServiceKind:  replicapreview.ServiceExisting,
		StartedByCLI: false,
		Performance: replicapreview.Performance{
			Applicable: true,
			Goal:       10 * time.Second,
			ReadyAfter: 25 * time.Millisecond,
		},
	}}
	var startedOptions replicapreview.Options
	var opened string
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Execute([]string{"replica", "preview", "--url", "http://127.0.0.1:4173/"}, Dependencies{
		Out: &stdout, ErrOut: &stderr,
		HTTPClient: &http.Client{Transport: transport},
		Store:      store,
		Updater:    &replicaPreviewUpdater{},
		Environment: skillcontent.Environment{
			Home: root, ConfigDir: filepath.Join(root, "config"),
		},
		Region: config.RegionCN,
		StartReplicaPreview: func(_ context.Context, options replicapreview.Options) (replicapreview.Running, error) {
			startedOptions = options
			return session, nil
		},
		OpenURL: func(_ context.Context, target string) error {
			opened = target
			return nil
		},
	})
	if exit != 0 {
		t.Fatalf("preview failed: exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
	if startedOptions.ProjectPath != "" || startedOptions.ExistingURL != "http://127.0.0.1:4173/" {
		t.Fatalf("preview inspected the wrong project: %#v", startedOptions)
	}
	if store.reads.Load() != 0 || transport.calls.Load() != 0 {
		t.Fatalf("preview crossed a remote/auth boundary: store=%d http=%d", store.reads.Load(), transport.calls.Load())
	}
	if !session.closed.Load() {
		t.Fatal("preview session was not closed")
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil || string(data) != source {
		t.Fatalf("preview modified source: data=%q err=%v", data, err)
	}
	if opened != "http://127.0.0.1:4173/" {
		t.Fatalf("unexpected local preview URL: %q", opened)
	}
	if strings.Contains(stdout.String(), "VICEME-REPLICA:") {
		t.Fatalf("preview generated a usable invitation: %q", stdout.String())
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Mode                        string `json:"mode"`
			PreviewURL                  string `json:"previewUrl"`
			PreviewOpened               bool   `json:"previewOpened"`
			PreviewVerified             bool   `json:"previewVerified"`
			BrowserVerificationRequired bool   `json:"browserVerificationRequired"`
			ReviewRequiredBy            string `json:"reviewRequiredBy"`
			RemoteUpload                bool   `json:"remoteUpload"`
			AuthenticationChecked       bool   `json:"authenticationChecked"`
			MerchantChecked             bool   `json:"merchantChecked"`
			PublicationCreated          bool   `json:"publicationCreated"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid command output: %v: %s", err, stdout.String())
	}
	if !envelope.OK || envelope.Data.Mode != "LOCAL_PREVIEW" || envelope.Data.PreviewURL != opened || !envelope.Data.PreviewOpened ||
		envelope.Data.PreviewVerified || !envelope.Data.BrowserVerificationRequired || envelope.Data.ReviewRequiredBy != "CREATOR" ||
		envelope.Data.RemoteUpload || envelope.Data.AuthenticationChecked || envelope.Data.MerchantChecked || envelope.Data.PublicationCreated {
		t.Fatalf("unexpected preview boundary output: %#v", envelope)
	}
}

func TestReplicaPreviewReportsCleanupFailureBeforeSuccess(t *testing.T) {
	root := t.TempDir()
	cleanupErr := errors.New("process tree is still running")
	session := &replicaPreviewSessionStub{
		result: replicapreview.Result{
			TargetURL:   "http://127.0.0.1:4173/",
			Performance: replicapreview.Performance{Applicable: true, Goal: 10 * time.Second},
		},
		closeErr: cleanupErr,
	}
	var stdout bytes.Buffer
	exit := Execute([]string{"replica", "preview", "--url", "http://127.0.0.1:4173/"}, Dependencies{
		Out: &stdout, ErrOut: &bytes.Buffer{}, Store: securestore.NewMemory(), Updater: &replicaPreviewUpdater{},
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN,
		StartReplicaPreview: func(context.Context, replicapreview.Options) (replicapreview.Running, error) {
			return session, nil
		},
		OpenURL: func(context.Context, string) error { return nil },
	})
	if exit != 10 || !session.closed.Load() {
		t.Fatalf("cleanup failure was not propagated: exit=%d closed=%t output=%q", exit, session.closed.Load(), stdout.String())
	}
	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("cleanup failure emitted more than one protocol envelope: %v: %q", err, stdout.String())
	}
	if envelope.OK || envelope.Error.Code != "CONFIRM_UNVERIFIED_REPLICA_ONLY" {
		t.Fatalf("unexpected cleanup failure envelope: %#v", envelope)
	}
}

func TestBrowserEnvironmentDoesNotExposeTheProcessCredential(t *testing.T) {
	environment := environmentWithoutProcessAccessToken([]string{
		"PATH=/usr/bin",
		"VICEME_ACCESS_TOKEN=secret",
		"viceme_access_token=case-insensitive-secret",
		"HOME=/home/creator",
	})
	joined := strings.Join(environment, "\n")
	if strings.Contains(strings.ToUpper(joined), processAccessTokenEnvironment) {
		t.Fatalf("browser environment retained the process credential: %q", joined)
	}
	if !strings.Contains(joined, "PATH=/usr/bin") || !strings.Contains(joined, "HOME=/home/creator") {
		t.Fatalf("browser environment lost unrelated values: %q", joined)
	}
}

func TestReplicaPreviewFailureRequiresAnExplicitReplicaOnlyConfirmation(t *testing.T) {
	root := t.TempDir()
	var stdout bytes.Buffer
	exit := Execute([]string{"replica", "preview", "--path", root}, Dependencies{
		Out: &stdout, ErrOut: &bytes.Buffer{}, Store: securestore.NewMemory(), Updater: &replicaPreviewUpdater{},
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN,
		StartReplicaPreview: func(context.Context, replicapreview.Options) (replicapreview.Running, error) {
			return nil, &replicapreview.StartError{Code: "REPLICA_PREVIEW_START_FAILED", Stage: replicapreview.StageStarting, Message: "project dev script exited before preview became ready"}
		},
	})
	if exit == 0 || !strings.Contains(stdout.String(), "CONFIRM_UNVERIFIED_REPLICA_ONLY") ||
		!strings.Contains(stdout.String(), "--confirm-unverified-replica-only") ||
		!strings.Contains(stdout.String(), "preview interaction") {
		t.Fatalf("preview failure did not expose the confirmation boundary: exit=%d output=%q", exit, stdout.String())
	}
}

func TestReplicaOnlyConfirmationDoesNotStartPreviewOrPerformRemoteWork(t *testing.T) {
	root := t.TempDir()
	var stdout bytes.Buffer
	transport := &replicaPreviewRoundTripper{}
	exit := Execute([]string{"replica", "preview", "--confirm-unverified-replica-only"}, Dependencies{
		Out: &stdout, ErrOut: &bytes.Buffer{}, HTTPClient: &http.Client{Transport: transport},
		Store: securestore.NewMemory(), Updater: &replicaPreviewUpdater{},
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN,
		StartReplicaPreview: func(context.Context, replicapreview.Options) (replicapreview.Running, error) {
			t.Fatal("confirmed Replica-only path started a preview")
			return nil, nil
		},
		OpenURL: func(context.Context, string) error {
			t.Fatal("confirmed Replica-only path opened a browser")
			return nil
		},
	})
	if exit != 0 || transport.calls.Load() != 0 || !strings.Contains(stdout.String(), `"mode": "REPLICA_ONLY"`) ||
		!strings.Contains(stdout.String(), `"previewVerified": false`) || !strings.Contains(stdout.String(), `"hostingRequested": false`) {
		t.Fatalf("unexpected Replica-only confirmation: exit=%d output=%q HTTP=%d", exit, stdout.String(), transport.calls.Load())
	}
}
