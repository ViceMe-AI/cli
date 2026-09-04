package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	testReplicaPublicationID        = "12121212-1212-4212-8212-121212121212"
	testReplicaPublicationRequestID = "23232323-2323-4232-8232-232323232323"
	testReplicaCreatorID            = "34343434-3434-4343-8343-343434343434"
)

func TestCreateWebsiteReplicaPublicationUsesOptionalAuthentication(t *testing.T) {
	for _, test := range []struct {
		name, token, expectedAuthorization string
	}{
		{name: "anonymous"},
		{name: "authenticated", token: "vme_cli_test", expectedAuthorization: "Bearer vme_cli_test"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodPost || request.URL.Path != "/v1/website-replica-publications" {
					t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
				}
				if authorization := request.Header.Get("Authorization"); authorization != test.expectedAuthorization {
					t.Fatalf("authorization=%q want=%q", authorization, test.expectedAuthorization)
				}
				var input map[string]any
				if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
					t.Fatal(err)
				}
				if confirmation, found := input["confirmation"]; !found || confirmation != nil {
					t.Fatalf("create request did not send an explicit null confirmation: %#v", input)
				}
				writeReplicaPublicationJSON(writer, map[string]any{
					"outcome": "ACTION_REQUIRED", "clientRequestId": testReplicaPublicationRequestID, "market": "CN",
					"nextAction": map[string]any{"kind": "AUTHENTICATE_CREATOR", "authUrl": "https://viceme.cn/login"},
				})
			}))
			defer server.Close()

			var tokens TokenSource
			if test.token != "" {
				tokens = staticToken(test.token)
			}
			client := NewClient(server.URL, server.Client(), tokens, "viceme/test")
			response, err := client.CreateWebsiteReplicaPublication(context.Background(), canonicalReplicaPublicationRequest())
			if err != nil || response.Outcome != "ACTION_REQUIRED" || response.NextAction.Kind != "AUTHENTICATE_CREATOR" {
				t.Fatalf("optional-auth create failed: response=%#v err=%v", response, err)
			}
		})
	}
}

func TestWebsiteReplicaPublicationCreateResponseIsStrictAndSemantic(t *testing.T) {
	canonical := canonicalReplicaPublicationConfirmationResponse()
	if err := callWebsiteReplicaPublicationCreateResponse(t, canonical); err != nil {
		t.Fatalf("canonical confirmation response was rejected: %v", err)
	}

	unknown := cloneReplicaResponse(t, canonical)
	unknown["nextAction"].(map[string]any)["confirmation"].(map[string]any)["review"].(map[string]any)["uploadUrl"] = "https://storage.example/source.zip"
	assertInvalidWebsiteReplicaResponse(t, callWebsiteReplicaPublicationCreateResponse(t, unknown))

	missingNullable := cloneReplicaResponse(t, canonical)
	delete(missingNullable["nextAction"].(map[string]any)["confirmation"].(map[string]any)["review"].(map[string]any), "canonicalOrigin")
	assertInvalidWebsiteReplicaResponse(t, callWebsiteReplicaPublicationCreateResponse(t, missingNullable))

	invalidTTL := cloneReplicaResponse(t, canonical)
	invalidTTL["nextAction"].(map[string]any)["confirmation"].(map[string]any)["expiresAt"] = "2026-09-01T13:04:56Z"
	assertInvalidWebsiteReplicaResponse(t, callWebsiteReplicaPublicationCreateResponse(t, invalidTTL))
}

func TestWebsiteReplicaPublicationAcceptsEveryCanonicalNextAction(t *testing.T) {
	actions := []map[string]any{
		{"kind": "AUTHENTICATE_CREATOR", "authUrl": "https://viceme.cn/login"},
		{"kind": "APPLY_CREATOR", "applicationUrl": "https://viceme.cn/me/creator-center"},
		{"kind": "WAIT_CREATOR_REVIEW", "onboardingId": testReplicaCreatorID, "statusUrl": "https://api.viceme.cn/v1/cli/merchant/onboarding/current"},
		{"kind": "SUPPLY_CREATOR_INFO", "onboardingId": testReplicaCreatorID, "statusUrl": "https://api.viceme.cn/v1/cli/merchant/onboarding/current"},
		{"kind": "CREATOR_APPLICATION_REJECTED", "onboardingId": testReplicaCreatorID, "statusUrl": "https://api.viceme.cn/v1/cli/merchant/onboarding/current"},
		{"kind": "SELECT_MERCHANT", "merchants": []map[string]any{{"id": testMerchantID, "displayName": "Replica Studio", "creatorHandle": "replica-maker"}}},
		{"kind": "CHOOSE_WORK_SLUG", "candidates": []map[string]any{{"slug": "replica-site", "workUrl": "https://viceme.cn/replica-maker/replica-site"}}},
		{"kind": "UPGRADE_CLI", "minimumProtocolVersion": WebsiteReplicaPublicationProtocolVersion, "upgradeUrl": "https://viceme.cn/cli"},
		{"kind": "CHECK_STATUS", "publicationId": testReplicaPublicationID, "statusUrl": "https://viceme.cn/me/website-replica-publications/" + testReplicaPublicationID},
		{"kind": "AUTHORIZE_SOURCE_UPLOAD", "publicationId": testReplicaPublicationID},
		canonicalReplicaPublicationConfirmationResponse()["nextAction"].(map[string]any),
	}
	for _, action := range actions {
		action := action
		t.Run(action["kind"].(string), func(t *testing.T) {
			response := map[string]any{
				"outcome": "ACTION_REQUIRED", "clientRequestId": testReplicaPublicationRequestID,
				"market": "CN", "nextAction": action,
			}
			if err := callWebsiteReplicaPublicationCreateResponse(t, response); err != nil {
				t.Fatalf("canonical %s action was rejected: %v", action["kind"], err)
			}
		})
	}
}

func TestWebsiteReplicaPublicationRejectsActionsThatContradictState(t *testing.T) {
	response := canonicalReplicaPublicationResponse("DRAFT", "WAITING_UPLOAD")
	response["allowedActions"] = []string{"SUBMIT", "CANCEL"}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeReplicaPublicationJSON(writer, response)
	}))
	defer server.Close()
	client := NewClient(server.URL, server.Client(), staticToken("vme_cli_test"), "viceme/test")
	_, err := client.GetWebsiteReplicaPublication(context.Background(), testReplicaPublicationID)
	assertInvalidWebsiteReplicaResponse(t, err)
}

func TestWebsiteReplicaPublicationRejectsRetryCountAboveContractLimit(t *testing.T) {
	response := canonicalReplicaPublicationResponse("DRAFT", "WAITING_UPLOAD")
	response["retry"].(map[string]any)["automaticRetries"] = 4
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeReplicaPublicationJSON(writer, response)
	}))
	defer server.Close()
	client := NewClient(server.URL, server.Client(), staticToken("vme_cli_test"), "viceme/test")
	_, err := client.GetWebsiteReplicaPublication(context.Background(), testReplicaPublicationID)
	assertInvalidWebsiteReplicaResponse(t, err)
}

func TestWebsiteReplicaPublicationAcceptsZodDatetimeWithoutSeconds(t *testing.T) {
	response := canonicalReplicaPublicationConfirmationResponse()
	confirmation := response["nextAction"].(map[string]any)["confirmation"].(map[string]any)
	confirmation["issuedAt"] = "2026-09-01T12:34Z"
	confirmation["expiresAt"] = "2026-09-01T13:04Z"
	if err := callWebsiteReplicaPublicationCreateResponse(t, response); err != nil {
		t.Fatalf("valid z.iso.datetime values without seconds were rejected: %v", err)
	}
}

func TestWebsiteReplicaPublicationAcceptsEveryCanonicalLifecycleState(t *testing.T) {
	for _, status := range []string{"DRAFT", "PROCESSING", "PUBLISHED", "PUBLISHED_DEGRADED", "FAILED", "CANCELLED"} {
		status := status
		t.Run(status, func(t *testing.T) {
			response := canonicalReplicaPublicationLifecycleResponse(status)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writeReplicaPublicationJSON(writer, response)
			}))
			defer server.Close()
			client := NewClient(server.URL, server.Client(), staticToken("vme_cli_test"), "viceme/test")
			publication, err := client.GetWebsiteReplicaPublication(context.Background(), testReplicaPublicationID)
			if err != nil || publication.Status != status {
				t.Fatalf("canonical %s lifecycle state was rejected: publication=%#v err=%v", status, publication, err)
			}
		})
	}
}

func TestWebsiteReplicaPublicationReadyRejectsTerminalProductMismatch(t *testing.T) {
	publication := canonicalReplicaPublicationLifecycleResponse("PUBLISHED")
	response := map[string]any{
		"outcome": "PUBLICATION_READY",
		"target": map[string]any{
			"resolution": "UPDATE", "merchantAccountId": testMerchantID,
			"workId": testWorkID, "replicaId": testReplicaID, "productId": testReplicaCreatorID,
			"workUrl": "https://viceme.cn/replica-maker/replica-site",
		},
		"publication": publication,
		"nextAction": map[string]any{
			"kind": "CHECK_STATUS", "publicationId": testReplicaPublicationID,
			"statusUrl": "https://viceme.cn/me/website-replica-publications/" + testReplicaPublicationID,
		},
	}
	assertInvalidWebsiteReplicaResponse(t, callWebsiteReplicaPublicationCreateResponse(t, response))
}

func canonicalReplicaPublicationRequest() CreateWebsiteReplicaPublicationRequest {
	return CreateWebsiteReplicaPublicationRequest{
		ProtocolVersion: WebsiteReplicaPublicationProtocolVersion,
		ClientRequestID: testReplicaPublicationRequestID, Market: "CN",
		ProjectFingerprint: strings.Repeat("a", 64),
		Target:             WebsiteReplicaPublicationTarget{Kind: "NEW_WORK", Slug: "replica-site"},
		Title:              "Replica", Summary: "Summary", PriceCents: 990,
		Source: WebsiteReplicaPublicationSourceArtifact{
			FileName: "source.zip", ContentType: "application/zip", SizeBytes: 1024, Digest: strings.Repeat("b", 64),
		},
	}
}

func canonicalReplicaPublicationConfirmationResponse() map[string]any {
	request := canonicalReplicaPublicationRequest()
	issuedAt := time.Date(2026, 9, 1, 12, 34, 57, 0, time.UTC)
	return map[string]any{
		"outcome": "ACTION_REQUIRED", "clientRequestId": request.ClientRequestID, "market": request.Market,
		"nextAction": map[string]any{
			"kind": "CONFIRM_PUBLICATION",
			"confirmation": map[string]any{
				"version": "wrv1-" + strings.Repeat("c", 64),
				"review": map[string]any{
					"resolution": "CREATE", "merchantAccountId": testMerchantID,
					"merchantDisplayName": "Replica Studio", "creatorAccountId": testReplicaCreatorID,
					"creatorHandle": "replica-maker", "creatorDisplayName": "Replica Maker",
					"projectFingerprint": request.ProjectFingerprint, "workUrl": "https://viceme.cn/replica-maker/replica-site",
					"canonicalOrigin": nil, "title": request.Title, "summary": request.Summary,
					"priceCents": request.PriceCents, "source": request.Source,
				},
				"issuedAt": issuedAt.Format(time.RFC3339), "expiresAt": issuedAt.Add(30 * time.Minute).Format(time.RFC3339),
			},
		},
	}
}

func canonicalReplicaPublicationResponse(status, sourceStatus string) map[string]any {
	now := "2026-09-01T12:34:57Z"
	verifiedAt := any(nil)
	if sourceStatus == "VERIFIED" || sourceStatus == "ACTIVATED" {
		verifiedAt = now
	}
	actions := []string{}
	if status == "DRAFT" && sourceStatus == "WAITING_UPLOAD" {
		actions = []string{"AUTHORIZE_SOURCE_UPLOAD", "COMPLETE_SOURCE_UPLOAD", "CANCEL"}
	}
	return map[string]any{
		"id": testReplicaPublicationID, "clientRequestId": testReplicaPublicationRequestID,
		"market": "CN", "merchantAccountId": testMerchantID, "workId": testWorkID, "replicaId": testReplicaID,
		"status": status, "statusUrl": "https://viceme.cn/me/website-replica-publications/" + testReplicaPublicationID,
		"allowedActions": actions,
		"retry":          map[string]any{"automaticRetries": 0, "maxAutomaticRetries": 3, "nextAttemptAt": nil},
		"source": map[string]any{
			"fileName": "source.zip", "contentType": "application/zip", "sizeBytes": 1024,
			"digest": strings.Repeat("b", 64), "status": sourceStatus, "verifiedAt": verifiedAt,
		},
		"failure": nil, "result": nil, "submittedAt": nil, "failedAt": nil, "cancelledAt": nil,
		"createdAt": now, "updatedAt": now,
	}
}

func canonicalReplicaPublicationLifecycleResponse(status string) map[string]any {
	sourceStatus := "WAITING_UPLOAD"
	if status == "PROCESSING" || status == "FAILED" {
		sourceStatus = "VERIFIED"
	}
	if status == "PUBLISHED" || status == "PUBLISHED_DEGRADED" {
		sourceStatus = "ACTIVATED"
	}
	response := canonicalReplicaPublicationResponse(status, sourceStatus)
	now := response["createdAt"].(string)
	switch status {
	case "PROCESSING":
		response["allowedActions"] = []string{"CANCEL"}
		response["submittedAt"] = now
	case "PUBLISHED", "PUBLISHED_DEGRADED":
		response["submittedAt"] = now
		response["result"] = map[string]any{
			"workUrl":   "https://viceme.cn/replica-maker/replica-site",
			"versionId": testVersionID, "version": 1,
			"shortCode":   "VMR-ABCDEFGHIJKLMNOPQRST",
			"instruction": "VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST",
			"product": map[string]any{
				"id": testReplicaProductID, "skuId": testSKUID, "title": "Replica", "currency": "CNY", "priceCents": 990,
			},
			"publishedAt": now,
		}
		if status == "PUBLISHED_DEGRADED" {
			response["failure"] = map[string]any{"code": "HOSTING_FAILED", "message": "Hosting failed", "retryable": false}
		}
	case "FAILED":
		response["allowedActions"] = []string{"RETRY"}
		response["submittedAt"] = now
		response["failedAt"] = now
		response["failure"] = map[string]any{"code": "PROCESSING_FAILED", "message": "Processing failed", "retryable": true}
	case "CANCELLED":
		response["cancelledAt"] = now
	}
	return response
}

func callWebsiteReplicaPublicationCreateResponse(t *testing.T, response map[string]any) error {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeReplicaPublicationJSON(writer, response)
	}))
	defer server.Close()
	client := NewClient(server.URL, server.Client(), staticToken("vme_cli_test"), "viceme/test")
	_, err := client.CreateWebsiteReplicaPublication(context.Background(), canonicalReplicaPublicationRequest())
	return err
}

func writeReplicaPublicationJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}
