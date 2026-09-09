package command

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/commerceartifact"
	"github.com/ViceMe-AI/cli/internal/output"
)

const expectedHostedDevCommerceKey = "MCowBQYDK2VwAyEAi9-dMC8FRYitvErpDTteooMjkEhYrDwUv-r11GiIQXo"

func TestCommerceRuntimePinsHostedDevelopmentKeyToOrigin(t *testing.T) {
	original := buildinfo.CommerceSkillTrustKeys
	buildinfo.CommerceSkillTrustKeys = ""
	t.Cleanup(func() { buildinfo.CommerceSkillTrustKeys = original })
	if _, err := commerceartifact.ParseTrustRing("v1:" + expectedHostedDevCommerceKey); err != nil {
		t.Fatalf("hosted development pin is not an Ed25519 SPKI key: %v", err)
	}
	for _, test := range []struct {
		name, origin, keyID string
		trusted             bool
	}{
		{"hosted dev", "https://dev.viceme.cn/api", "v1", true},
		{"canonical dev origin", "https://DEV.VICEME.CN:443/api/", "v1", true},
		{"production CN", "https://api.viceme.cn", "v1", false},
		{"production gateway", "https://viceme.cn/api", "v1", false},
		{"production global", "https://api.viceme.ai", "v1", false},
		{"other deployment", "https://private.example/api", "v1", false},
		{"lookalike suffix", "https://dev.viceme.cn.example/api", "v1", false},
		{"lookalike subdomain", "https://other.dev.viceme.cn/api", "v1", false},
		{"different port", "https://dev.viceme.cn:8443/api", "v1", false},
		{"insecure dev", "http://dev.viceme.cn/api", "v1", false},
		{"unknown dev key", "https://dev.viceme.cn/api", "v2", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport := &unexpectedCommerceTrustNetwork{}
			runtime := &Runtime{apiBaseURL: test.origin, deps: Dependencies{HTTPClient: &http.Client{Transport: transport}}}
			key, err := runtime.resolveCommerceTrustKey(context.Background(), test.keyID)
			if transport.calls != 0 {
				t.Fatal("remote origin attempted dynamic trust-key discovery")
			}
			if test.trusted {
				if err != nil || key != expectedHostedDevCommerceKey {
					t.Fatalf("hosted development key rejected: key=%q err=%v", key, err)
				}
			} else if key != "" || err == nil || output.AsError(err).Subtype != "COMMERCE_SKILL_SIGNING_KEY_UNTRUSTED" {
				t.Fatalf("untrusted origin/key accepted: key=%q err=%v", key, err)
			}
		})
	}
}

func TestHostedDevelopmentReplicaStillVerifiesSignatureAndEmbeddedKey(t *testing.T) {
	const replicaID = "33333333-3333-4333-8333-333333333333"
	const versionID = "55555555-5555-4555-8555-555555555555"
	signer := newReplicaTestSigner(t, "v1")
	license := signedReplicaTestLicense(t, signer, replicaID, versionID, 1, "VMO-TEST", "test-digest")
	runtime := &Runtime{apiBaseURL: "https://dev.viceme.cn/api"}
	for _, test := range []struct{ name, embeddedKey, code string }{
		{"forged embedded key", signer.publicKey, "REPLICA_LICENSE_SIGNING_KEY_UNTRUSTED"},
		{"forged signature", expectedHostedDevCommerceKey, "REPLICA_LICENSE_SIGNATURE_INVALID"},
	} {
		t.Run(test.name, func(t *testing.T) {
			license.SigningPublicKey = test.embeddedKey
			encoded, err := json.Marshal(license)
			if err != nil {
				t.Fatal(err)
			}
			err = verifyReplicaLicense(context.Background(), runtime, api.WebsiteReplicaDownload{
				ReplicaID: replicaID, VersionID: versionID, Version: 1, ArtifactDigest: "test-digest", License: encoded,
			}, "VMO-TEST")
			if err == nil || output.AsError(err).Subtype != test.code {
				t.Fatalf("expected %s, got %v", test.code, err)
			}
		})
	}
}

func TestCommerceRuntimeSeparatesSameKeyIDAcrossOrigins(t *testing.T) {
	const productionKey = "MCowBQYDK2VwAyEA11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo"
	original := buildinfo.CommerceSkillTrustKeys
	buildinfo.CommerceSkillTrustKeys = "v1:" + productionKey
	t.Cleanup(func() { buildinfo.CommerceSkillTrustKeys = original })
	for origin, expected := range map[string]string{
		"https://dev.viceme.cn/api": expectedHostedDevCommerceKey,
		"https://api.viceme.cn":     productionKey,
		"https://api.viceme.ai":     productionKey,
	} {
		runtime := &Runtime{apiBaseURL: origin}
		key, err := runtime.resolveCommerceTrustKey(context.Background(), "v1")
		if err != nil || key != expected {
			t.Fatalf("key ID crossed origin boundary for %s: key=%q err=%v", origin, key, err)
		}
	}
}

func TestCommerceRuntimeRetainsLoopbackTrustDiscovery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/commerce-skill-trust-keys/local-dev" {
			t.Errorf("unexpected trust endpoint: %s", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSONResponse(writer, map[string]any{"keyId": "local-dev", "algorithm": "Ed25519", "publicKey": expectedHostedDevCommerceKey})
	}))
	defer server.Close()
	runtime := &Runtime{apiBaseURL: server.URL, deps: Dependencies{HTTPClient: server.Client()}}
	key, err := runtime.resolveCommerceTrustKey(context.Background(), "local-dev")
	if err != nil || key != expectedHostedDevCommerceKey {
		t.Fatalf("loopback development trust failed: %v", err)
	}
}

type unexpectedCommerceTrustNetwork struct{ calls int }

func (transport *unexpectedCommerceTrustNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	transport.calls++
	return nil, errors.New("unexpected trust-key network request")
}
