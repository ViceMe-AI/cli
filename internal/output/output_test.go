package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestPrinterUsesOneStableEnvelopeAndKeepsStdoutClean(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	printer := Printer{Out: &stdout, ErrOut: &stderr, ExecutingCLIVersion: "1.2.3"}
	if err := printer.Success(map[string]string{"status": "ready"}); err != nil {
		t.Fatal(err)
	}
	var success map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &success); err != nil {
		t.Fatal(err)
	}
	if success["ok"] != true || stderr.Len() != 0 {
		t.Fatalf("unexpected success envelope: %s stderr=%s", stdout.String(), stderr.String())
	}
	meta := success["meta"].(map[string]any)
	if meta["executingCliVersion"] != "1.2.3" {
		t.Fatalf("missing executing CLI version: %s", stdout.String())
	}
	if _, legacy := meta["cliVersion"]; legacy {
		t.Fatalf("ambiguous legacy CLI version leaked: %s", stdout.String())
	}

	stdout.Reset()
	exit := printer.Failure(Network("API_UNREACHABLE", "offline", errors.New("dial failed")))
	if exit != ExitNetwork || stderr.Len() != 0 {
		t.Fatalf("failure polluted stderr or used wrong exit: exit=%d stderr=%q", exit, stderr.String())
	}
	var failure map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	errorObject := failure["error"].(map[string]any)
	if failure["ok"] != false || errorObject["code"] != "API_UNREACHABLE" || errorObject["retryable"] != true {
		t.Fatalf("unexpected failure envelope: %s", stdout.String())
	}
	if _, leaked := errorObject["Cause"]; leaked || bytes.Contains(stdout.Bytes(), []byte("dial failed")) {
		t.Fatalf("internal cause leaked: %s", stdout.String())
	}
}

func TestPrinterReportsTheGenerationThatAutomaticallyUpdatedBeforeExecution(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	printer := Printer{
		Out:                 &stdout,
		ExecutingCLIVersion: "0.16.0",
		AutoUpdate: &AutoUpdateMeta{
			From: "0.15.2", To: "0.16.0", Status: "updated",
		},
	}
	if err := printer.Success(map[string]string{"status": "ready"}); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	meta := result["meta"].(map[string]any)
	autoUpdate := meta["autoUpdate"].(map[string]any)
	if meta["executingCliVersion"] != "0.16.0" || autoUpdate["from"] != "0.15.2" ||
		autoUpdate["to"] != "0.16.0" || autoUpdate["status"] != "updated" {
		t.Fatalf("automatic generation metadata=%#v", meta)
	}
}
