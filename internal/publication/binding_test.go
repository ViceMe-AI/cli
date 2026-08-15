package publication

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWorkspaceBindingSurvivesContentChangesAndMoves(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "skill")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "SKILL.md"), []byte(testSkillMarkdown), 0o644); err != nil {
		t.Fatal(err)
	}
	store := testBindingStore(t)
	ids := idSequence()
	first, err := store.ResolveOrCreate(workspace, "WORKSPACE", digest("a"), "", ids)
	if err != nil {
		t.Fatal(err)
	}
	binding := testBinding(first.ClientWorkID, digest("a"), store)
	if err := store.Save(workspace, "WORKSPACE", binding); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "notes.md"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := store.ResolveOrCreate(workspace, "WORKSPACE", digest("b"), "", ids)
	if err != nil {
		t.Fatal(err)
	}
	if changed.ClientWorkID != first.ClientWorkID || changed.Binding == nil || changed.Binding.ListingID != binding.ListingID {
		t.Fatalf("workspace content change lost stable identity: %#v", changed)
	}
	moved := filepath.Join(root, "moved-skill")
	if err := os.Rename(workspace, moved); err != nil {
		t.Fatal(err)
	}
	resolved, err := store.ResolveOrCreate(moved, "WORKSPACE", digest("b"), "", ids)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ClientWorkID != first.ClientWorkID {
		t.Fatalf("moved workspace lost stable identity: %#v", resolved)
	}
}

func TestZIPSidecarAndFallbackRecoverRenameAndMissingSidecar(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "skill.zip")
	if err := os.WriteFile(zipPath, []byte("zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := testBindingStore(t)
	ids := idSequence()
	first, err := store.ResolveOrCreate(zipPath, "ZIP", digest("c"), "", ids)
	if err != nil {
		t.Fatal(err)
	}
	binding := testBinding(first.ClientWorkID, digest("c"), store)
	if err := store.Save(zipPath, "ZIP", binding); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(zipPath + ".viceme.json"); err != nil {
		t.Fatalf("ZIP sidecar was not adjacent: %v", err)
	}
	renamed := filepath.Join(root, "renamed.zip")
	if err := os.Rename(zipPath, renamed); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(zipPath+".viceme.json", renamed+".viceme.json"); err != nil {
		t.Fatal(err)
	}
	resolved, err := store.ResolveOrCreate(renamed, "ZIP", digest("d"), "", ids)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ClientWorkID != first.ClientWorkID {
		t.Fatalf("renamed ZIP and sidecar lost stable identity: %#v", resolved)
	}
	if err := os.Remove(renamed + ".viceme.json"); err != nil {
		t.Fatal(err)
	}
	fallback, err := store.ResolveOrCreate(renamed, "ZIP", digest("c"), "", ids)
	if err != nil {
		t.Fatal(err)
	}
	if fallback.ClientWorkID != first.ClientWorkID {
		t.Fatalf("endpoint-scoped digest fallback lost identity: %#v", fallback)
	}
}

func TestBindingIntentConvergesConcurrentPrepareAndExplicitNewSeparates(t *testing.T) {
	store := testBindingStore(t)
	path := filepath.Join(t.TempDir(), "skill.zip")
	var mu sync.Mutex
	next := 0
	newID := func() string {
		mu.Lock()
		defer mu.Unlock()
		next++
		return fmt.Sprintf("00000000-0000-4000-8000-%012d", next)
	}
	results := make([]ResolvedSourceIdentity, 8)
	errors := make([]error, 8)
	var wait sync.WaitGroup
	for index := range results {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			results[index], errors[index] = store.ResolveOrCreate(path, "ZIP", digest("e"), "", newID)
		}(index)
	}
	wait.Wait()
	for _, err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, result := range results[1:] {
		if result.ClientWorkID != results[0].ClientWorkID || result.ClientRequestID != results[0].ClientRequestID {
			t.Fatalf("concurrent prepare identities diverged: %#v", results)
		}
	}
	separate, err := store.ResolveOrCreate(path, "ZIP", digest("e"), "CREATE_NEW", newID)
	if err != nil {
		t.Fatal(err)
	}
	if separate.ClientWorkID == results[0].ClientWorkID {
		t.Fatal("explicit new Listing reused the old clientWorkId")
	}
	retry, err := store.ResolveOrCreate(path, "ZIP", digest("e"), "CREATE_NEW", newID)
	if err != nil {
		t.Fatal(err)
	}
	if retry.ClientWorkID != separate.ClientWorkID || retry.ClientRequestID != separate.ClientRequestID {
		t.Fatalf("explicit new retry did not preserve the in-flight identity: first=%#v retry=%#v", separate, retry)
	}
	bound, err := store.ResolveOrCreate(path, "ZIP", digest("e"), "BIND_EXISTING:22222222-2222-4222-8222-222222222222", newID)
	if err != nil {
		t.Fatal(err)
	}
	if bound.ClientRequestID == separate.ClientRequestID {
		t.Fatal("explicit bind reused an incompatible explicit-new idempotency key")
	}
	boundRetry, err := store.ResolveOrCreate(path, "ZIP", digest("e"), "BIND_EXISTING:22222222-2222-4222-8222-222222222222", newID)
	if err != nil {
		t.Fatal(err)
	}
	if boundRetry.ClientRequestID != bound.ClientRequestID {
		t.Fatal("explicit bind response-loss retry changed its idempotency key")
	}
}

func TestBindingRejectsAnotherEndpointScope(t *testing.T) {
	store := testBindingStore(t)
	path := filepath.Join(t.TempDir(), "skill.zip")
	binding := testBinding("11111111-1111-4111-8111-111111111111", digest("f"), store)
	if err := store.Save(path, "ZIP", binding); err != nil {
		t.Fatal(err)
	}
	other := store
	other.EndpointOrigin = "https://api.viceme.ai"
	_, err := other.ResolveOrCreate(path, "ZIP", digest("f"), "", idSequence())
	if err == nil {
		t.Fatal("binding from another endpoint was accepted")
	}
}

func testBindingStore(t *testing.T) BindingStore {
	t.Helper()
	return BindingStore{Directory: filepath.Join(t.TempDir(), "bindings"), EndpointOrigin: "https://api.viceme.cn", Market: "CN"}
}

func testBinding(clientWorkID, packageDigest string, store BindingStore) SkillBinding {
	return SkillBinding{
		APIVersion: BindingAPIVersion, Kind: "SkillListing",
		ListingID: "22222222-2222-4222-8222-222222222222", ClientWorkID: clientWorkID,
		Market: store.Market, EndpointOrigin: store.EndpointOrigin,
		BindingReceipt: "signed-receipt", LastPackageDigest: packageDigest,
	}
}

func digest(character string) string {
	return string([]byte(character)[0]) + repeat(character, 63)
}

func repeat(value string, count int) string {
	result := ""
	for range count {
		result += value
	}
	return result
}

func idSequence() func() string {
	next := 0
	return func() string {
		next++
		return fmt.Sprintf("00000000-0000-4000-8000-%012d", next)
	}
}
