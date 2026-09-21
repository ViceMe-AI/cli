package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGuidanceHTTPEncodingKeepsUTF8AndHTMLAndBoundsCredentialEnvelope(t *testing.T) {
	calls := 0
	prompt := strings.Repeat("中文<>&", 600_000)
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		raw, _ := io.ReadAll(request.Body)
		if len(raw) > SkillGuidanceMaxRequestBytes || strings.Contains(string(raw), `\u003c`) {
			t.Fatal("guidance HTTP encoding expanded the task")
		}
		var input SkillGuidanceSubmit
		if json.Unmarshal(raw, &input) != nil || input.Prompt != prompt {
			t.Fatal("task changed during HTTP encoding")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"requestId":"11111111-1111-4111-8111-111111111111","releaseId":"22222222-2222-4222-8222-222222222222","version":1,"status":"QUEUED","instructions":null,"message":null,"outcome":null,"errorCode":null,"retryable":false,"expiresAt":"2099-01-01T00:00:00Z"}`))}, nil
	})
	client := NewClient("http://127.0.0.1:1234", &http.Client{Transport: transport}, nil, "test")
	input := SkillGuidanceSubmit{Prompt: prompt, Facts: map[string]string{}}
	if _, err := client.SubmitSkillGuidance(context.Background(), input, "test-install", "test-secret"); err != nil || calls != 1 {
		t.Fatalf("large guidance request: %v calls=%d", err, calls)
	}
	input.Prompt = strings.Repeat("a", SkillGuidanceMaxRequestBytes-10)
	if _, err := client.SubmitSkillGuidance(context.Background(), input, "test-install", "test-secret"); err == nil || calls != 1 {
		t.Fatal("credential envelope exceeded cap but reached network")
	}
}
