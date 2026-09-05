package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrialUseRequiresExplicitAuthoritativeOutcome(t *testing.T) {
	for _, test := range []struct {
		body string
		ok   bool
	}{
		{`{"allowed":true,"remainingUses":0,"limitUses":2,"reason":null,"purchaseUrl":null}`, true},
		{`{"allowed":false,"reason":"EXHAUSTED","purchaseUrl":"https://shop.example.test/buy"}`, true},
		{`{"reason":"EXHAUSTED","purchaseUrl":"https://shop.example.test/buy"}`, false},
		{`{"allowed":null,"reason":"EXHAUSTED","purchaseUrl":"https://shop.example.test/buy"}`, false},
		{`{"allowed":false,"reason":"UNKNOWN","purchaseUrl":"https://shop.example.test/buy"}`, false},
		{`{"allowed":false,"reason":"EXHAUSTED","purchaseUrl":null}`, false},
		{`{"allowed":false,"reason":"EXHAUSTED","purchaseUrl":"javascript:alert(1)"}`, false},
		{`{"allowed":true}`, false},
	} {
		t.Run(test.body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			client := NewClient(server.URL, server.Client(), nil, "viceme/test")
			_, err := client.ConsumeSkillTrialUse(context.Background(), "product", "install", "test-secret", "request-id")
			if (err == nil) != test.ok {
				t.Fatalf("unexpected validation result: %v", err)
			}
		})
	}
}
