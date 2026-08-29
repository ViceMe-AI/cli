package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/output"
)

const (
	testMerchantAccountID     = "11111111-1111-4111-8111-111111111111"
	testWebsiteWorkID         = "22222222-2222-4222-8222-222222222222"
	testWebsiteVerificationID = "33333333-3333-4333-8333-333333333333"
	testCommerceApplicationID = "44444444-4444-4444-8444-444444444444"
	testOriginID              = "55555555-5555-4555-8555-555555555555"
	testReturnURLID           = "66666666-6666-4666-8666-666666666666"
	testOtherID               = "77777777-7777-4777-8777-777777777777"
	testProductID             = "88888888-8888-4888-8888-888888888888"
	testTimestamp             = "2026-08-27T10:00:00Z"
)

func TestMerchantEngagementClientUsesShopContracts(t *testing.T) {
	t.Parallel()

	createVerificationRequest := CreateWebsiteVerificationRequest{
		MerchantAccountID: testMerchantAccountID,
		ExpectedRevision:  3,
	}
	verifyWebsiteRequest := VerifyWebsiteRequest{
		MerchantAccountID:           testMerchantAccountID,
		ExpectedVerificationVersion: 2,
	}
	revokeWebsiteRequest := RevokeWebsiteOwnershipRequest{
		MerchantAccountID: testMerchantAccountID,
		ExpectedRevision:  7,
	}
	createAccessRequest := CreateWorkSdkAccessRequest{
		MerchantAccountID: testMerchantAccountID,
		Features:          []string{"danmaku", "tip"},
	}
	updateAccessRequest := UpdateWorkSdkAccessRequest{
		MerchantAccountID:     testMerchantAccountID,
		ExpectedConfigVersion: 1,
		Features:              []string{"tip"},
	}
	createApplicationRequest := CreateCommerceApplicationRequest{
		MerchantAccountID: testMerchantAccountID,
		WorkID:            testWebsiteWorkID,
		Kind:              "WEBSITE_WIDGET",
		Environment:       "SANDBOX",
		DisplayName:       "Demo Widget",
		Origins:           []string{"https://example.com"},
		ReturnURLs:        []string{"https://example.com/return"},
	}
	normalizedApplicationRequest := CreateCommerceApplicationRequest{
		MerchantAccountID: testMerchantAccountID,
		WorkID:            testWebsiteWorkID,
		Kind:              "WEBSITE_WIDGET",
		Environment:       "SANDBOX",
		DisplayName:       "  Normalized Widget  ",
		Origins:           []string{"https://EXAMPLE.COM:0443/"},
		ReturnURLs:        []string{`https://example.com:00080/a\..\b^|c`},
	}
	internationalApplicationRequest := CreateCommerceApplicationRequest{
		MerchantAccountID: testMerchantAccountID,
		WorkID:            testWebsiteWorkID,
		Kind:              "WEBSITE_WIDGET",
		Environment:       "SANDBOX",
		DisplayName:       "国际化组件",
		Origins:           []string{"https://例子.com/"},
		ReturnURLs:        []string{"https://例子.com/a/%2e%2e/return?message=你好 世界&next=a/b?c"},
	}
	numericIPApplicationRequest := CreateCommerceApplicationRequest{
		MerchantAccountID: testMerchantAccountID,
		WorkID:            testWebsiteWorkID,
		Kind:              "WEBSITE_WIDGET",
		Environment:       "SANDBOX",
		DisplayName:       "Numeric IP Widget",
		Origins:           []string{"https://2130706433", "https://[0:0:0:0:0:0:0:1]"},
		ReturnURLs:        []string{"https://0x7f000001/a/../return", "https://[0:0:0:0:0:0:0:1]/a/../return"},
	}
	unicodeDisplayName := strings.Repeat("赏", 41)
	updatedName := "Updated Widget"
	updatedOrigins := []string{"https://widget.example.com"}
	updateApplicationRequest := UpdateCommerceApplicationRequest{
		MerchantAccountID: testMerchantAccountID,
		ExpectedRevision:  1,
		DisplayName:       &updatedName,
		Origins:           &updatedOrigins,
	}
	activateRequest := CommerceApplicationCommand{MerchantAccountID: testMerchantAccountID, ExpectedRevision: 2}
	suspendRequest := CommerceApplicationCommand{MerchantAccountID: testMerchantAccountID, ExpectedRevision: 3}

	tests := []struct {
		name     string
		method   string
		path     string
		query    string
		body     any
		response any
		call     func(*Client) error
	}{
		{
			name: "create Website verification", method: http.MethodPost,
			path: "/v1/cli/merchant/works/" + testWebsiteWorkID + "/website-verifications",
			body: createVerificationRequest, response: testWebsiteVerification("PENDING", true),
			call: func(client *Client) error {
				_, err := client.CreateWebsiteVerification(context.Background(), testWebsiteWorkID, createVerificationRequest)
				return err
			},
		},
		{
			name: "get latest Website verification", method: http.MethodGet,
			path:  "/v1/cli/merchant/works/" + testWebsiteWorkID + "/website-verifications/latest",
			query: "merchantAccountId=" + testMerchantAccountID, response: testWebsiteVerification("VERIFIED", false),
			call: func(client *Client) error {
				_, err := client.GetLatestWebsiteVerification(context.Background(), testWebsiteWorkID, testMerchantAccountID)
				return err
			},
		},
		{
			name: "verify Website", method: http.MethodPost,
			path: "/v1/cli/merchant/works/" + testWebsiteWorkID + "/website-verifications/verify",
			body: verifyWebsiteRequest, response: testWebsiteMerchantWork("VERIFIED", 4, 2),
			call: func(client *Client) error {
				_, err := client.VerifyWebsite(context.Background(), testWebsiteWorkID, verifyWebsiteRequest)
				return err
			},
		},
		{
			name: "revoke Website ownership", method: http.MethodPost,
			path: "/v1/cli/merchant/works/" + testWebsiteWorkID + "/website-verifications/revoke",
			body: revokeWebsiteRequest, response: testWebsiteMerchantWork("REVOKED", 8, 2),
			call: func(client *Client) error {
				_, err := client.RevokeWebsiteOwnership(context.Background(), testWebsiteWorkID, revokeWebsiteRequest)
				return err
			},
		},
		{
			name: "create Work SDK access", method: http.MethodPost,
			path: "/v1/cli/merchant/works/" + testWebsiteWorkID + "/sdk-access",
			body: createAccessRequest, response: testWorkSdkAccess(testWebsiteWorkID, "ACTIVE", 1, []string{"tip", "danmaku"}),
			call: func(client *Client) error {
				_, err := client.CreateWorkSdkAccess(context.Background(), testWebsiteWorkID, createAccessRequest)
				return err
			},
		},
		{
			name: "get Work SDK access", method: http.MethodGet,
			path:  "/v1/cli/merchant/works/" + testWebsiteWorkID + "/sdk-access",
			query: "merchantAccountId=" + testMerchantAccountID, response: testWorkSdkAccess(testWebsiteWorkID, "ACTIVE", 1, []string{"tip"}),
			call: func(client *Client) error {
				_, err := client.GetWorkSdkAccess(context.Background(), testWebsiteWorkID, testMerchantAccountID)
				return err
			},
		},
		{
			name: "list Work SDK accesses", method: http.MethodGet,
			path: "/v1/cli/merchant/work-sdk-accesses", query: "merchantAccountId=" + testMerchantAccountID,
			response: WorkSdkAccessesResponse{Items: []WorkSdkAccess{testWorkSdkAccess(testWebsiteWorkID, "ACTIVE", 1, []string{"tip"})}},
			call: func(client *Client) error {
				_, err := client.ListWorkSdkAccesses(context.Background(), testMerchantAccountID)
				return err
			},
		},
		{
			name: "update Work SDK access", method: http.MethodPut,
			path: "/v1/cli/merchant/works/" + testWebsiteWorkID + "/sdk-access",
			body: updateAccessRequest, response: testWorkSdkAccess(testWebsiteWorkID, "ACTIVE", 2, []string{"tip"}),
			call: func(client *Client) error {
				_, err := client.UpdateWorkSdkAccess(context.Background(), testWebsiteWorkID, updateAccessRequest)
				return err
			},
		},
		{
			name: "disable Work SDK access", method: http.MethodDelete,
			path:  "/v1/cli/merchant/works/" + testWebsiteWorkID + "/sdk-access",
			query: "merchantAccountId=" + testMerchantAccountID, response: testWorkSdkAccess(testWebsiteWorkID, "DISABLED", 2, []string{"tip"}),
			call: func(client *Client) error {
				_, err := client.DisableWorkSdkAccess(context.Background(), testWebsiteWorkID, testMerchantAccountID)
				return err
			},
		},
		{
			name: "create Commerce Application", method: http.MethodPost,
			path: "/v1/cli/merchant/commerce-applications", body: createApplicationRequest,
			response: testCommerceApplication("WEBSITE_WIDGET", "SANDBOX", "DRAFT", 1, "Demo Widget", []string{"https://example.com"}, []string{"https://example.com/return"}),
			call: func(client *Client) error {
				_, err := client.CreateCommerceApplication(context.Background(), createApplicationRequest)
				return err
			},
		},
		{
			name: "create Commerce Application with contract normalization", method: http.MethodPost,
			path: "/v1/cli/merchant/commerce-applications", body: normalizedApplicationRequest,
			response: testCommerceApplication("WEBSITE_WIDGET", "SANDBOX", "DRAFT", 1, "Normalized Widget", []string{"https://example.com"}, []string{"https://example.com:80/b%5E|c"}),
			call: func(client *Client) error {
				_, err := client.CreateCommerceApplication(context.Background(), normalizedApplicationRequest)
				return err
			},
		},
		{
			name: "create Commerce Application with an international domain", method: http.MethodPost,
			path: "/v1/cli/merchant/commerce-applications", body: internationalApplicationRequest,
			response: testCommerceApplication("WEBSITE_WIDGET", "SANDBOX", "DRAFT", 1, "国际化组件", []string{"https://xn--fsqu00a.com"}, []string{"https://xn--fsqu00a.com/return?message=%E4%BD%A0%E5%A5%BD%20%E4%B8%96%E7%95%8C&next=a/b?c"}),
			call: func(client *Client) error {
				_, err := client.CreateCommerceApplication(context.Background(), internationalApplicationRequest)
				return err
			},
		},
		{
			name: "create Commerce Application with WHATWG IP forms", method: http.MethodPost,
			path: "/v1/cli/merchant/commerce-applications", body: numericIPApplicationRequest,
			response: testCommerceApplication("WEBSITE_WIDGET", "SANDBOX", "DRAFT", 1, "Numeric IP Widget", []string{"https://127.0.0.1", "https://[::1]"}, []string{"https://127.0.0.1/return", "https://[::1]/return"}),
			call: func(client *Client) error {
				_, err := client.CreateCommerceApplication(context.Background(), numericIPApplicationRequest)
				return err
			},
		},
		{
			name: "list Commerce Applications", method: http.MethodGet,
			path: "/v1/cli/merchant/commerce-applications", query: "merchantAccountId=" + testMerchantAccountID,
			response: CommerceApplicationsResponse{Items: []CommerceApplication{
				testCommerceApplication("WEBSITE_WIDGET", "SANDBOX", "DRAFT", 1, "Demo Widget", []string{"https://example.com"}, []string{}),
			}},
			call: func(client *Client) error {
				_, err := client.ListCommerceApplications(context.Background(), testMerchantAccountID)
				return err
			},
		},
		{
			name: "get Commerce Application", method: http.MethodGet,
			path:     "/v1/cli/merchant/commerce-applications/" + testCommerceApplicationID,
			query:    "merchantAccountId=" + testMerchantAccountID,
			response: testCommerceApplication("WEBSITE_WIDGET", "SANDBOX", "DRAFT", 1, "Demo Widget", []string{"https://example.com"}, []string{}),
			call: func(client *Client) error {
				_, err := client.GetCommerceApplication(context.Background(), testCommerceApplicationID, testMerchantAccountID)
				return err
			},
		},
		{
			name: "get Commerce Application with multibyte display name", method: http.MethodGet,
			path:     "/v1/cli/merchant/commerce-applications/" + testCommerceApplicationID,
			query:    "merchantAccountId=" + testMerchantAccountID,
			response: testCommerceApplication("WEBSITE_WIDGET", "SANDBOX", "DRAFT", 1, unicodeDisplayName, []string{"https://example.com"}, []string{}),
			call: func(client *Client) error {
				_, err := client.GetCommerceApplication(context.Background(), testCommerceApplicationID, testMerchantAccountID)
				return err
			},
		},
		{
			name: "update Commerce Application", method: http.MethodPatch,
			path:     "/v1/cli/merchant/commerce-applications/" + testCommerceApplicationID,
			body:     updateApplicationRequest,
			response: testCommerceApplication("WEBSITE_WIDGET", "SANDBOX", "DRAFT", 2, updatedName, updatedOrigins, []string{}),
			call: func(client *Client) error {
				_, err := client.UpdateCommerceApplication(context.Background(), testCommerceApplicationID, updateApplicationRequest)
				return err
			},
		},
		{
			name: "activate Commerce Application", method: http.MethodPost,
			path:     "/v1/cli/merchant/commerce-applications/" + testCommerceApplicationID + "/activate",
			body:     activateRequest,
			response: testCommerceApplication("WEBSITE_WIDGET", "SANDBOX", "ACTIVE", 3, updatedName, updatedOrigins, []string{}),
			call: func(client *Client) error {
				_, err := client.CommandCommerceApplication(context.Background(), testCommerceApplicationID, "activate", activateRequest)
				return err
			},
		},
		{
			name: "suspend Commerce Application", method: http.MethodPost,
			path:     "/v1/cli/merchant/commerce-applications/" + testCommerceApplicationID + "/suspend",
			body:     suspendRequest,
			response: testCommerceApplication("WEBSITE_WIDGET", "SANDBOX", "SUSPENDED", 4, updatedName, updatedOrigins, []string{}),
			call: func(client *Client) error {
				_, err := client.CommandCommerceApplication(context.Background(), testCommerceApplicationID, "suspend", suspendRequest)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != test.method || request.URL.Path != test.path || request.URL.RawQuery != test.query {
					t.Errorf("request = %s %s?%s, want %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery, test.method, test.path, test.query)
				}
				if authorization := request.Header.Get("Authorization"); authorization != "Bearer vme_cli_test" {
					t.Errorf("Authorization = %q", authorization)
				}
				assertRequestJSON(t, request, test.body)
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write(mustJSON(test.response))
			}))
			defer server.Close()

			client := NewClient(server.URL, server.Client(), staticToken("vme_cli_test"), "viceme/test")
			if err := test.call(client); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMerchantEngagementRecoveryUsesAuthoritativeResponseContracts(t *testing.T) {
	t.Parallel()

	latest := testWebsiteVerification("PENDING", false)
	if err := latest.validateAPIResponse(); err != nil {
		t.Fatalf("latest verification without plaintext challenge: %v", err)
	}
	if latest.Challenge != nil {
		t.Fatal("latest verification unexpectedly exposes a plaintext challenge")
	}

	for _, status := range []string{"DRAFT", "ACTIVE", "SUSPENDED", "REVOKED"} {
		if !validCommerceApplicationStatus(status) {
			t.Fatalf("authoritative Commerce Application status %q was rejected", status)
		}
	}
	if validCommerceApplicationStatus("ARCHIVED") {
		t.Fatal("Commerce Application contract accepted Product-only ARCHIVED status")
	}
}

func TestMerchantEngagementClientRejectsInvalidSuccessfulResponses(t *testing.T) {
	t.Parallel()

	createAccessRequest := CreateWorkSdkAccessRequest{
		MerchantAccountID: testMerchantAccountID,
		Features:          []string{"danmaku", "tip"},
	}
	createApplicationRequest := CreateCommerceApplicationRequest{
		MerchantAccountID: testMerchantAccountID,
		WorkID:            testWebsiteWorkID,
		Kind:              "WEBSITE_WIDGET",
		Environment:       "SANDBOX",
		DisplayName:       "Demo Widget",
		Origins:           []string{"https://example.com"},
		ReturnURLs:        []string{"https://example.com/return"},
	}
	verifyRequest := VerifyWebsiteRequest{MerchantAccountID: testMerchantAccountID, ExpectedVerificationVersion: 2}
	revokeRequest := RevokeWebsiteOwnershipRequest{MerchantAccountID: testMerchantAccountID, ExpectedRevision: 7}
	updatedName := "Updated Widget"

	emptyVerificationID := testWebsiteVerification("VERIFIED", false)
	emptyVerificationID.ID = ""
	zeroVerificationVersion := testWebsiteVerification("VERIFIED", false)
	zeroVerificationVersion.Version = 0
	wrongVerificationWork := testWebsiteVerification("VERIFIED", false)
	wrongVerificationWork.WebsiteWorkID = testOtherID
	wrongAccessWork := testWorkSdkAccess(testOtherID, "ACTIVE", 1, []string{"tip"})
	zeroAccessVersion := testWorkSdkAccess(testWebsiteWorkID, "ACTIVE", 0, []string{"tip"})
	nonAdvancingAccessVersion := testWorkSdkAccess(testWebsiteWorkID, "ACTIVE", 1, []string{"tip"})
	invalidAccessFeature := testWorkSdkAccess(testWebsiteWorkID, "ACTIVE", 1, []string{"other"})
	duplicateAccessFeature := testWorkSdkAccess(testWebsiteWorkID, "ACTIVE", 1, []string{"tip", "tip"})
	incompleteAccessFeatures := testWorkSdkAccess(testWebsiteWorkID, "ACTIVE", 1, []string{"danmaku"})
	wrongWorkKind := testWebsiteMerchantWork("VERIFIED", 4, 2)
	wrongWorkKind.Kind = "SKILL"
	wrongWorkKind.Skill = json.RawMessage(`{"listingId":"99999999-9999-4999-8999-999999999999","listingStatus":"DRAFT"}`)
	wrongWorkKind.Website = nil
	wrongWorkOrigin := testWebsiteMerchantWork("VERIFIED", 4, 2)
	wrongWorkOrigin.Origin = "PLATFORM_AUTHORED"
	wrongOwnershipStatus := testWebsiteMerchantWork("UNVERIFIED", 4, 2)
	wrongVerificationVersion := testWebsiteMerchantWork("VERIFIED", 4, 3)
	nonAdvancingWorkRevision := testWebsiteMerchantWork("REVOKED", 7, 2)
	nonCanonicalWebsiteOrigin := testWebsiteMerchantWork("VERIFIED", 4, 2)
	nonCanonicalWebsiteOrigin.Website.CanonicalOrigin = "https://EXAMPLE.com"
	websiteOriginWithCredentials := testWebsiteMerchantWork("VERIFIED", 4, 2)
	websiteOriginWithCredentials.Website.CanonicalOrigin = "https://user:password@example.com"
	websiteOriginWithPath := testWebsiteMerchantWork("VERIFIED", 4, 2)
	websiteOriginWithPath.Website.CanonicalOrigin = "https://example.com/path"
	websiteOriginWithQuery := testWebsiteMerchantWork("VERIFIED", 4, 2)
	websiteOriginWithQuery.Website.CanonicalOrigin = "https://example.com/?source=test"
	websiteOriginWithFragment := testWebsiteMerchantWork("VERIFIED", 4, 2)
	websiteOriginWithFragment.Website.CanonicalOrigin = "https://example.com/#fragment"
	wrongApplicationID := testCommerceApplication("WEBSITE_WIDGET", "SANDBOX", "DRAFT", 1, "Demo Widget", []string{"https://example.com"}, []string{})
	wrongApplicationID.ID = testOtherID
	zeroApplicationRevision := testCommerceApplication("WEBSITE_WIDGET", "SANDBOX", "DRAFT", 0, "Demo Widget", []string{"https://example.com"}, []string{})
	wrongApplicationWork := testCommerceApplication("WEBSITE_WIDGET", "SANDBOX", "DRAFT", 1, "Demo Widget", []string{"https://example.com"}, []string{"https://example.com/return"})
	wrongApplicationWork.WorkID = testOtherID
	wrongApplicationKind := testCommerceApplication("HOSTED_CHECKOUT", "SANDBOX", "DRAFT", 1, "Demo Widget", []string{"https://example.com"}, []string{"https://example.com/return"})
	wrongApplicationEnvironment := testCommerceApplication("WEBSITE_WIDGET", "PRODUCTION", "DRAFT", 1, "Demo Widget", []string{"https://example.com"}, []string{"https://example.com/return"})
	wrongApplicationOrigin := testCommerceApplication("WEBSITE_WIDGET", "SANDBOX", "DRAFT", 1, "Demo Widget", []string{"https://other.example.com"}, []string{"https://example.com/return"})
	wrongApplicationReturnURL := testCommerceApplication("WEBSITE_WIDGET", "SANDBOX", "DRAFT", 1, "Demo Widget", []string{"https://example.com"}, []string{"https://example.com/other"})
	widgetWithProduct := testCommerceApplication("WEBSITE_WIDGET", "SANDBOX", "DRAFT", 1, "Demo Widget", []string{"https://example.com"}, []string{})
	widgetWithProduct.Products = []CommerceApplicationProduct{{ProductID: testProductID, Title: "Product", Status: "ACTIVE"}}
	nonAdvancingApplication := testCommerceApplication("WEBSITE_WIDGET", "SANDBOX", "DRAFT", 1, updatedName, []string{"https://example.com"}, []string{})
	nonAdvancingCommand := testCommerceApplication("WEBSITE_WIDGET", "SANDBOX", "ACTIVE", 2, "Demo Widget", []string{"https://example.com"}, []string{})

	tests := []struct {
		name     string
		response any
		call     func(*Client) error
	}{
		{
			name: "empty body", response: rawResponse("\n"),
			call: func(client *Client) error {
				_, err := client.GetWorkSdkAccess(context.Background(), testWebsiteWorkID, testMerchantAccountID)
				return err
			},
		},
		{
			name: "empty verification ID", response: emptyVerificationID,
			call: func(client *Client) error {
				_, err := client.GetLatestWebsiteVerification(context.Background(), testWebsiteWorkID, testMerchantAccountID)
				return err
			},
		},
		{
			name: "zero verification version", response: zeroVerificationVersion,
			call: func(client *Client) error {
				_, err := client.GetLatestWebsiteVerification(context.Background(), testWebsiteWorkID, testMerchantAccountID)
				return err
			},
		},
		{
			name: "wrong verification Work ID", response: wrongVerificationWork,
			call: func(client *Client) error {
				_, err := client.GetLatestWebsiteVerification(context.Background(), testWebsiteWorkID, testMerchantAccountID)
				return err
			},
		},
		{
			name: "wrong access Work ID", response: wrongAccessWork,
			call: func(client *Client) error {
				_, err := client.GetWorkSdkAccess(context.Background(), testWebsiteWorkID, testMerchantAccountID)
				return err
			},
		},
		{
			name: "zero access version", response: zeroAccessVersion,
			call: func(client *Client) error {
				_, err := client.GetWorkSdkAccess(context.Background(), testWebsiteWorkID, testMerchantAccountID)
				return err
			},
		},
		{
			name: "non-advancing access version", response: nonAdvancingAccessVersion,
			call: func(client *Client) error {
				_, err := client.UpdateWorkSdkAccess(context.Background(), testWebsiteWorkID, UpdateWorkSdkAccessRequest{
					MerchantAccountID:     testMerchantAccountID,
					ExpectedConfigVersion: 1,
					Features:              []string{"tip"},
				})
				return err
			},
		},
		{
			name: "invalid access feature", response: invalidAccessFeature,
			call: func(client *Client) error {
				_, err := client.GetWorkSdkAccess(context.Background(), testWebsiteWorkID, testMerchantAccountID)
				return err
			},
		},
		{
			name: "duplicate access feature", response: duplicateAccessFeature,
			call: func(client *Client) error {
				_, err := client.GetWorkSdkAccess(context.Background(), testWebsiteWorkID, testMerchantAccountID)
				return err
			},
		},
		{
			name: "incomplete requested feature set", response: incompleteAccessFeatures,
			call: func(client *Client) error {
				_, err := client.CreateWorkSdkAccess(context.Background(), testWebsiteWorkID, createAccessRequest)
				return err
			},
		},
		{
			name: "wrong Website Work kind", response: wrongWorkKind,
			call: func(client *Client) error {
				_, err := client.VerifyWebsite(context.Background(), testWebsiteWorkID, verifyRequest)
				return err
			},
		},
		{
			name: "wrong Website Work origin", response: wrongWorkOrigin,
			call: func(client *Client) error {
				_, err := client.VerifyWebsite(context.Background(), testWebsiteWorkID, verifyRequest)
				return err
			},
		},
		{
			name: "wrong Website ownership status", response: wrongOwnershipStatus,
			call: func(client *Client) error {
				_, err := client.VerifyWebsite(context.Background(), testWebsiteWorkID, verifyRequest)
				return err
			},
		},
		{
			name: "wrong Website verification version", response: wrongVerificationVersion,
			call: func(client *Client) error {
				_, err := client.VerifyWebsite(context.Background(), testWebsiteWorkID, verifyRequest)
				return err
			},
		},
		{
			name: "non-advancing Work revision", response: nonAdvancingWorkRevision,
			call: func(client *Client) error {
				_, err := client.RevokeWebsiteOwnership(context.Background(), testWebsiteWorkID, revokeRequest)
				return err
			},
		},
		{
			name: "non-canonical Website origin", response: nonCanonicalWebsiteOrigin,
			call: func(client *Client) error {
				_, err := client.VerifyWebsite(context.Background(), testWebsiteWorkID, verifyRequest)
				return err
			},
		},
		{
			name: "Website origin with credentials", response: websiteOriginWithCredentials,
			call: func(client *Client) error {
				_, err := client.VerifyWebsite(context.Background(), testWebsiteWorkID, verifyRequest)
				return err
			},
		},
		{
			name: "Website origin with path", response: websiteOriginWithPath,
			call: func(client *Client) error {
				_, err := client.VerifyWebsite(context.Background(), testWebsiteWorkID, verifyRequest)
				return err
			},
		},
		{
			name: "Website origin with query", response: websiteOriginWithQuery,
			call: func(client *Client) error {
				_, err := client.VerifyWebsite(context.Background(), testWebsiteWorkID, verifyRequest)
				return err
			},
		},
		{
			name: "Website origin with fragment", response: websiteOriginWithFragment,
			call: func(client *Client) error {
				_, err := client.VerifyWebsite(context.Background(), testWebsiteWorkID, verifyRequest)
				return err
			},
		},
		{
			name: "wrong Application ID", response: wrongApplicationID,
			call: func(client *Client) error {
				_, err := client.GetCommerceApplication(context.Background(), testCommerceApplicationID, testMerchantAccountID)
				return err
			},
		},
		{
			name: "zero Application revision", response: zeroApplicationRevision,
			call: func(client *Client) error {
				_, err := client.GetCommerceApplication(context.Background(), testCommerceApplicationID, testMerchantAccountID)
				return err
			},
		},
		{
			name: "wrong created Application Work", response: wrongApplicationWork,
			call: func(client *Client) error {
				_, err := client.CreateCommerceApplication(context.Background(), createApplicationRequest)
				return err
			},
		},
		{
			name: "wrong created Application kind", response: wrongApplicationKind,
			call: func(client *Client) error {
				_, err := client.CreateCommerceApplication(context.Background(), createApplicationRequest)
				return err
			},
		},
		{
			name: "wrong created Application environment", response: wrongApplicationEnvironment,
			call: func(client *Client) error {
				_, err := client.CreateCommerceApplication(context.Background(), createApplicationRequest)
				return err
			},
		},
		{
			name: "wrong created Application Origin", response: wrongApplicationOrigin,
			call: func(client *Client) error {
				_, err := client.CreateCommerceApplication(context.Background(), createApplicationRequest)
				return err
			},
		},
		{
			name: "wrong created Application Return URL", response: wrongApplicationReturnURL,
			call: func(client *Client) error {
				_, err := client.CreateCommerceApplication(context.Background(), createApplicationRequest)
				return err
			},
		},
		{
			name: "Website Widget Product", response: widgetWithProduct,
			call: func(client *Client) error {
				_, err := client.GetCommerceApplication(context.Background(), testCommerceApplicationID, testMerchantAccountID)
				return err
			},
		},
		{
			name: "non-advancing Application revision", response: nonAdvancingApplication,
			call: func(client *Client) error {
				_, err := client.UpdateCommerceApplication(context.Background(), testCommerceApplicationID, UpdateCommerceApplicationRequest{
					MerchantAccountID: testMerchantAccountID,
					ExpectedRevision:  1,
					DisplayName:       &updatedName,
				})
				return err
			},
		},
		{
			name: "non-advancing Application command revision", response: nonAdvancingCommand,
			call: func(client *Client) error {
				_, err := client.CommandCommerceApplication(context.Background(), testCommerceApplicationID, "activate", CommerceApplicationCommand{
					MerchantAccountID: testMerchantAccountID,
					ExpectedRevision:  2,
				})
				return err
			},
		},
		{
			name: "nil Work SDK access list", response: WorkSdkAccessesResponse{},
			call: func(client *Client) error {
				_, err := client.ListWorkSdkAccesses(context.Background(), testMerchantAccountID)
				return err
			},
		},
		{
			name: "invalid Work SDK access list item", response: WorkSdkAccessesResponse{Items: []WorkSdkAccess{zeroAccessVersion}},
			call: func(client *Client) error {
				_, err := client.ListWorkSdkAccesses(context.Background(), testMerchantAccountID)
				return err
			},
		},
		{
			name: "nil Commerce Application list", response: CommerceApplicationsResponse{},
			call: func(client *Client) error {
				_, err := client.ListCommerceApplications(context.Background(), testMerchantAccountID)
				return err
			},
		},
		{
			name: "invalid Commerce Application list item", response: CommerceApplicationsResponse{Items: []CommerceApplication{zeroApplicationRevision}},
			call: func(client *Client) error {
				_, err := client.ListCommerceApplications(context.Background(), testMerchantAccountID)
				return err
			},
		},
	}

	const secretToken = "vme_cli_secret_should_not_leak"
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				switch response := test.response.(type) {
				case rawResponse:
					_, _ = io.WriteString(writer, string(response))
				default:
					_, _ = writer.Write(mustJSON(response))
				}
			}))
			defer server.Close()

			err := test.call(NewClient(server.URL, server.Client(), staticToken(secretToken), ""))
			cliError := output.AsError(err)
			if err == nil || cliError.Subtype != "RESPONSE_INVALID" || cliError.Retryable {
				t.Fatalf("invalid 2xx response was accepted: %#v", cliError)
			}
			encoded, encodeErr := json.Marshal(cliError)
			if encodeErr != nil {
				t.Fatal(encodeErr)
			}
			if bytes.Contains(encoded, []byte(secretToken)) || strings.Contains(err.Error(), secretToken) {
				t.Fatalf("Bearer credential leaked into output: %s", encoded)
			}
		})
	}
}

func assertRequestJSON(t *testing.T, request *http.Request, expected any) {
	t.Helper()
	actual, err := io.ReadAll(request.Body)
	if err != nil {
		t.Error(err)
		return
	}
	if expected == nil {
		if len(bytes.TrimSpace(actual)) != 0 {
			t.Errorf("body = %s, want empty", actual)
		}
		return
	}

	want := mustJSON(expected)
	var actualValue any
	var expectedValue any
	if err := json.Unmarshal(actual, &actualValue); err != nil {
		t.Errorf("body is not JSON: %s", actual)
		return
	}
	if err := json.Unmarshal(want, &expectedValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actualValue, expectedValue) {
		t.Errorf("body = %s, want %s", actual, want)
	}
}

type rawResponse string

func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

func testWebsiteVerification(status string, includeChallenge bool) WebsiteVerification {
	verification := WebsiteVerification{
		ID:            testWebsiteVerificationID,
		WebsiteWorkID: testWebsiteWorkID,
		Version:       2,
		DNSRecordName: "_viceme.example.com",
		Status:        status,
		ExpiresAt:     testTimestamp,
	}
	if includeChallenge {
		challenge := "abcdefghijklmnopqrstuvwxyzABCDEFG1234567890"
		verification.Challenge = &challenge
	}
	if status == "VERIFIED" {
		verifiedAt := testTimestamp
		verification.VerifiedAt = &verifiedAt
	}
	return verification
}

func testWebsiteMerchantWork(ownershipStatus string, revision, verificationVersion int) MerchantWork {
	merchantAccountID := testMerchantAccountID
	work := MerchantWork{
		ID:       testWebsiteWorkID,
		Kind:     "WEBSITE",
		Origin:   "USER_AUTHORED",
		Slug:     "demo-widget",
		Title:    "Demo Widget",
		Status:   "PUBLISHED",
		Revision: revision,
		Owner: WorkOwner{
			Kind:              "MERCHANT",
			MerchantAccountID: &merchantAccountID,
		},
		Skill:          json.RawMessage("null"),
		Service:        json.RawMessage("null"),
		ActiveRevision: json.RawMessage("null"),
		DraftRevision:  json.RawMessage("null"),
		CreatedAt:      testTimestamp,
		UpdatedAt:      testTimestamp,
	}
	work.Website = &WebsiteWork{
		CanonicalOrigin:     "https://example.com",
		DomainASCII:         "example.com",
		OwnershipStatus:     ownershipStatus,
		VerificationVersion: verificationVersion,
	}
	if ownershipStatus == "VERIFIED" {
		verifiedAt := testTimestamp
		work.Website.VerifiedAt = &verifiedAt
	}
	return work
}

func testWorkSdkAccess(workID, status string, configVersion int, features []string) WorkSdkAccess {
	return WorkSdkAccess{
		WorkID:        workID,
		WorkKey:       "wrk_test_access",
		Status:        status,
		ConfigVersion: configVersion,
		Features:      features,
		CreatedAt:     testTimestamp,
		UpdatedAt:     testTimestamp,
	}
}

func testCommerceApplication(kind, environment, status string, revision int, displayName string, origins, returnURLs []string) CommerceApplication {
	applicationOrigins := make([]CommerceApplicationOrigin, len(origins))
	for index, origin := range origins {
		applicationOrigins[index] = CommerceApplicationOrigin{ID: testOriginID, Origin: origin}
	}
	applicationReturnURLs := make([]CommerceApplicationReturnURL, len(returnURLs))
	for index, returnURL := range returnURLs {
		applicationReturnURLs[index] = CommerceApplicationReturnURL{ID: testReturnURLID, URL: returnURL}
	}
	return CommerceApplication{
		ID:                testCommerceApplicationID,
		WorkID:            testWebsiteWorkID,
		MerchantAccountID: testMerchantAccountID,
		PublicClientID:    "vca_123456789012345678901234",
		Kind:              kind,
		Environment:       environment,
		Status:            status,
		DisplayName:       displayName,
		Revision:          revision,
		Origins:           applicationOrigins,
		ReturnURLs:        applicationReturnURLs,
		Products:          []CommerceApplicationProduct{},
		CreatedAt:         testTimestamp,
		UpdatedAt:         testTimestamp,
	}
}
