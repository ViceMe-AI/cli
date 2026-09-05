package command

import (
	"context"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/ViceMe-AI/cli/internal/replicacontent"
	"github.com/ViceMe-AI/cli/internal/replicapreview"
	"github.com/ViceMe-AI/cli/internal/replicapublication"
)

func TestReplicaStorageDenialStopsBeforePreview(t *testing.T) {
	project := newReplicaPublicationTestProject(t)
	root := t.TempDir()
	original := privatefile.ReplaceFile
	privatefile.ReplaceFile = func(from, to string) error {
		if strings.HasPrefix(to, root+string(filepath.Separator)) {
			return syscall.EPERM
		}
		return original(from, to)
	}
	t.Cleanup(func() { privatefile.ReplaceFile = original })
	previewCalls := 0
	runtime := &Runtime{apiBaseURL: "https://api.viceme.cn", configBase: root, profile: config.Profile{MarketRegion: config.RegionCN}, deps: Dependencies{
		OpenURL: func(context.Context, string) error { return nil }, Now: time.Now, ErrOut: io.Discard, NewID: func() string { return replicaPublicationTestRequestID },
		StartReplicaPreview: func(context.Context, replicapreview.Options) (replicapreview.Running, error) {
			previewCalls++
			return &replicaPreviewSessionStub{result: replicapreview.Result{TargetURL: "http://127.0.0.1:4173/", Reused: true, ServiceKind: replicapreview.ServiceExisting}}, nil
		},
	}}
	_, err := publishWebsiteReplica(context.Background(), runtime, replicaPublishOptions{ProjectPath: project, Slug: "replica-site", Title: "Replica title", PriceCents: 1, PreviewURL: "http://127.0.0.1:4173/", PreviewReviewed: true})
	failure := output.AsError(err)
	if previewCalls != 0 || failure.Subtype != "REPLICA_PUBLICATION_STORAGE_PERMISSION_REQUIRED" {
		t.Fatalf("storage denial did not stop before preview: previewCalls=%d error=%v", previewCalls, err)
	}
}

func TestReplicaStorageSelectionPinsExistingRequest(t *testing.T) {
	project := newReplicaPublicationTestProject(t)
	runtime := &Runtime{configBase: t.TempDir(), apiBaseURL: "https://api.viceme.cn", profile: config.Profile{Name: "default", MarketRegion: config.RegionCN}, deps: Dependencies{Now: time.Now}}
	fingerprint, canonical, err := replicapublication.ProjectFingerprint(runtime.apiBaseURL, "CN", project)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := replicacontent.FreezeSourceArchive(canonical, replicacontent.FreezeSourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer frozen.Cleanup()
	pending := replicapublication.Pending{EndpointOrigin: runtime.apiBaseURL, Market: "CN", ProjectPath: canonical, ProjectFingerprint: fingerprint, ClientRequestID: replicaPublicationTestRequestID,
		Request: api.CreateWebsiteReplicaPublicationRequest{ProtocolVersion: 2, Market: "CN", ClientRequestID: replicaPublicationTestRequestID, ProjectFingerprint: fingerprint, Source: api.WebsiteReplicaPublicationSourceArtifact{Digest: frozen.Summary.Digest, SizeBytes: frozen.Summary.SizeBytes}}, SourceArchive: frozen.Summary, ArtifactExpiresAt: time.Now().Add(time.Minute), Hosting: "REPLICA_ONLY"}
	global := replicaPublicationStore(runtime)
	if err := global.Save(&pending); err != nil {
		t.Fatal(err)
	}
	runtime.replicaProject = canonical
	if _, _, err := prepareReplicaPublishStore(runtime, canonical, fingerprint); err == nil || output.AsError(err).Subtype != "REPLICA_PUBLICATION_STORAGE_CONFLICT" {
		t.Fatalf("existing global request moved: %v", err)
	}
	if err := global.Delete(pending); err != nil {
		t.Fatal(err)
	}
	local, err := projectReplicaPublicationStore(runtime, canonical)
	if err != nil {
		t.Fatal(err)
	}
	if err := local.Save(&pending); err != nil {
		t.Fatal(err)
	}
	runtime.replicaProject = ""
	selected, unlock, err := prepareReplicaPublishStore(runtime, canonical, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	if selected.Directory != local.Directory || runtime.replicaProject != canonical {
		t.Fatal("project state was not rediscovered")
	}
	if err := global.Save(&pending); err != nil {
		t.Fatal(err)
	}
	if _, _, err := prepareReplicaPublishStore(runtime, canonical, fingerprint); err == nil {
		t.Fatal("ambiguous dual state was accepted")
	}
	for _, store := range []replicapublication.Store{global, local} {
		loaded, found, err := store.LoadProject(fingerprint)
		if err != nil || !found || loaded.ClientRequestID != pending.ClientRequestID {
			t.Fatal("conflict damaged existing identity")
		}
	}
}

func TestReplicaStorageRejectsProjectAliasAndSymlink(t *testing.T) {
	project := newReplicaPublicationTestProject(t)
	other := newReplicaPublicationTestProject(t)
	before, _ := os.ReadDir(other)
	runtime := &Runtime{configBase: t.TempDir(), apiBaseURL: "https://api.viceme.cn", profile: config.Profile{MarketRegion: config.RegionCN}, deps: Dependencies{Now: time.Now}, replicaProject: other}
	fp, canonical, err := replicapublication.ProjectFingerprint(runtime.apiBaseURL, "CN", project)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := prepareReplicaPublishStore(runtime, canonical, fp); err == nil || output.AsError(err).Subtype != "REPLICA_STORAGE_PROJECT_MISMATCH" {
		t.Fatalf("unrelated project accepted: %v", err)
	}
	if err := os.Symlink(other, filepath.Join(project, ".viceme")); err != nil {
		if goruntime.GOOS == "windows" {
			t.Skipf("symlink privilege unavailable: %v", err)
		}
		t.Fatal(err)
	}
	runtime.replicaProject = project
	if _, _, err := prepareReplicaPublishStore(runtime, canonical, fp); err == nil {
		t.Fatal("symlinked storage accepted")
	}
	entries, err := os.ReadDir(other)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(entries) {
		t.Fatal("symlink target mutated")
	}
	for i, entry := range entries {
		if entry.Name() != before[i].Name() {
			t.Fatal("symlink target mutated")
		}
	}
}

func TestReplicaStorageSerializesBothModes(t *testing.T) {
	project := newReplicaPublicationTestProject(t)
	runtime := &Runtime{configBase: t.TempDir(), apiBaseURL: "https://api.viceme.cn", profile: config.Profile{MarketRegion: config.RegionCN}, deps: Dependencies{Now: time.Now}}
	fp, canonical, err := replicapublication.ProjectFingerprint(runtime.apiBaseURL, "CN", project)
	if err != nil {
		t.Fatal(err)
	}
	_, release, err := prepareReplicaPublishStore(runtime, canonical, fp)
	if err != nil {
		t.Fatal(err)
	}
	second := *runtime
	second.replicaProject = canonical
	done := make(chan error, 1)
	go func() {
		_, release, err := prepareReplicaPublishStore(&second, canonical, fp)
		if err == nil {
			err = release()
		}
		done <- err
	}()
	select {
	case err := <-done:
		_ = release()
		t.Fatalf("second storage mode bypassed the lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("storage lock was not released")
	}
}
