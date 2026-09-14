package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestReplicaInvitationUsesPublicStartAndBoundSession(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["flowId"] != "10000000-0000-4000-8000-000000000001" {
			t.Error("wrong flow")
		}
		switch calls {
		case 1:
			if r.URL.Path != "/v1/website-replica-invitation-flows" || r.Header.Get("Authorization") != "" || len(body) != 4 {
				t.Error("start leaked data or credentials")
			}
		case 2:
			if r.URL.Path != "/v1/website-replica-sessions/session/orders/order/invitation-flow" || r.Header.Get("Authorization") != "Bearer example-session" || len(body) != 1 {
				t.Error("association changed session")
			}
		}
		_, _ = w.Write([]byte(`{"recorded":true}`))
	}))
	defer server.Close()
	client := NewClient(server.URL, server.Client(), nil, "test")
	id := "10000000-0000-4000-8000-000000000001"
	if !client.RecordReplicaInvitation(context.Background(), id, "VMR-ABCDEFGHIJKLMNOPQRST", "test") || !client.AssociateReplicaInvitation(context.Background(), id, "order", "session", "example-session") {
		t.Fatal("event not recorded")
	}
}

func TestReplicaInvitationTimeoutIsBoundedAndOldServerIsOptional(t *testing.T) {
	for _, status := range []int{404, 503} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		client := NewClient(server.URL, server.Client(), nil, "test")
		if client.ReportReplicaInvitation(context.Background(), "flow", "INSTALL_COMPLETED") {
			t.Error("failed event reported success")
		}
		server.Close()
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer server.Close()
	client := NewClient(server.URL, server.Client(), nil, "test")
	start := time.Now()
	if client.ReportReplicaInvitation(context.Background(), "flow", "INSTALL_COMPLETED") {
		t.Fatal("timed out event reported success")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("analytics exceeded timeout budget")
	}
}
