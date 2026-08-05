package appmanifest

import (
	"os"
	"strings"
	"testing"
)

func TestLinkIntentReusesStableRequestIDUntilRemoved(t *testing.T) {
	directory := t.TempDir()
	ids := []string{
		"550e8400-e29b-41d4-a716-446655440010",
		"550e8400-e29b-41d4-a716-446655440011",
	}
	index := 0
	newID := func() string {
		id := ids[index]
		index++
		return id
	}
	first, err := LoadOrCreateLinkIntent(directory, "Poster Lab", "EXTERNAL", newID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreateLinkIntent(directory, "Poster Lab", "EXTERNAL", newID)
	if err != nil {
		t.Fatal(err)
	}
	if first.ClientRequestID != second.ClientRequestID || index != 1 {
		t.Fatalf("request ID was not reused: first=%#v second=%#v calls=%d", first, second, index)
	}
	if err := RemoveLinkIntent(directory); err != nil {
		t.Fatal(err)
	}
	third, err := LoadOrCreateLinkIntent(directory, "Poster Lab", "EXTERNAL", newID)
	if err != nil {
		t.Fatal(err)
	}
	if third.ClientRequestID == first.ClientRequestID || index != 2 {
		t.Fatalf("removed intent was unexpectedly reused: %#v", third)
	}
}

func TestLinkIntentRejectsChangedInputAndUnknownFields(t *testing.T) {
	directory := t.TempDir()
	newID := func() string { return "550e8400-e29b-41d4-a716-446655440010" }
	if _, err := LoadOrCreateLinkIntent(directory, "Poster Lab", "EXTERNAL", newID); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateLinkIntent(directory, "Different", "EXTERNAL", newID); err == nil {
		t.Fatal("changed App creation input was accepted")
	}
	data := []byte(`{"schemaVersion":1,"clientRequestId":"550e8400-e29b-41d4-a716-446655440010","name":"Poster Lab","hostingMode":"EXTERNAL","secret":"no"}`)
	if err := os.WriteFile(LinkIntentPath(directory), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateLinkIntent(directory, "Poster Lab", "EXTERNAL", newID); err == nil {
		t.Fatal("unknown pending intent field was accepted")
	}
}

func TestLinkIntentMeasuresNameLengthInCharacters(t *testing.T) {
	directory := t.TempDir()
	newID := func() string { return "550e8400-e29b-41d4-a716-446655440010" }
	if _, err := LoadOrCreateLinkIntent(directory, strings.Repeat("图", 100), "EXTERNAL", newID); err != nil {
		t.Fatalf("100-character name was rejected: %v", err)
	}

	otherDirectory := t.TempDir()
	if _, err := LoadOrCreateLinkIntent(otherDirectory, strings.Repeat("图", 101), "EXTERNAL", newID); err == nil {
		t.Fatal("101-character name was accepted")
	}
}
