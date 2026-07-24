package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestPrinterBusinessWritesIndentedBareResult(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	printer := &Printer{Out: &stdout, ErrOut: &bytes.Buffer{}}

	if err := printer.Business(map[string]any{"authenticated": true, "profile": "local"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout.String(), `"ok"`) || strings.Contains(stdout.String(), `"data"`) || strings.Contains(stdout.String(), `"meta"`) {
		t.Fatalf("business output contains transport fields: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "\n  \"authenticated\": true,") {
		t.Fatalf("business output is not indented: %q", stdout.String())
	}
}

func TestPrinterSuccessKeepsProtocolEnvelopeWithoutDefaultMeta(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	printer := &Printer{Out: &stdout, ErrOut: &bytes.Buffer{}}

	if err := printer.Success(map[string]any{"status": "compiling"}); err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["ok"] != true || envelope["data"] == nil {
		t.Fatalf("unexpected protocol envelope: %#v", envelope)
	}
	if _, exists := envelope["meta"]; exists {
		t.Fatalf("protocol envelope contains irrelevant meta: %#v", envelope)
	}
	if !strings.Contains(stdout.String(), "\n  \"ok\": true,") {
		t.Fatalf("protocol output is not indented: %q", stdout.String())
	}
}

func TestPrinterSuccessWithMetaEmitsOnlyMeaningfulMeta(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		meta     Meta
		wantMeta bool
	}{
		{name: "empty", meta: Meta{}},
		{name: "wait timeout", meta: Meta{WaitTimedOut: boolPointer(true)}, wantMeta: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			printer := &Printer{Out: &stdout, ErrOut: &bytes.Buffer{}}
			if err := printer.SuccessWithMeta(map[string]any{"status": "compiling"}, test.meta); err != nil {
				t.Fatal(err)
			}
			var envelope map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			_, exists := envelope["meta"]
			if exists != test.wantMeta {
				t.Fatalf("meta exists=%v, want %v: %#v", exists, test.wantMeta, envelope)
			}
		})
	}
}

func TestPrinterFailureWritesIndentedTypedEnvelopeWithoutMeta(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	printer := &Printer{Out: &bytes.Buffer{}, ErrOut: &stderr}

	if code := printer.Failure(Validation("bad_input", "bad input")); code != ExitValidation {
		t.Fatalf("exit code=%d", code)
	}
	var envelope map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["ok"] != false || envelope["error"] == nil {
		t.Fatalf("unexpected error envelope: %#v", envelope)
	}
	if _, exists := envelope["meta"]; exists {
		t.Fatalf("error envelope contains irrelevant meta: %#v", envelope)
	}
	if !strings.Contains(stderr.String(), "\n  \"ok\": false,") {
		t.Fatalf("error output is not indented: %q", stderr.String())
	}
}

func TestPrinterRawPreservesBytes(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	printer := &Printer{Out: &stdout, ErrOut: &bytes.Buffer{}}
	content := []byte("# Skill\n\nExact content.\n")

	if err := printer.Raw(content); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stdout.Bytes(), content) {
		t.Fatalf("raw output=%q, want %q", stdout.Bytes(), content)
	}
}

func boolPointer(value bool) *bool {
	return &value
}
