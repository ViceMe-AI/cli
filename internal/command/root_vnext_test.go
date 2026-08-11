package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cliembed "github.com/ViceMe-AI/cli"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

type failingEntropyReader struct{}

func (failingEntropyReader) Read([]byte) (int, error) {
	return 0, errors.New("entropy unavailable")
}

func TestUUIDFallbackStillProducesAValidVersionFourUUID(t *testing.T) {
	t.Parallel()
	value := uuidFromEntropy(failingEntropyReader{}, time.Unix(1_700_000_000, 42), 123, 7)
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !pattern.MatchString(value) {
		t.Fatalf("fallback returned an invalid UUID: %q", value)
	}
}

func TestBareCommandKeepsMachineOutputAsJSON(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Execute(nil, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN,
	})
	if exit != 0 {
		t.Fatalf("bare command failed: exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("bare command polluted machine output: result=%#v err=%v stdout=%q", result, err, stdout.String())
	}
	data, ok := result["data"].(map[string]any)
	if result["ok"] != true || !ok || data["command"] != "viceme" {
		t.Fatalf("bare command returned an unexpected envelope: %#v", result)
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Fatalf("human help was not isolated on stderr: %q", stderr.String())
	}
}

func TestDoctorIncludesUnauthenticatedNetworkReadiness(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	environment := skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")}
	skills := skillcontent.New(cliembed.EmbeddedSkills())
	reports := skills.InstallSet(officialSkillNames, "agents", environment)
	for _, report := range reports {
		if !report.AllSucceeded {
			t.Fatalf("test Skill install failed: %#v", reports)
		}
	}
	var readinessStatus atomic.Int32
	var leakedAuthorization atomic.Bool
	readinessStatus.Store(http.StatusNoContent)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/health/ready" {
			http.NotFound(writer, request)
			return
		}
		if request.Header.Get("Authorization") != "" {
			leakedAuthorization.Store(true)
		}
		writer.WriteHeader(int(readinessStatus.Load()))
	}))
	defer server.Close()
	store := securestore.NewMemory()
	run := func() (int, map[string]any) {
		t.Helper()
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exit := Execute([]string{"doctor", "--agent", "agents"}, Dependencies{
			Out: &stdout, ErrOut: &stderr, Store: store, Skills: skills,
			Environment: environment, APIBaseURL: server.URL, Region: config.RegionCN,
		})
		var result map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatalf("Doctor did not emit JSON: exit=%d stdout=%q stderr=%q err=%v", exit, stdout.String(), stderr.String(), err)
		}
		return exit, result
	}
	if exit, result := run(); exit != 0 {
		t.Fatalf("healthy Doctor failed: exit=%d result=%#v", exit, result)
	} else {
		data, ok := result["data"].(map[string]any)
		network, networkOK := data["network"].(map[string]any)
		if !ok || !networkOK || network["healthy"] != true {
			t.Fatalf("Doctor omitted network readiness: %#v", result)
		}
	}
	if leakedAuthorization.Load() {
		t.Fatal("Doctor readiness probe leaked a stored credential")
	}
	readinessStatus.Store(http.StatusServiceUnavailable)
	if exit, result := run(); exit == 0 || result["ok"] != false {
		t.Fatalf("Doctor accepted an unavailable API: exit=%d result=%#v", exit, result)
	}
}
