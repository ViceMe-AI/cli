package command

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/publication"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

const deliverTestOriginalSkill = `---
name: deliver-demo
description: Channel delivery round-trip fixture.
---

# Deliver Demo

Author business body with instructions.
`

// TestPublicationDeliverAppliesTrialChannelAndRestoresOnRepublish covers the
// full author loop of the channel delivery contract: deliver applies the
// official gate onto the original path, re-running is idempotent, local edits
// to generated files are preserved as conflicts, and a later publish of the
// gated directory uploads the restored original package.
func TestPublicationDeliverAppliesTrialChannelAndRestoresOnRepublish(t *testing.T) {
	t.Parallel()
	const publicationID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	const productID = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	const listingID = "66666666-6666-4666-8666-666666666666"
	const releaseID = publicationID

	root := t.TempDir()
	source := filepath.Join(root, "skill")
	if err := os.MkdirAll(filepath.Join(source, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(deliverTestOriginalSkill), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "scripts", "run.sh"), []byte("#!/bin/sh\necho demo\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	pristine, err := publication.Build(source)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/cli/auth/status":
			writeJSONResponse(writer, map[string]any{
				"authenticated": true,
				"user":          map[string]any{"id": "55555555-5555-4555-8555-555555555555", "displayName": "Creator", "avatarUrl": nil},
				"scopes":        []string{"profile:read", "skill-publication:read", "skill-publication:write"},
				"expiresAt":     "2027-08-27T00:00:00Z",
			})
		case request.Method == http.MethodGet && request.URL.Path == "/v1/creator/skill-publications/"+publicationID:
			writeJSONResponse(writer, map[string]any{
				"id": publicationID, "listingId": listingID, "status": "PUBLISHED",
				"product": map[string]any{"id": productID, "slug": "deliver-demo", "detailUrl": "https://viceme.cn/creator/deliver-demo", "releaseId": releaseID},
				"editions": []any{},
			})
		case request.Method == http.MethodGet && request.URL.Path == "/v1/creator/skill-publications/"+publicationID+"/package":
			writeJSONResponse(writer, map[string]any{
				"url":            "http://" + request.Host + "/artifact/deliver-demo.zip",
				"fileName":       "deliver-demo.zip",
				"releaseId":      releaseID,
				"artifactDigest": pristine.Artifact.Digest,
				"expiresAt":      "2027-08-27T00:00:00Z",
			})
		case request.Method == http.MethodGet && request.URL.Path == "/artifact/deliver-demo.zip":
			writer.Header().Set("Content-Type", "application/zip")
			_, _ = writer.Write(pristine.Bytes)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	store := securestore.NewMemory()
	scope, err := credentialScopeForAPIBase(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{Store: store, Region: "global", ProfileID: "default", ProfileName: "default", Scope: scope}
	if err := manager.Save(credentialauth.Credential{AccessToken: "vme_cli_test", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	dependencies := Dependencies{
		Out: &stdout, ErrOut: &stdout, Store: store, APIBaseURL: server.URL, Region: config.RegionGlobal,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Now:         func() time.Time { return time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC) },
	}
	execute := func(arguments ...string) (int, map[string]any) {
		t.Helper()
		stdout.Reset()
		exit := Execute(arguments, dependencies)
		var envelope map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("command did not emit one JSON envelope: exit=%d stdout=%q err=%v", exit, stdout.String(), err)
		}
		return exit, envelope
	}

	exit, envelope := execute("publication", "deliver", publicationID, "--skill-dir", source)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("first delivery failed: exit=%d envelope=%v", exit, envelope)
	}
	data, _ := envelope["data"].(map[string]any)
	if data["branch"] != "viceme-skill-deliver-demo" || data["trialBodyEditPosition"] != ".viceme/trial-body.md" {
		t.Fatalf("delivery identity fields are wrong: %#v", data)
	}
	gatedSkill, err := os.ReadFile(filepath.Join(source, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !containsGate(gatedSkill, productID) || bytes.Contains(gatedSkill, []byte("Author business body")) {
		t.Fatalf("SKILL.md was not gated: %q", gatedSkill)
	}
	trialBody, err := os.ReadFile(filepath.Join(source, ".viceme", "trial-body.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(trialBody) != deliverTestOriginalSkill {
		t.Fatalf("trial body does not preserve the authored document verbatim: %q", trialBody)
	}
	runtimeManifest, err := os.ReadFile(filepath.Join(source, ".viceme", "runtime.json"))
	if err != nil {
		t.Fatal(err)
	}
	identity := struct {
		ProductID string `json:"productId"`
		ReleaseID string `json:"releaseId"`
		Kind      string `json:"kind"`
		Market    string `json:"market"`
		Files     map[string]string `json:"files"`
	}{}
	if err := json.Unmarshal(runtimeManifest, &identity); err != nil {
		t.Fatal(err)
	}
	if identity.ProductID != productID || identity.ReleaseID != releaseID || identity.Kind != "trial" || identity.Market != "global" {
		t.Fatalf("runtime manifest identity is wrong: %s", runtimeManifest)
	}
	if _, ok := identity.Files[skillcontent.TrialBodyPath]; !ok {
		t.Fatalf("runtime manifest does not cover the trial body: %s", runtimeManifest)
	}
	if _, err := os.Stat(filepath.Join(source, skillcontent.TrialRuntimePath)); err != nil {
		t.Fatalf("generated runtime reference missing: %v", err)
	}
	script, err := os.ReadFile(filepath.Join(source, "scripts", "run.sh"))
	if err != nil || string(script) != "#!/bin/sh\necho demo\n" {
		t.Fatalf("author business file was touched: %q err=%v", script, err)
	}
	zipInfo, _ := data["zip"].(map[string]any)
	zipPath, _ := zipInfo["path"].(string)
	if zipPath != filepath.Join(root, "skill-viceme-trial-channel.zip") {
		t.Fatalf("channel ZIP default path is wrong: %#v", zipInfo)
	}
	if _, err := os.Stat(zipPath); err != nil {
		t.Fatalf("channel ZIP was not written: %v", err)
	}

	exit, envelope = execute("publication", "deliver", publicationID, "--skill-dir", source)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("idempotent re-delivery failed: exit=%d envelope=%v", exit, envelope)
	}
	data, _ = envelope["data"].(map[string]any)
	if list, _ := data["written"].([]any); len(list) != 0 {
		t.Fatalf("no-change re-delivery rewrote files: %#v", data["written"])
	}

	// Author edits a generated file by hand: the delivery must preserve it and
	// report a conflict instead of silently overwriting their work.
	environmentPath := filepath.Join(source, ".viceme", "environment.json")
	if err := os.WriteFile(environmentPath, []byte("{\"market\":\"global\"}\n// hand edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exit, envelope = execute("publication", "deliver", publicationID, "--skill-dir", source)
	if exit == 0 || envelope["ok"] != false {
		t.Fatalf("local edit did not surface as a conflict: exit=%d envelope=%v", exit, envelope)
	}
	failure, _ := envelope["error"].(map[string]any)
	if failure["code"] != "SKILL_CHANNEL_LOCAL_CONFLICT" {
		t.Fatalf("unexpected conflict code: %#v", failure)
	}
	preserved, err := os.ReadFile(environmentPath)
	if err != nil || string(preserved) != "{\"market\":\"global\"}\n// hand edit\n" {
		t.Fatalf("conflicting local edit was not preserved: %q err=%v", preserved, err)
	}

	// Publishing the gated directory must upload the restored original: the
	// rebuilt package digest equals the pristine one and no generated file
	// leaks into it.
	restored, err := publication.Build(source)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Artifact.Digest != pristine.Artifact.Digest {
		t.Fatalf("restored package digest drifted: %s != %s", restored.Artifact.Digest, pristine.Artifact.Digest)
	}
	if restored.FileCount != pristine.FileCount {
		t.Fatalf("restored package file count drifted: %d != %d", restored.FileCount, pristine.FileCount)
	}
}

// TestPublicationDeliverRejectsForeignBinding proves listing identity is
// verified through the local binding, not through names or paths.
func TestPublicationDeliverRejectsForeignBinding(t *testing.T) {
	t.Parallel()
	const publicationID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	const productID = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	const listingID = "66666666-6666-4666-8666-666666666666"

	root := t.TempDir()
	source := filepath.Join(root, "skill")
	if err := os.MkdirAll(filepath.Join(source, ".viceme"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(deliverTestOriginalSkill), 0o644); err != nil {
		t.Fatal(err)
	}
	binding := publication.SkillBinding{
		APIVersion: publication.BindingAPIVersion, Kind: "SkillListing",
		ListingID: "77777777-7777-4777-8777-777777777777", ClientWorkID: "work",
		Market: "global", EndpointOrigin: "", BindingReceipt: "receipt",
	}
	data, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, ".viceme", "skill.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/cli/auth/status":
			writeJSONResponse(writer, map[string]any{
				"authenticated": true,
				"user":          map[string]any{"id": "55555555-5555-4555-8555-555555555555", "displayName": "Creator", "avatarUrl": nil},
				"scopes":        []string{"profile:read", "skill-publication:read", "skill-publication:write"},
				"expiresAt":     "2027-08-27T00:00:00Z",
			})
		case request.Method == http.MethodGet && request.URL.Path == "/v1/creator/skill-publications/"+publicationID:
			writeJSONResponse(writer, map[string]any{
				"id": publicationID, "listingId": listingID, "status": "PUBLISHED",
				"product": map[string]any{"id": productID, "slug": "deliver-demo", "detailUrl": "https://viceme.cn/creator/deliver-demo", "releaseId": publicationID},
				"editions": []any{},
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	store := securestore.NewMemory()
	scope, err := credentialScopeForAPIBase(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{Store: store, Region: "global", ProfileID: "default", ProfileName: "default", Scope: scope}
	if err := manager.Save(credentialauth.Credential{AccessToken: "vme_cli_test", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	binding.EndpointOrigin = server.URL
	bound, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, ".viceme", "skill.json"), bound, 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	dependencies := Dependencies{
		Out: &stdout, ErrOut: &stdout, Store: store, APIBaseURL: server.URL, Region: config.RegionGlobal,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Now:         func() time.Time { return time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC) },
	}
	stdout.Reset()
	exit := Execute([]string{"publication", "deliver", publicationID, "--skill-dir", source}, dependencies)
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("command did not emit one JSON envelope: exit=%d stdout=%q err=%v", exit, stdout.String(), err)
	}
	if exit == 0 || envelope["ok"] != false {
		t.Fatalf("foreign binding did not fail the delivery: exit=%d envelope=%v", exit, envelope)
	}
	failure, _ := envelope["error"].(map[string]any)
	if failure["code"] != "SKILL_CHANNEL_BINDING_MISMATCH" {
		t.Fatalf("unexpected binding failure code: %#v", failure)
	}
	if _, err := os.Stat(filepath.Join(source, ".viceme", "runtime.json")); !os.IsNotExist(err) {
		t.Fatalf("delivery must not touch the directory after a binding mismatch: %v", err)
	}
}

func containsGate(skill []byte, productID string) bool {
	return bytes.Contains(skill, []byte("<!-- viceme-trial:v1 product="+productID+" -->"))
}
