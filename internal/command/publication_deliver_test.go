package command

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/channeldelivery"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/publication"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

const deliverAuthorBody = `---
name: deliver-demo
description: Channel delivery round-trip fixture.
---

# Deliver Demo

Author business body with instructions.
`

// deliverFixture prepares one authored Skill directory under root and its
// built package. The CLI home/config live at root level, never inside the
// Skill directory.
func deliverFixture(t *testing.T, root, body string) (string, publication.Package) {
	t.Helper()
	source := filepath.Join(root, "skill")
	if err := os.MkdirAll(filepath.Join(source, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "scripts", "run.sh"), []byte("#!/bin/sh\necho demo\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	pkg, err := publication.Build(source)
	if err != nil {
		t.Fatal(err)
	}
	return source, pkg
}

// deliverServerState backs the fake API: two publications of one product
// (first release and updated release) plus the consumer trial endpoints.
type deliverServerState struct {
	productID     string
	listingID     string
	publications  map[string]publicationBytes
	trialRequests []string
}

type publicationBytes struct {
	releaseID string
	pkg       publication.Package
}

func (state *deliverServerState) handler() http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/cli/auth/status":
			writeJSONResponse(writer, map[string]any{
				"authenticated": true,
				"user":          map[string]any{"id": "55555555-5555-4555-8555-555555555555", "displayName": "Creator", "avatarUrl": nil},
				"scopes":        []string{"profile:read", "skill-publication:read", "skill-publication:write"},
				"expiresAt":     "2027-08-27T00:00:00Z",
			})
		case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/v1/creator/skill-publications/") && strings.HasSuffix(request.URL.Path, "/package"):
			id := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/v1/creator/skill-publications/"), "/package")
			current, ok := state.publications[id]
			if !ok {
				http.NotFound(writer, request)
				return
			}
			writeJSONResponse(writer, map[string]any{
				"url": "http://" + request.Host + "/artifact/" + current.releaseID, "fileName": "deliver-demo.zip",
				"releaseId": current.releaseID, "artifactDigest": current.pkg.Artifact.Digest,
				"expiresAt": "2027-08-27T00:00:00Z",
			})
		case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/artifact/"):
			id := strings.TrimPrefix(request.URL.Path, "/artifact/")
			current, ok := state.publications[id]
			if !ok {
				http.NotFound(writer, request)
				return
			}
			writer.Header().Set("Content-Type", "application/zip")
			_, _ = writer.Write(current.pkg.Bytes)
		case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/v1/creator/skill-publications/"):
			id := strings.TrimPrefix(request.URL.Path, "/v1/creator/skill-publications/")
			current, ok := state.publications[id]
			if !ok {
				http.NotFound(writer, request)
				return
			}
			writeJSONResponse(writer, map[string]any{
				"id": id, "listingId": state.listingID, "status": "PUBLISHED",
				"product":  map[string]any{"id": state.productID, "slug": "deliver-demo", "detailUrl": "https://viceme.cn/creator/deliver-demo", "releaseId": current.releaseID},
				"editions": []any{},
			})
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/trial-grants"):
			state.trialRequests = append(state.trialRequests, request.URL.Path)
			writeJSONResponse(writer, map[string]any{
				"installId": "11111111-1111-4111-8111-111111111111", "secret": strings.Repeat("ab", 32),
				"limitUses": 3, "remainingUses": 3,
			})
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/trial-use"):
			state.trialRequests = append(state.trialRequests, request.URL.Path)
			writeJSONResponse(writer, map[string]any{
				"allowed": true, "remainingUses": 2, "limitUses": 3,
			})
		default:
			http.NotFound(writer, request)
		}
	}
}

type deliverHarness struct {
	server   *httptest.Server
	state    *deliverServerState
	root     string
	store    securestore.Store
	deps     Dependencies
	execute  func(arguments ...string) (int, map[string]any)
	stdout   *bytes.Buffer
	source   string
	firstID  string
	updateID string
}

func newDeliverHarness(t *testing.T, body string) *deliverHarness {
	t.Helper()
	const productID = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	const listingID = "66666666-6666-4666-8666-666666666666"
	root := t.TempDir()
	source, first := deliverFixture(t, root, body)
	updated := strings.Replace(body, "Author business body with instructions.", "Updated author business body.", 1)
	_, updatedPkg := deliverFixture(t, t.TempDir(), updated)
	state := &deliverServerState{
		productID: productID, listingID: listingID,
		publications: map[string]publicationBytes{
			"dddddddd-dddd-4ddd-8ddd-dddddddddddd": {releaseID: "dddddddd-dddd-4ddd-8ddd-dddddddddddd", pkg: first},
			"cccccccc-cccc-4ccc-8ccc-cccccccccccc": {releaseID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", pkg: updatedPkg},
		},
	}
	server := httptest.NewServer(state.handler())

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
	deps := Dependencies{
		Out: &stdout, ErrOut: &stdout, Store: store, APIBaseURL: server.URL, Region: config.RegionGlobal,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Now:         func() time.Time { return time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC) },
	}
	harness := &deliverHarness{
		server: server, state: state, root: root, store: store, deps: deps, stdout: &stdout,
		source: source, firstID: "dddddddd-dddd-4ddd-8ddd-dddddddddddd", updateID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
	}
	harness.execute = func(arguments ...string) (int, map[string]any) {
		t.Helper()
		stdout.Reset()
		exit := Execute(arguments, deps)
		var envelope map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("command did not emit one JSON envelope: exit=%d stdout=%q err=%v", exit, stdout.String(), err)
		}
		return exit, envelope
	}
	return harness
}

// writeRealBinding writes the sidecar exactly the publish flow produces:
// the server answers markets in upper case while the CLI region is lower
// case, which the delivery must accept.
func (h *deliverHarness) writeRealBinding(t *testing.T, directory string) {
	t.Helper()
	binding := publication.SkillBinding{
		APIVersion: publication.BindingAPIVersion, Kind: "SkillListing",
		ListingID: h.state.listingID, ClientWorkID: "88888888-8888-4888-8888-888888888888",
		Market: "GLOBAL", EndpointOrigin: h.server.URL, BindingReceipt: "receipt",
		LastPackageDigest: strings.Repeat("0a", 32),
	}
	data, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(directory, ".viceme"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ".viceme", "skill.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestPublicationDeliverAppliesTrialChannelAndRestoresOnRepublish covers the
// author loop of the channel delivery contract: a real-format binding passes,
// deliver applies the official gate plus the credential-free install identity,
// re-running is idempotent and keeps reporting the commit scope, the second
// publication inherits the baseline instead of conflicting, local edits stop
// the delivery, and a later publish of the gated directory uploads the
// restored original package.
func TestPublicationDeliverAppliesTrialChannelAndRestoresOnRepublish(t *testing.T) {
	t.Parallel()
	harness := newDeliverHarness(t, deliverAuthorBody)
	defer harness.server.Close()
	source := harness.source
	harness.writeRealBinding(t, source)

	exit, envelope := harness.execute("publication", "deliver", harness.firstID, "--skill-dir", source)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("first delivery failed against a real-format binding: exit=%d envelope=%v", exit, envelope)
	}
	data, _ := envelope["data"].(map[string]any)
	if data["branch"] != "viceme-skill-deliver-demo" || data["trialBodyEditPosition"] != ".viceme/trial-body.md" {
		t.Fatalf("delivery identity fields are wrong: %#v", data)
	}
	assertDeliveredChannel(t, source, harness.state.productID, "dddddddd-dddd-4ddd-8ddd-dddddddddddd", deliverAuthorBody, data)

	scope, _ := data["commitScope"].([]any)
	if len(scope) == 0 {
		t.Fatalf("first delivery reported an empty commit scope: %#v", data)
	}

	// A lost response re-run reports the same managed paths; the scope must
	// not collapse to "this run wrote nothing".
	exit, envelope = harness.execute("publication", "deliver", harness.firstID, "--skill-dir", source)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("idempotent re-delivery failed: exit=%d envelope=%v", exit, envelope)
	}
	data, _ = envelope["data"].(map[string]any)
	if written, _ := data["written"].([]any); len(written) != 0 {
		t.Fatalf("no-change re-delivery rewrote files: %#v", data["written"])
	}
	again, _ := data["commitScope"].([]any)
	if len(again) != len(scope) {
		t.Fatalf("commit scope disappeared on the re-run: %d vs %d", len(again), len(scope))
	}

	// Second publication of the same product: the record is keyed by product
	// and directory, so the previous baseline lets generated files move to the
	// new release instead of surfacing as conflicts.
	exit, envelope = harness.execute("publication", "deliver", harness.updateID, "--skill-dir", source)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("update delivery did not inherit the previous baseline: exit=%d envelope=%v", exit, envelope)
	}
	data, _ = envelope["data"].(map[string]any)
	assertDeliveredChannel(t, source, harness.state.productID, "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
		strings.Replace(deliverAuthorBody, "Author business body with instructions.", "Updated author business body.", 1), data)

	// Author edits a generated file by hand: preserved, reported, never overwritten.
	environmentPath := filepath.Join(source, ".viceme", "environment.json")
	if err := os.WriteFile(environmentPath, []byte("{\"market\":\"global\"}\n// hand edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exit, envelope = harness.execute("publication", "deliver", harness.updateID, "--skill-dir", source)
	if exit == 0 || envelope["ok"] != false {
		t.Fatalf("local edit did not surface as a conflict: exit=%d envelope=%v", exit, envelope)
	}
	failure, _ := envelope["error"].(map[string]any)
	if failure["code"] != "SKILL_CHANNEL_LOCAL_CONFLICT" {
		t.Fatalf("unexpected conflict code: %#v", failure)
	}
	if preserved, err := os.ReadFile(environmentPath); err != nil || string(preserved) != "{\"market\":\"global\"}\n// hand edit\n" {
		t.Fatalf("conflicting local edit was not preserved: %q err=%v", preserved, err)
	}

	// Publishing the gated directory must upload the restored original.
	restored, err := publication.Build(source)
	if err != nil {
		t.Fatal(err)
	}
	current := harness.state.publications[harness.updateID].pkg
	if restored.Artifact.Digest != current.Artifact.Digest {
		t.Fatalf("restored package does not match the published update: %s != %s", restored.Artifact.Digest, current.Artifact.Digest)
	}
}

// TestPublicationDeliverBlocksWhileAnotherDeliveryHoldsTheLock proves the
// directory mutex is taken before any local state is read or written.
func TestPublicationDeliverBlocksWhileAnotherDeliveryHoldsTheLock(t *testing.T) {
	t.Parallel()
	harness := newDeliverHarness(t, deliverAuthorBody)
	defer harness.server.Close()
	source := harness.source

	store := channeldelivery.Store{Directory: filepath.Join(harness.root, "config", "channel-deliveries"), EndpointOrigin: harness.server.URL}
	unlock, err := store.Lock(source)
	if err != nil {
		t.Fatal(err)
	}
	exit, envelope := harness.execute("publication", "deliver", harness.firstID, "--skill-dir", source)
	if exit == 0 || envelope["ok"] != false {
		t.Fatalf("delivery did not stop on the held lock: exit=%d envelope=%v", exit, envelope)
	}
	failure, _ := envelope["error"].(map[string]any)
	if failure["code"] != "SKILL_CHANNEL_DELIVERY_IN_PROGRESS" {
		t.Fatalf("unexpected lock failure code: %#v", failure)
	}
	if skill, err := os.ReadFile(filepath.Join(source, "SKILL.md")); err != nil || string(skill) != deliverAuthorBody {
		t.Fatalf("blocked delivery still wrote the gate entry: %q %v", skill, err)
	}
	if _, err := os.Stat(filepath.Join(harness.root, "skill-viceme-trial-channel.zip")); !os.IsNotExist(err) {
		t.Fatalf("blocked delivery still wrote the channel ZIP: %v", err)
	}
	records, err := filepath.Glob(filepath.Join(harness.root, "config", "channel-deliveries", "*", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("blocked delivery still persisted delivery state: %v", records)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	exit, envelope = harness.execute("publication", "deliver", harness.firstID, "--skill-dir", source)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("delivery did not proceed after the lock was released: exit=%d envelope=%v", exit, envelope)
	}
}

// TestPublicationDeliverStopsOnUnpublishedAuthorFiles proves the directory,
// the channel ZIP, and an author-packed ZIP cannot fork: local authored
// content that the target release does not contain blocks the delivery.
func TestPublicationDeliverStopsOnUnpublishedAuthorFiles(t *testing.T) {
	t.Parallel()
	harness := newDeliverHarness(t, deliverAuthorBody)
	defer harness.server.Close()
	source := harness.source

	exit, _ := harness.execute("publication", "deliver", harness.firstID, "--skill-dir", source)
	if exit != 0 {
		t.Fatal("first delivery failed")
	}
	if err := os.WriteFile(filepath.Join(source, "scripts", "run.sh"), []byte("#!/bin/sh\necho edited\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	exit, envelope := harness.execute("publication", "deliver", harness.firstID, "--skill-dir", source)
	if exit == 0 || envelope["ok"] != false {
		t.Fatalf("unpublished author edit did not block the delivery: exit=%d envelope=%v", exit, envelope)
	}
	failure, _ := envelope["error"].(map[string]any)
	if failure["code"] != "SKILL_CHANNEL_LOCAL_CONFLICT" {
		t.Fatalf("unexpected code for unpublished author work: %#v", failure)
	}
	details, _ := failure["details"].(map[string]any)
	conflicts, _ := details["conflicts"].([]any)
	found := false
	for _, item := range conflicts {
		if conflict, _ := item.(map[string]any); conflict != nil && conflict["path"] == "scripts/run.sh" {
			found = true
		}
	}
	if !found {
		t.Fatalf("conflicts did not name the drifted author file: %#v", details["conflicts"])
	}
}

// TestPublicationDeliverRejectsForeignBinding proves listing identity is
// verified through the local binding, not through names or paths.
func TestPublicationDeliverRejectsForeignBinding(t *testing.T) {
	t.Parallel()
	harness := newDeliverHarness(t, deliverAuthorBody)
	defer harness.server.Close()
	source := harness.source
	binding := publication.SkillBinding{
		APIVersion: publication.BindingAPIVersion, Kind: "SkillListing",
		ListingID: "77777777-7777-4777-8777-777777777777", ClientWorkID: "88888888-8888-4888-8888-888888888888",
		Market: "GLOBAL", EndpointOrigin: harness.server.URL, BindingReceipt: "receipt",
		LastPackageDigest: strings.Repeat("0a", 32),
	}
	data, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, ".viceme"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, ".viceme", "skill.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	exit, envelope := harness.execute("publication", "deliver", harness.firstID, "--skill-dir", source)
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

func assertDeliveredChannel(t *testing.T, source, productID, releaseID, authoredBody string, data map[string]any) {
	t.Helper()
	gatedSkill, err := os.ReadFile(filepath.Join(source, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !containsGate(gatedSkill, productID) || bytes.Contains(gatedSkill, []byte("business body")) {
		t.Fatalf("SKILL.md was not gated: %q", gatedSkill)
	}
	trialBody, err := os.ReadFile(filepath.Join(source, ".viceme", "trial-body.md"))
	if err != nil || string(trialBody) != authoredBody {
		t.Fatalf("trial body does not preserve the authored document verbatim: %q err=%v", trialBody, err)
	}
	manifest, err := os.ReadFile(filepath.Join(source, ".viceme", "runtime.json"))
	if err != nil {
		t.Fatal(err)
	}
	identity := struct {
		ProductID string            `json:"productId"`
		ReleaseID string            `json:"releaseId"`
		Kind      string            `json:"kind"`
		Market    string            `json:"market"`
		Files     map[string]string `json:"files"`
	}{}
	if err := json.Unmarshal(manifest, &identity); err != nil {
		t.Fatal(err)
	}
	if identity.ProductID != productID || identity.ReleaseID != releaseID || identity.Kind != "trial" || identity.Market != "global" {
		t.Fatalf("runtime manifest identity is wrong: %s", manifest)
	}
	install, err := os.ReadFile(filepath.Join(source, filepath.FromSlash(skillcontent.InstallManifestPath)))
	if err != nil {
		t.Fatalf("channel package lacks the install identity: %v", err)
	}
	owner := struct {
		ProductID string `json:"product_id"`
		ReleaseID string `json:"release_id"`
		InstallID string `json:"install_id"`
	}{ProductID: productID, ReleaseID: releaseID}
	if err := json.Unmarshal(install, &owner); err != nil || owner.ProductID != productID || owner.ReleaseID != releaseID {
		t.Fatalf("install identity is wrong: %s", install)
	}
	script, err := os.ReadFile(filepath.Join(source, "scripts", "run.sh"))
	if err != nil || string(script) != "#!/bin/sh\necho demo\n" {
		t.Fatalf("author business file was touched: %q err=%v", script, err)
	}
	zipInfo, _ := data["zip"].(map[string]any)
	zipPath, _ := zipInfo["path"].(string)
	if zipPath == "" {
		t.Fatalf("channel ZIP path missing: %#v", data)
	}
	archive, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	names := map[string]bool{}
	for _, entry := range archive.File {
		names[entry.Name] = true
	}
	for _, required := range []string{"SKILL.md", skillcontent.TrialBodyPath, skillcontent.InstallManifestPath, skillcontent.TrialRuntimePath, "scripts/run.sh"} {
		if !names[required] {
			t.Fatalf("channel ZIP lacks %s: %v", required, names)
		}
	}
}

func containsGate(skill []byte, productID string) bool {
	return bytes.Contains(skill, []byte("<!-- viceme-trial:v1 product="+productID+" -->"))
}

// TestPublicationDeliverLocksOnPhysicalDirectoryIdentity proves a parent-path
// alias cannot fork the delivery lock: a delivery invoked through a symbolic
// link above the Skill directory blocks against a lock taken on the physical
// path, and a successful delivery through the alias keys one record that the
// physical path finds again.
func TestPublicationDeliverLocksOnPhysicalDirectoryIdentity(t *testing.T) {
	t.Parallel()
	harness := newDeliverHarness(t, deliverAuthorBody)
	defer harness.server.Close()
	root := harness.root
	physical := filepath.Join(root, "real", "skill")
	if err := os.MkdirAll(filepath.Join(physical, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(physical, "SKILL.md"), []byte(deliverAuthorBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(physical, "scripts", "run.sh"), []byte("#!/bin/sh\necho demo\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	aliasParent := filepath.Join(root, "alias")
	if err := os.Symlink(filepath.Join(root, "real"), aliasParent); err != nil {
		t.Fatal(err)
	}
	aliased := filepath.Join(aliasParent, "skill")

	store := channeldelivery.Store{Directory: filepath.Join(root, "config", "channel-deliveries"), EndpointOrigin: harness.server.URL}
	unlock, err := store.Lock(physical)
	if err != nil {
		t.Fatal(err)
	}
	exit, envelope := harness.execute("publication", "deliver", harness.firstID, "--skill-dir", aliased)
	if exit == 0 || envelope["ok"] != false {
		t.Fatalf("alias path bypassed the physical lock: exit=%d envelope=%v", exit, envelope)
	}
	failure, _ := envelope["error"].(map[string]any)
	if failure["code"] != "SKILL_CHANNEL_DELIVERY_IN_PROGRESS" {
		t.Fatalf("unexpected lock failure code: %#v", failure)
	}
	if _, err := os.Stat(filepath.Join(physical, ".viceme", "runtime.json")); !os.IsNotExist(err) {
		t.Fatalf("alias delivery wrote through a held lock: %v", err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}

	exit, envelope = harness.execute("publication", "deliver", harness.firstID, "--skill-dir", aliased)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("delivery through the alias failed after unlock: exit=%d envelope=%v", exit, envelope)
	}
	records, err := filepath.Glob(filepath.Join(root, "config", "channel-deliveries", "*", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("alias and physical paths must share one record: %v", records)
	}
	if _, exists, err := (channeldelivery.Store{Directory: filepath.Join(root, "config", "channel-deliveries"), EndpointOrigin: harness.server.URL}).Load(harness.state.productID, physical); err != nil || !exists {
		t.Fatalf("physical path lost the record written through the alias: %v %v", exists, err)
	}
}

// TestPublicationDeliverKeepsRemovedManagedPathsInScope proves deletions of
// previously delivered managed files stay in the commit scope across re-runs,
// so a lost response cannot leave an obsolete generated file on the branch.
func TestPublicationDeliverKeepsRemovedManagedPathsInScope(t *testing.T) {
	t.Parallel()
	harness := newDeliverHarness(t, deliverAuthorBody)
	defer harness.server.Close()
	source := harness.source

	exit, envelope := harness.execute("publication", "deliver", harness.firstID, "--skill-dir", source)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("first delivery failed: exit=%d envelope=%v", exit, envelope)
	}

	store := channeldelivery.Store{Directory: filepath.Join(harness.root, "config", "channel-deliveries"), EndpointOrigin: harness.server.URL}
	record, exists, err := store.Load(harness.state.productID, source)
	if err != nil || !exists {
		t.Fatalf("record missing after delivery: %v %v", exists, err)
	}
	legacy := filepath.Join(source, ".viceme", "legacy.json")
	if err := os.WriteFile(legacy, []byte("previous generator output"), 0o644); err != nil {
		t.Fatal(err)
	}
	record.AppliedFiles[".viceme/legacy.json"] = fmt.Sprintf("%x", sha256.Sum256([]byte("previous generator output")))
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}

	exit, envelope = harness.execute("publication", "deliver", harness.firstID, "--skill-dir", source)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("delivery with a stale managed file failed: exit=%d envelope=%v", exit, envelope)
	}
	data, _ := envelope["data"].(map[string]any)
	if removed, _ := data["removed"].([]any); len(removed) != 1 || removed[0] != ".viceme/legacy.json" {
		t.Fatalf("stale managed file was not removed: %#v", data["removed"])
	}
	if !containsScopePath(data, ".viceme/legacy.json") {
		t.Fatalf("commit scope lost the removed path on the removing run: %#v", data["commitScope"])
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("stale managed file still exists: %v", err)
	}

	// Simulate the lost response: a second run must still report the deletion
	// in the commit scope even though nothing is removed this time.
	exit, envelope = harness.execute("publication", "deliver", harness.firstID, "--skill-dir", source)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("re-run after the removal failed: exit=%d envelope=%v", exit, envelope)
	}
	data, _ = envelope["data"].(map[string]any)
	if !containsScopePath(data, ".viceme/legacy.json") {
		t.Fatalf("commit scope forgot the pending deletion on re-run: %#v", data["commitScope"])
	}
}

func containsScopePath(data map[string]any, want string) bool {
	scope, _ := data["commitScope"].([]any)
	for _, item := range scope {
		if path, _ := item.(string); path == want {
			return true
		}
	}
	return false
}

// TestPublicationDeliverHonorsViceMeIgnore proves the authored-surface check
// reuses the complete packaging filter: a local file excluded by the author's
// .vicemeignore stays out of the release without blocking the delivery.
func TestPublicationDeliverHonorsViceMeIgnore(t *testing.T) {
	t.Parallel()
	harness := newDeliverHarness(t, deliverAuthorBody)
	defer harness.server.Close()
	source := harness.source

	if err := os.WriteFile(filepath.Join(source, "notes.txt"), []byte("local only, never packaged"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, ".vicemeignore"), []byte("notes.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignored, err := publication.Build(source)
	if err != nil {
		t.Fatal(err)
	}
	harness.state.publications[harness.firstID] = publicationBytes{releaseID: "dddddddd-dddd-4ddd-8ddd-dddddddddddd", pkg: ignored}

	exit, envelope := harness.execute("publication", "deliver", harness.firstID, "--skill-dir", source)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("ignored local file blocked the delivery: exit=%d envelope=%v", exit, envelope)
	}
	if notes, err := os.ReadFile(filepath.Join(source, "notes.txt")); err != nil || string(notes) != "local only, never packaged" {
		t.Fatalf("ignored local file was touched: %q %v", notes, err)
	}
}

// TestPublicationDeliverLocksAcrossCaseAliases proves a case-variant spelling
// of the same directory cannot fork the lock or the record on volumes where
// letter case is not significant. The probe skips case-sensitive filesystems,
// where such aliases are genuinely different directories.
func TestPublicationDeliverLocksAcrossCaseAliases(t *testing.T) {
	t.Parallel()
	harness := newDeliverHarness(t, deliverAuthorBody)
	defer harness.server.Close()
	root := harness.root
	lower := filepath.Join(root, "case-lock")
	if err := os.MkdirAll(filepath.Join(lower, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lower, "SKILL.md"), []byte(deliverAuthorBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lower, "scripts", "run.sh"), []byte("#!/bin/sh\necho demo\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !channeldelivery.CaseInsensitiveVolume(lower) {
		t.Skip("filesystem treats letter case as significant; case aliases are distinct directories")
	}
	upper := filepath.Join(root, "CASE-LOCK")

	store := channeldelivery.Store{Directory: filepath.Join(root, "config", "channel-deliveries"), EndpointOrigin: harness.server.URL}
	unlock, err := store.Lock(lower)
	if err != nil {
		t.Fatal(err)
	}
	exit, envelope := harness.execute("publication", "deliver", harness.firstID, "--skill-dir", upper)
	if exit == 0 || envelope["ok"] != false {
		t.Fatalf("case alias bypassed the directory lock: exit=%d envelope=%v", exit, envelope)
	}
	failure, _ := envelope["error"].(map[string]any)
	if failure["code"] != "SKILL_CHANNEL_DELIVERY_IN_PROGRESS" {
		t.Fatalf("unexpected lock failure code: %#v", failure)
	}
	if _, err := os.Stat(filepath.Join(lower, ".viceme", "runtime.json")); !os.IsNotExist(err) {
		t.Fatalf("case-alias delivery wrote through a held lock: %v", err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}

	exit, envelope = harness.execute("publication", "deliver", harness.firstID, "--skill-dir", upper)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("delivery through the case alias failed: exit=%d envelope=%v", exit, envelope)
	}
	records, err := filepath.Glob(filepath.Join(root, "config", "channel-deliveries", "*", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("case aliases must share one record: %v", records)
	}
}

// TestPublicationDeliverPrunesIgnoredDirectories proves directory-level
// .vicemeignore patterns (bare names and wildcards) exclude whole subtrees
// exactly as the packaging walk does, instead of flagging their files.
func TestPublicationDeliverPrunesIgnoredDirectories(t *testing.T) {
	t.Parallel()
	harness := newDeliverHarness(t, deliverAuthorBody)
	defer harness.server.Close()
	source := harness.source

	for _, dir := range []string{"drafts", "dev-notes"} {
		if err := os.MkdirAll(filepath.Join(source, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, dir, "notes.txt"), []byte("local only"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, ".vicemeignore"), []byte("drafts\ndev-*\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignored, err := publication.Build(source)
	if err != nil {
		t.Fatal(err)
	}
	harness.state.publications[harness.firstID] = publicationBytes{releaseID: "dddddddd-dddd-4ddd-8ddd-dddddddddddd", pkg: ignored}

	exit, envelope := harness.execute("publication", "deliver", harness.firstID, "--skill-dir", source)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("ignored directories blocked the delivery: exit=%d envelope=%v", exit, envelope)
	}
	if notes, err := os.ReadFile(filepath.Join(source, "drafts", "notes.txt")); err != nil || string(notes) != "local only" {
		t.Fatalf("ignored directory content was touched: %q %v", notes, err)
	}
}

// TestPublicationDeliverStopsOnExecutableBitDrift proves an authored file
// with identical bytes but a different executable bit cannot fork the channel
// directory from the delivered ZIP.
func TestPublicationDeliverStopsOnExecutableBitDrift(t *testing.T) {
	t.Parallel()
	harness := newDeliverHarness(t, deliverAuthorBody)
	defer harness.server.Close()
	source := harness.source

	if runtime.GOOS == "windows" {
		t.Skip("POSIX chmod cannot express clearing the executable bit on Windows")
	}
	exit, _ := harness.execute("publication", "deliver", harness.firstID, "--skill-dir", source)
	if exit != 0 {
		t.Fatal("first delivery failed")
	}
	script := filepath.Join(source, "scripts", "run.sh")
	if err := os.Chmod(script, 0o644); err != nil {
		t.Fatal(err)
	}
	exit, envelope := harness.execute("publication", "deliver", harness.firstID, "--skill-dir", source)
	if exit == 0 || envelope["ok"] != false {
		t.Fatalf("executable-bit drift did not stop the delivery: exit=%d envelope=%v", exit, envelope)
	}
	failure, _ := envelope["error"].(map[string]any)
	if failure["code"] != "SKILL_CHANNEL_LOCAL_CONFLICT" {
		t.Fatalf("unexpected code for executable-bit drift: %#v", failure)
	}
	details, _ := failure["details"].(map[string]any)
	conflicts, _ := details["conflicts"].([]any)
	found := false
	for _, item := range conflicts {
		if conflict, _ := item.(map[string]any); conflict != nil && conflict["path"] == "scripts/run.sh" &&
			strings.Contains(conflict["reason"].(string), "executable bit") {
			found = true
		}
	}
	if !found {
		t.Fatalf("conflicts did not name the executable-bit drift: %#v", details["conflicts"])
	}
	if info, err := os.Stat(script); err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("author's mode change was not preserved: %v", err)
	}
}

// TestPublicationDeliverHandlesNonASCIIParentDirectories covers the panic the
// fourth review found: the case probe must index runes, not bytes, so a legal
// path with Chinese components delivers normally.
func TestPublicationDeliverHandlesNonASCIIParentDirectories(t *testing.T) {
	t.Parallel()
	harness := newDeliverHarness(t, deliverAuthorBody)
	defer harness.server.Close()
	source := filepath.Join(harness.root, "中文目录", "skill")
	if err := os.MkdirAll(filepath.Join(source, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(deliverAuthorBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "scripts", "run.sh"), []byte("#!/bin/sh\necho demo\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	exit, envelope := harness.execute("publication", "deliver", harness.firstID, "--skill-dir", source)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("delivery under a non-ASCII parent directory failed: exit=%d envelope=%v", exit, envelope)
	}
	if _, err := os.Stat(filepath.Join(source, ".viceme", "runtime.json")); err != nil {
		t.Fatalf("channel files were not applied: %v", err)
	}
}
