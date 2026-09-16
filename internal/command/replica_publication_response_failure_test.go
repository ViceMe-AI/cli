package command

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/output"
)

func TestReplicaPublicationInvalidResponseStopsHostRecovery(t *testing.T) {
	for _, operation := range []string{"publish", "status", "resume", "cancel"} {
		t.Run(operation, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				writeJSONResponse(w, map[string]any{"unexpected": "private-provider-diagnostic"})
			}))
			defer server.Close()
			t.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken)
			deps := replicaPublicationTestDependencies(t, t.TempDir(), server, time.Date(2026, 9, 16, 2, 0, 0, 0, time.UTC))
			var stdout bytes.Buffer
			deps.Out = &stdout
			args := []string{"replica", operation, replicaPublicationTestID}
			if operation == "publish" {
				args = replicaPublicationTestArguments(newReplicaPublicationTestProject(t), "replica-site")
			}
			exit := Execute(args, deps)
			var envelope struct {
				OK    bool `json:"ok"`
				Error struct {
					Code      string         `json:"code"`
					Retryable bool           `json:"retryable"`
					Details   map[string]any `json:"details"`
				} `json:"error"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if exit != output.ExitInternal || envelope.OK || envelope.Error.Code != "RESPONSE_INVALID" || envelope.Error.Retryable || envelope.Error.Details["nextAction"] != "STOP_AND_REPORT" {
				t.Fatalf("missing stop boundary: exit=%d output=%s", exit, stdout.String())
			}
			if calls.Load() != 1 {
				t.Fatalf("invalid response triggered additional requests: %d", calls.Load())
			}
			if strings.Contains(stdout.String(), "private-provider-diagnostic") {
				t.Fatal("raw response leaked")
			}
		})
	}
}

func TestReplicaPublicationResponseFailurePreservesOtherRecoveryActions(t *testing.T) {
	original := output.Network("API_UNREACHABLE", "unreachable", nil)
	if replicaPublicationResponseFailure(original) != original {
		t.Fatal("unrelated network recovery was changed")
	}
	invalid := output.Internal("RESPONSE_INVALID", "invalid", nil).WithDetails(map[string]any{"publicationId": replicaPublicationTestID})
	result := output.AsError(replicaPublicationResponseFailure(invalid))
	if result.Details.(map[string]any)["publicationId"] != replicaPublicationTestID {
		t.Fatal("lost known publication identity")
	}
	if _, modified := invalid.Details.(map[string]any)["nextAction"]; modified {
		t.Fatal("modified the original failure")
	}
}
