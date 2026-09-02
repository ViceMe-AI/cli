package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/output"
)

const (
	testReplicaID        = "11111111-1111-4111-8111-111111111111"
	testUploadID         = "22222222-2222-4222-8222-222222222222"
	testVersionID        = "33333333-3333-4333-8333-333333333333"
	testReplicaProductID = "44444444-4444-4444-8444-444444444444"
	testSKUID            = "55555555-5555-4555-8555-555555555555"
	testQuoteID          = "66666666-6666-4666-8666-666666666666"
	testWorkID           = "77777777-7777-4777-8777-777777777777"
	testEntitlementID    = "88888888-8888-4888-8888-888888888888"
	testFulfillmentID    = "99999999-9999-4999-8999-999999999999"
	testTaskID           = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	testCaseID           = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	testMerchantID       = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	testShortCode        = "VMR-ABCDEFGHIJKLMNOPQRST"
	testOrderNo          = "VMO-20260901-000001"
	testReplicaTimestamp = "2026-09-01T12:34:56.123Z"
)

type websiteReplicaResponseCase struct {
	name     string
	response map[string]any
	call     func(*Client) error
}

func TestWebsiteReplicaClientAcceptsCanonicalStrictResponses(t *testing.T) {
	t.Parallel()
	for _, test := range canonicalWebsiteReplicaResponseCases() {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := callWebsiteReplicaResponse(t, test.response, test.call); err != nil {
				t.Fatalf("canonical response was rejected: %v", err)
			}
		})
	}
}

func TestWebsiteReplicaClientRejectsUnknownFields(t *testing.T) {
	t.Parallel()
	tests := canonicalWebsiteReplicaResponseCases()
	mutations := []func(map[string]any){
		func(response map[string]any) { response["unexpected"] = true },
		func(response map[string]any) { response["product"].(map[string]any)["unexpected"] = true },
		func(response map[string]any) { response["creator"].(map[string]any)["unexpected"] = true },
		func(response map[string]any) {
			response["paymentOptions"].([]any)[0].(map[string]any)["unexpected"] = true
		},
		func(response map[string]any) { response["paymentAction"].(map[string]any)["unexpected"] = true },
		func(response map[string]any) {
			response["fulfillment"].(map[string]any)["tasks"].([]any)[0].(map[string]any)["unexpected"] = true
		},
		func(response map[string]any) {
			response["license"].(map[string]any)["claims"].(map[string]any)["unexpected"] = true
		},
	}
	for index, mutation := range mutations {
		test := tests[index]
		response := cloneReplicaResponse(t, test.response)
		mutation(response)
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertInvalidWebsiteReplicaResponse(t, callWebsiteReplicaResponse(t, response, test.call))
		})
	}
}

func TestWebsiteReplicaClientRejectsMissingRequiredFields(t *testing.T) {
	t.Parallel()
	tests := canonicalWebsiteReplicaResponseCases()
	mutations := []func(map[string]any){
		func(response map[string]any) { delete(response["upload"].(map[string]any), "headers") },
		func(response map[string]any) { delete(response, "publishedAt") },
		func(response map[string]any) { delete(response, "summary") },
		func(response map[string]any) { delete(response, "contractSummary") },
		func(response map[string]any) { delete(response, "paymentAction") },
		func(response map[string]any) { delete(response, "serviceCase") },
		func(response map[string]any) { delete(response, "expiresAt") },
	}
	for index, mutation := range mutations {
		test := tests[index]
		response := cloneReplicaResponse(t, test.response)
		mutation(response)
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertInvalidWebsiteReplicaResponse(t, callWebsiteReplicaResponse(t, response, test.call))
		})
	}
}

func TestWebsiteReplicaClientRejectsInvalidTimestamps(t *testing.T) {
	t.Parallel()
	tests := canonicalWebsiteReplicaResponseCases()
	mutations := []func(map[string]any){
		func(response map[string]any) {
			response["upload"].(map[string]any)["expiresAt"] = "2026-09-01T20:34:56+08:00"
		},
		func(response map[string]any) { response["publishedAt"] = "2026-02-30T12:34:56Z" },
		func(response map[string]any) { response["publishedAt"] = "2026-09-01T24:00:00Z" },
		func(response map[string]any) { response["expiresAt"] = "2026-09-01T12:34:56z" },
		func(response map[string]any) { response["expiresAt"] = "2026-09-01T12:34:56" },
		func(response map[string]any) {
			response["payment"].(map[string]any)["paidAt"] = "2026-09-01T20:34:56+08:00"
		},
		func(response map[string]any) {
			response["license"].(map[string]any)["claims"].(map[string]any)["issuedAt"] = "not-a-time"
		},
	}
	for index, mutation := range mutations {
		test := tests[index]
		if index == 2 {
			test = tests[1]
		}
		response := cloneReplicaResponse(t, test.response)
		mutation(response)
		t.Run(test.name+"-"+string(rune('a'+index)), func(t *testing.T) {
			t.Parallel()
			assertInvalidWebsiteReplicaResponse(t, callWebsiteReplicaResponse(t, response, test.call))
		})
	}
}

func TestWebsiteReplicaClientRejectsCrossResourceMismatches(t *testing.T) {
	t.Parallel()
	tests := []websiteReplicaResponseCase{
		{
			name: "published Replica",
			response: mutateReplicaResponse(t, replicaPublicationResponse(), func(response map[string]any) {
				response["replicaId"] = testUploadID
			}),
			call: canonicalWebsiteReplicaResponseCases()[1].call,
		},
		{
			name: "resolved short code",
			response: mutateReplicaResponse(t, replicaResolutionResponse(), func(response map[string]any) {
				response["shortCode"] = "VMR-TSRQPONMLKJIHGFEDCBA"
			}),
			call: canonicalWebsiteReplicaResponseCases()[2].call,
		},
		{
			name: "order status",
			response: mutateReplicaResponse(t, replicaOrderStatusResponse(), func(response map[string]any) {
				response["orderNo"] = "VMO-20260901-999999"
			}),
			call: canonicalWebsiteReplicaResponseCases()[5].call,
		},
		{
			name: "service case order",
			response: mutateReplicaResponse(t, replicaOrderStatusResponse(), func(response map[string]any) {
				response["serviceCase"].(map[string]any)["orderNo"] = "VMO-20260901-999999"
			}),
			call: canonicalWebsiteReplicaResponseCases()[5].call,
		},
		{
			name: "service case fulfillment",
			response: mutateReplicaResponse(t, replicaOrderStatusResponse(), func(response map[string]any) {
				response["serviceCase"].(map[string]any)["fulfillmentId"] = testVersionID
			}),
			call: canonicalWebsiteReplicaResponseCases()[5].call,
		},
		{
			name: "service case without fulfillment",
			response: mutateReplicaResponse(t, replicaOrderStatusResponse(), func(response map[string]any) {
				response["fulfillment"] = nil
			}),
			call: canonicalWebsiteReplicaResponseCases()[5].call,
		},
		{
			name: "download license",
			response: mutateReplicaResponse(t, replicaDownloadResponse(), func(response map[string]any) {
				response["license"].(map[string]any)["claims"].(map[string]any)["versionId"] = testUploadID
			}),
			call: canonicalWebsiteReplicaResponseCases()[6].call,
		},
		{
			name: "download license version",
			response: mutateReplicaResponse(t, replicaDownloadResponse(), func(response map[string]any) {
				response["license"].(map[string]any)["claims"].(map[string]any)["version"] = 2
			}),
			call: canonicalWebsiteReplicaResponseCases()[6].call,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertInvalidWebsiteReplicaResponse(t, callWebsiteReplicaResponse(t, test.response, test.call))
		})
	}
}

func TestWebsiteReplicaClientRejectsJSAPIActionForNativeReplicaQuote(t *testing.T) {
	t.Parallel()
	response := replicaOrderResponse()
	response["paymentAction"] = map[string]any{
		"type": "JSAPI", "appId": "wx-app", "timeStamp": "1234567890",
		"nonceStr": "nonce", "package": "prepay_id=example", "signType": "RSA", "paySign": "signature",
	}
	assertInvalidWebsiteReplicaResponse(t, callWebsiteReplicaResponse(t, response, canonicalWebsiteReplicaResponseCases()[4].call))
}

func TestWebsiteReplicaClientAcceptsQRCodeAction(t *testing.T) {
	t.Parallel()
	response := replicaOrderResponse()
	response["paymentAction"] = map[string]any{
		"type": "QR_CODE", "content": "weixin://wxpay/bizpayurl?pr=replica-test",
	}
	if err := callWebsiteReplicaResponse(t, response, canonicalWebsiteReplicaResponseCases()[4].call); err != nil {
		t.Fatalf("valid QR_CODE response was rejected: %v", err)
	}
}

func TestWebsiteReplicaDatetimeMatchesZodContract(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"2026-09-01T12:34Z",
		"2026-09-01T12:34:56Z",
		"2026-09-01T12:34:56.1234567890Z",
	} {
		if !validZodDatetime(value) {
			t.Errorf("valid datetime was rejected: %s", value)
		}
	}
	for _, value := range []string{
		"2026-09-01T20:34:56+08:00",
		"2026-09-01T12:34:56",
		"2026-09-01T12:34:56z",
		"2026-02-30T12:34:56Z",
		"2026-09-01T24:00:00Z",
	} {
		if validZodDatetime(value) {
			t.Errorf("invalid datetime was accepted: %s", value)
		}
	}
}

func TestWebsiteReplicaURLAndUUIDMatchZodContract(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"https://example.com/%zz",
		"foo:bar",
		"mailto:user@example.com",
		"urn:isbn:9780141036144",
	} {
		if !validAbsoluteURL(value) {
			t.Errorf("valid URL was rejected: %s", value)
		}
	}
	for _, value := range []string{"relative/path", "http://", "https://exa mple.com"} {
		if validAbsoluteURL(value) {
			t.Errorf("invalid URL was accepted: %s", value)
		}
	}
	for _, value := range []string{
		"00000000-0000-0000-0000-000000000000",
		"ffffffff-ffff-ffff-ffff-ffffffffffff",
		"11111111-1111-4111-8111-111111111111",
	} {
		if !zodUUIDPattern.MatchString(value) {
			t.Errorf("valid UUID was rejected: %s", value)
		}
	}
	if zodUUIDPattern.MatchString("11111111-1111-0111-8111-111111111111") {
		t.Error("invalid UUID version was accepted")
	}
}

func TestWebsiteReplicaClientDistinguishesMissingFromNullableFields(t *testing.T) {
	t.Parallel()
	response := replicaOrderResponse()
	response["status"] = "PAID"
	response["paymentAction"] = nil
	call := canonicalWebsiteReplicaResponseCases()[4].call
	if err := callWebsiteReplicaResponse(t, response, call); err != nil {
		t.Fatalf("explicit null paymentAction was rejected: %v", err)
	}
	delete(response, "paymentAction")
	assertInvalidWebsiteReplicaResponse(t, callWebsiteReplicaResponse(t, response, call))
}

func canonicalWebsiteReplicaResponseCases() []websiteReplicaResponseCase {
	return []websiteReplicaResponseCase{
		{
			name: "authorize upload", response: replicaUploadResponse(),
			call: func(client *Client) error {
				_, err := client.CreateWebsiteReplicaUpload(context.Background(), CreateWebsiteReplicaUploadRequest{})
				return err
			},
		},
		{
			name: "complete upload", response: replicaPublicationResponse(),
			call: func(client *Client) error {
				_, err := client.CompleteWebsiteReplicaUpload(context.Background(), testReplicaID, testUploadID)
				return err
			},
		},
		{
			name: "resolve", response: replicaResolutionResponse(),
			call: func(client *Client) error {
				_, err := client.ResolveWebsiteReplica(context.Background(), "VICEME-REPLICA:"+testShortCode)
				return err
			},
		},
		{
			name: "quote", response: replicaQuoteResponse(),
			call: func(client *Client) error {
				_, err := client.CreateWebsiteReplicaQuote(context.Background(), CreateWebsiteReplicaQuoteRequest{})
				return err
			},
		},
		{
			name: "order", response: replicaOrderResponse(),
			call: func(client *Client) error {
				_, err := client.CreateWebsiteReplicaOrder(context.Background(), CreateWebsiteReplicaOrderRequest{})
				return err
			},
		},
		{
			name: "order status", response: replicaOrderStatusResponse(),
			call: func(client *Client) error {
				_, err := client.GetWebsiteReplicaOrderStatus(context.Background(), testOrderNo)
				return err
			},
		},
		{
			name: "download", response: replicaDownloadResponse(),
			call: func(client *Client) error {
				_, err := client.GetWebsiteReplicaDownload(context.Background(), testShortCode)
				return err
			},
		},
	}
}

func replicaUploadResponse() map[string]any {
	return map[string]any{
		"replicaId": testReplicaID, "uploadId": testUploadID,
		"upload": map[string]any{
			"method": "PUT", "url": "https://objects.example/source.zip",
			"headers": map[string]string{}, "expiresAt": testReplicaTimestamp,
		},
	}
}

func replicaPublicationResponse() map[string]any {
	return map[string]any{
		"replicaId": testReplicaID, "versionId": testVersionID, "version": 1,
		"shortCode": testShortCode, "instruction": "VICEME-REPLICA:" + testShortCode,
		"product": replicaProductResponse(), "publishedAt": testReplicaTimestamp,
	}
}

func replicaResolutionResponse() map[string]any {
	return map[string]any{
		"replicaId": testReplicaID, "shortCode": testShortCode, "title": "Replica", "summary": "Source",
		"creator": map[string]any{"handle": "replica-maker", "displayName": "Replica Maker"},
		"product": replicaProductResponse(),
	}
}

func replicaProductResponse() map[string]any {
	return map[string]any{
		"id": testReplicaProductID, "skuId": testSKUID, "title": "Replica", "currency": "CNY", "priceCents": 990,
	}
}

func replicaQuoteResponse() map[string]any {
	return map[string]any{
		"id":      testQuoteID,
		"product": map[string]any{"id": testReplicaProductID, "slug": "replica-source", "title": "Replica"},
		"attribution": map[string]any{
			"subjectWorkId": testWorkID, "entryWorkId": nil, "commerceApplicationId": nil,
		},
		"sku": map[string]any{
			"id": testSKUID, "code": "default", "title": "\u6c38\u4e45\u6e90\u7801\u4e0b\u8f7d", "selectedOptions": map[string]string{},
		},
		"currency": "CNY", "unitAmountCents": 990, "quantity": 1,
		"subtotalAmountCents": 990, "shippingAmountCents": 0, "totalAmountCents": 990,
		"contractSummary": map[string]any{
			"publicFields": map[string]any{}, "sensitiveFieldKeys": []string{}, "assetCount": 0,
		},
		"fulfillment":    map[string]any{"capabilities": []string{"DIGITAL_ENTITLEMENT"}, "estimatedState": "AWAITING_PAYMENT"},
		"paymentOptions": []map[string]any{{"provider": "WECHAT_PAY", "scenes": []string{"NATIVE"}}},
		"expiresAt":      testReplicaTimestamp,
	}
}

func replicaOrderResponse() map[string]any {
	return map[string]any{
		"orderNo": testOrderNo, "status": "PENDING",
		"paymentAction": map[string]any{"type": "REDIRECT", "url": "https://pay.example/checkout"},
		"expiresAt":     testReplicaTimestamp,
	}
}

func replicaOrderStatusResponse() map[string]any {
	task := map[string]any{
		"id": testTaskID, "sequence": 1, "capabilityCode": "DIGITAL_ENTITLEMENT", "status": "SUCCEEDED", "version": 1,
		"failureCode": nil, "resultSummary": nil, "startedAt": testReplicaTimestamp, "completedAt": testReplicaTimestamp,
	}
	return map[string]any{
		"orderNo": testOrderNo,
		"payment": map[string]any{"status": "PAID", "paidAt": testReplicaTimestamp, "closedAt": nil},
		"fulfillment": map[string]any{
			"id": testFulfillmentID, "status": "SUCCEEDED", "version": 1,
			"currentTask": task, "tasks": []map[string]any{task}, "failureCode": nil, "resultSummary": nil,
		},
		"serviceCase": map[string]any{
			"id": testCaseID, "caseNo": "VMC-20260901-000001", "orderNo": testOrderNo, "fulfillmentId": testFulfillmentID,
			"work":     map[string]any{"creatorHandle": "replica-maker", "slug": "replica-source", "title": "Replica"},
			"merchant": map[string]any{"id": testMerchantID, "displayName": "Replica Maker"},
			"status":   "SUBMITTED", "currentStageCode": "submitted",
			"stages": []map[string]any{{"code": "submitted", "label": "Submitted", "terminal": false}},
			"intake": map[string]any{}, "publicProgress": map[string]any{}, "lockVersion": 1,
			"events": []map[string]any{{
				"sequence": 1, "fromStatus": nil, "toStatus": "SUBMITTED", "stageCode": "submitted",
				"actorType": "SYSTEM", "note": nil, "publicMessage": nil, "createdAt": testReplicaTimestamp,
			}},
			"submittedAt": testReplicaTimestamp, "completedAt": nil, "updatedAt": testReplicaTimestamp,
		},
	}
}

func replicaDownloadResponse() map[string]any {
	return map[string]any{
		"replicaId": testReplicaID, "versionId": testVersionID, "version": 1,
		"fileName": "source.zip", "sizeBytes": 1024, "artifactDigest": strings.Repeat("a", 64),
		"downloadUrl": "https://objects.example/source.zip", "expiresAt": testReplicaTimestamp,
		"license": map[string]any{
			"claims": map[string]any{
				"schemaVersion": "website-replica-license/v1", "entitlementId": testEntitlementID,
				"replicaId": testReplicaID, "versionId": testVersionID, "version": 1, "orderNo": testOrderNo,
				"artifactDigest": strings.Repeat("a", 64), "licenseTermsVersion": "website-replica-license/v1", "issuedAt": testReplicaTimestamp,
			},
			"algorithm": "Ed25519", "signingKeyId": "replica-signing-v1",
			"signingPublicKey": strings.Repeat("p", 32), "signature": strings.Repeat("s", 32),
		},
	}
}

func callWebsiteReplicaResponse(t *testing.T, response map[string]any, call func(*Client) error) error {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(writer).Encode(response); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	return call(NewClient(server.URL, server.Client(), staticToken("vme_cli_test"), "viceme/test"))
}

func assertInvalidWebsiteReplicaResponse(t *testing.T, err error) {
	t.Helper()
	cliError := output.AsError(err)
	if err == nil || cliError.Subtype != "RESPONSE_INVALID" || cliError.Retryable {
		t.Fatalf("invalid response was accepted: %#v", cliError)
	}
}

func mutateReplicaResponse(t *testing.T, response map[string]any, mutation func(map[string]any)) map[string]any {
	t.Helper()
	cloned := cloneReplicaResponse(t, response)
	mutation(cloned)
	return cloned
}

func cloneReplicaResponse(t *testing.T, response map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var cloned map[string]any
	if err := json.Unmarshal(data, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}
