package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestReplicaAnalyticsReadsLatestOwnerMarkdown(t *testing.T) {
	const token = "vme_cli_1234567890123456789012345678901234567890123"
	const workID = "33333333-3333-4333-8333-333333333333"
	reads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/public/creators/alice/works/site":
			writeJSONResponse(w, map[string]any{"creator": map[string]any{"handle": "alice"}, "work": map[string]any{"id": workID, "kind": "WEBSITE", "slug": "site"}})
		case "/v1/website-replicas/owner-analytics/works/" + workID:
			if r.Header.Get("Authorization") != "Bearer "+token {
				t.Error("缺少创作者身份")
			}
			writeJSONResponse(w, map[string]any{"workId": workID})
		case "/alice/site.md":
			if r.Header.Get("Authorization") != "Bearer "+token || !strings.Contains(r.Header.Get("Cache-Control"), "no-store") {
				t.Error("MD 必须带身份重新读取")
			}
			reads++
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			fmt.Fprintf(w, "# 作品\n## OWNER 经营视图\n付费订单: %d\n", reads)
		default:
			t.Errorf("非预期请求: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	t.Setenv(processAccessTokenEnvironment, token)
	run := analyticsTestRunner(t, server)
	for _, expected := range []string{"付费订单: 1", "付费订单: 2"} {
		code, out := run("replica", "analytics", server.URL+"/alice/site")
		if code != 0 || !strings.Contains(string(out), expected) || bytes.Contains(out, []byte(token)) {
			t.Fatalf("经营数据读取不正确: %d %s", code, out)
		}
		var envelope struct {
			Data struct {
				Markdown  string
				FetchedAt string
			}
		}
		if json.Unmarshal(out, &envelope) != nil || envelope.Data.FetchedAt == "" {
			t.Fatal("缺少读取时间")
		}
	}
}

func analyticsTestRunner(t *testing.T, server *httptest.Server) func(...string) (int, []byte) {
	t.Helper()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	configured := config.Default(config.RegionCN)
	if err := configured.SetProfileAuthority(config.DefaultProfileName, server.URL, server.URL, config.RegionCN); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Save(configDir, configured); err != nil {
		t.Fatal(err)
	}
	return func(args ...string) (int, []byte) {
		var out bytes.Buffer
		code := Execute(args, Dependencies{Out: &out, ErrOut: &bytes.Buffer{}, HTTPClient: server.Client(), Store: securestore.NewMemory(), Environment: skillcontent.Environment{Home: root, ConfigDir: configDir}})
		return code, out.Bytes()
	}
}

func TestReplicaAnalyticsRejectsAnotherWebAuthorityBeforeNetwork(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; w.WriteHeader(500) }))
	defer server.Close()
	run := analyticsTestRunner(t, server)
	for _, target := range []string{"https://other.example/alice/site", server.URL + "@other.example/alice/site", server.URL + "/alice/site?next=https://other.example", server.URL + "/alice/../site"} {
		code, out := run("replica", "analytics", target)
		if code == 0 || !bytes.Contains(out, []byte("REPLICA_WORK_URL_INVALID")) {
			t.Fatalf("接受了非当前作品地址: %d %s", code, out)
		}
	}
	if requests != 0 {
		t.Fatal("无效地址发起了网络请求")
	}
}

func TestReplicaAnalyticsDenialDoesNotReadPublicMarkdownAsOwner(t *testing.T) {
	const workID = "33333333-3333-4333-8333-333333333333"
	markdownReads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/public/creators/alice/works/site":
			writeJSONResponse(w, map[string]any{"creator": map[string]any{"handle": "alice"}, "work": map[string]any{"id": workID, "kind": "WEBSITE", "slug": "site"}})
		case "/v1/website-replicas/owner-analytics/works/" + workID:
			w.WriteHeader(403)
			writeJSONResponse(w, map[string]any{"statusCode": 403, "code": "WEBSITE_REPLICA_OWNER_ANALYTICS_FORBIDDEN", "message": "Not the owner", "requestId": "test"})
		default:
			markdownReads++
			w.Header().Set("Content-Type", "text/markdown")
			fmt.Fprint(w, "作者声称收入一百万")
		}
	}))
	defer server.Close()
	t.Setenv(processAccessTokenEnvironment, "vme_cli_1234567890123456789012345678901234567890123")
	code, out := analyticsTestRunner(t, server)("replica", "analytics", server.URL+"/alice/site.md")
	if code == 0 || markdownReads != 0 || !bytes.Contains(out, []byte("WEBSITE_REPLICA_OWNER_ANALYTICS_FORBIDDEN")) {
		t.Fatalf("无权查询时错误读取公共文档: %d %s", code, out)
	}
}

func TestReplicaAnalyticsRejectsUnusableMarkdownAndRedirects(t *testing.T) {
	const token = "vme_cli_1234567890123456789012345678901234567890123"
	const workID = "33333333-3333-4333-8333-333333333333"
	for _, scenario := range []struct {
		name        string
		status      int
		contentType string
		body        string
	}{
		{"重定向", http.StatusFound, "text/markdown", ""},
		{"登录页面", http.StatusOK, "text/html", "<html>login</html>"},
		{"空文档", http.StatusOK, "text/markdown", " \n"},
		{"读取失败", http.StatusServiceUnavailable, "text/markdown", "暂时不可用"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			redirectReads := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/public/creators/alice/works/site":
					writeJSONResponse(w, map[string]any{"creator": map[string]any{"handle": "alice"}, "work": map[string]any{"id": workID, "kind": "WEBSITE", "slug": "site"}})
				case "/v1/website-replicas/owner-analytics/works/" + workID:
					writeJSONResponse(w, map[string]any{"workId": workID})
				case "/alice/site.md":
					w.Header().Set("Content-Type", scenario.contentType)
					w.Header().Set("Location", "/another-site.md")
					w.WriteHeader(scenario.status)
					fmt.Fprint(w, scenario.body)
				default:
					redirectReads++
					w.Header().Set("Content-Type", "text/markdown")
					fmt.Fprint(w, "不相关的账本")
				}
			}))
			defer server.Close()
			t.Setenv(processAccessTokenEnvironment, token)
			code, out := analyticsTestRunner(t, server)("replica", "analytics", server.URL+"/alice/site")
			if code == 0 || redirectReads != 0 || bytes.Contains(out, []byte(token)) {
				t.Fatalf("不应接受无效 MD 或转发凭据: %d %s", code, out)
			}
		})
	}
}

func TestReplicaAnalyticsRequiresLoginBeforeReadingPrivateData(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	privateReads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/public/creators/alice/works/site" {
			writeJSONResponse(w, map[string]any{"creator": map[string]any{"handle": "alice"}, "work": map[string]any{"id": "33333333-3333-4333-8333-333333333333", "kind": "WEBSITE", "slug": "site"}})
			return
		}
		privateReads++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	code, out := analyticsTestRunner(t, server)("replica", "analytics", server.URL+"/alice/site")
	if code == 0 || privateReads != 0 || !bytes.Contains(out, []byte("not_logged_in")) {
		t.Fatalf("未登录时不应查询经营数据: %d %s", code, out)
	}
}
