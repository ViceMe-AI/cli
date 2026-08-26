package command

import (
	"encoding/json"
	"testing"
)

func TestBuildInteractionInputGuideUsesStructuredSchema(t *testing.T) {
	guide, err := buildInteractionInputGuide(json.RawMessage(`{
		"type":"object",
		"required":["request"],
		"properties":{
			"quantity":{"type":"number","title":"Quantity"},
			"request":{"type":"string","format":"uri","title":"Request","description":"Provide input"}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(guide) != 2 || guide[0].Key != "quantity" || guide[0].Required || guide[1].Key != "request" || !guide[1].Required || guide[1].Label != "Request" || guide[1].Format != "uri" {
		t.Fatalf("unexpected guide: %#v", guide)
	}
}

func TestNormalizeInlineJSONObjectRejectsNonObject(t *testing.T) {
	if _, err := normalizeInlineJSONObject(`["request"]`); err == nil {
		t.Fatal("expected an object validation error")
	}
	if value, err := normalizeInlineJSONObject(`{"request":"value"}`); err != nil || string(value) != `{"request":"value"}` {
		t.Fatalf("unexpected normalized input: %s, %v", value, err)
	}
}

func TestInteractionInstanceURLUsesReturnedProjection(t *testing.T) {
	projection := json.RawMessage(`{"instance":{"instanceNo":"INT202608260001","work":{"creatorHandle":"alice","slug":"structured-service"}}}`)
	if url := interactionInstanceURL(projection); url != "/interaction/alice/structured-service/INT202608260001" {
		t.Fatalf("unexpected instance URL: %s", url)
	}
}

func TestInteractionRuntimeVersionIsIndependentFromCommerceProtocol(t *testing.T) {
	if err := validateInteractionRuntimeVersion("1.6.0"); err != nil {
		t.Fatalf("current Interaction Runtime was rejected: %v", err)
	}
	if err := validateInteractionRuntimeVersion("1.6.1"); err == nil {
		t.Fatal("future Interaction Runtime requirement was accepted")
	}
}

func TestInteractionProjectionNextActionUsesRuntimeProjection(t *testing.T) {
	tests := []struct {
		projection string
		want       string
	}{
		{`{"instance":{"lifecycleStatus":"OPEN"},"tasks":[{"type":"OPEN"}],"allowedActions":[]}`, "COMPLETE_TASK"},
		{`{"instance":{"lifecycleStatus":"OPEN"},"tasks":[],"allowedActions":[{"code":"CONTINUE"}]}`, "EXECUTE_ACTION"},
		{`{"instance":{"lifecycleStatus":"OPEN"},"tasks":[],"allowedActions":[]}`, "WAIT_FOR_ROLE"},
		{`{"instance":{"lifecycleStatus":"COMPLETED"},"tasks":[],"allowedActions":[]}`, "INTERACTION_CLOSED"},
	}
	for _, test := range tests {
		if got := interactionProjectionNextAction(json.RawMessage(test.projection)); got != test.want {
			t.Fatalf("unexpected next action %q for %s; want %q", got, test.projection, test.want)
		}
	}
}
