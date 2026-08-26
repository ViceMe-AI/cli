package command

import (
	"encoding/json"
	"testing"
)

func TestBuildInteractionInputGuideUsesScenarioSchema(t *testing.T) {
	guide, err := buildInteractionInputGuide(json.RawMessage(`{
		"type":"object",
		"required":["resume"],
		"properties":{
			"years":{"type":"number","title":"工作年限"},
			"resume":{"type":"string","format":"uri","title":"简历","description":"上传简历"}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(guide) != 2 || guide[0].Key != "resume" || !guide[0].Required || guide[0].Label != "简历" || guide[0].Format != "uri" || guide[1].Key != "years" {
		t.Fatalf("unexpected guide: %#v", guide)
	}
}

func TestNormalizeInlineJSONObjectRejectsNonObject(t *testing.T) {
	if _, err := normalizeInlineJSONObject(`["resume"]`); err == nil {
		t.Fatal("expected an object validation error")
	}
	if value, err := normalizeInlineJSONObject(`{"resume":"asset"}`); err != nil || string(value) != `{"resume":"asset"}` {
		t.Fatalf("unexpected normalized input: %s, %v", value, err)
	}
}

func TestInteractionInstanceURLUsesReturnedProjection(t *testing.T) {
	projection := json.RawMessage(`{"instance":{"instanceNo":"INT202608260001","work":{"creatorHandle":"alice","slug":"recruitment"}}}`)
	if url := interactionInstanceURL(projection); url != "/interaction/alice/recruitment/INT202608260001" {
		t.Fatalf("unexpected instance URL: %s", url)
	}
}

func TestInteractionRuntimeVersionIsIndependentFromCommerceProtocol(t *testing.T) {
	if err := validateInteractionRuntimeVersion("1.5.0"); err != nil {
		t.Fatalf("current Interaction Runtime was rejected: %v", err)
	}
	if err := validateInteractionRuntimeVersion("1.5.1"); err == nil {
		t.Fatal("future Interaction Runtime requirement was accepted")
	}
}
