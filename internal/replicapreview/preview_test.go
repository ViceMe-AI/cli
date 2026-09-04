package replicapreview

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestInspectProjectUsesTheDeclaredPackageManagerAndDevScript(t *testing.T) {
	root := t.TempDir()
	writePreviewFixture(t, root)

	plan, err := inspectProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if plan.command != "pnpm" || strings.Join(plan.arguments, " ") != "run dev" {
		t.Fatalf("unexpected project plan: %#v", plan)
	}
	if !plan.dependenciesInstalled {
		t.Fatal("existing dependencies were not detected")
	}
}

func TestSlowStartupIsReportedOutsideTheBaselineWithoutHidingItsDuration(t *testing.T) {
	goal := 10 * time.Second
	readyAfter := goal + time.Millisecond
	result := performance(goal, readyAfter)

	if result.Applicable || result.ExcludedReason != "FIRST_BUILD_OR_SLOW_PROJECT_STARTUP" {
		t.Fatalf("slow startup was not reported separately: %#v", result)
	}
	if result.Goal != goal || result.ReadyAfter != readyAfter {
		t.Fatalf("slow startup timing was hidden: %#v", result)
	}
}

func TestStartReusesAnExplicitRunningLoopbackService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "ready")
	}))
	defer server.Close()

	session, err := Start(context.Background(), Options{ExistingURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result := session.Result()
	if !result.Reused || result.StartedByCLI || result.TargetURL != server.URL+"/" {
		t.Fatalf("unexpected reuse result: %#v", result)
	}
	if err := session.Wait(context.Background()); err != nil {
		t.Fatalf("reused service should not keep the CLI alive: %v", err)
	}
}

func TestStartRejectsNonLoopbackReuseTargets(t *testing.T) {
	_, err := Start(context.Background(), Options{ExistingURL: "https://example.com"})
	assertStartError(t, err, "REPLICA_PREVIEW_URL_INVALID", StageInspect)
}

func TestMissingDependenciesAreReportedAsColdInstallOutsideThePerformanceTarget(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{
  "packageManager": "pnpm@9.15.0",
  "scripts": {"dev": "vite"}
}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var stages []Stage
	_, err := Start(context.Background(), Options{
		ProjectPath: root,
		Report: func(event Event) {
			stages = append(stages, event.Stage)
		},
	})
	assertStartError(t, err, "REPLICA_PREVIEW_DEPENDENCIES_MISSING", StageColdInstall)
	if !containsStage(stages, StageColdInstall) {
		t.Fatalf("cold-install stage was not reported: %#v", stages)
	}
}

func TestOwnedDevServerIsCleanedAndBaselineP95StaysUnderTenSeconds(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the process-tree fixture uses a POSIX launcher")
	}
	root := t.TempDir()
	writePreviewFixture(t, root)
	packageJSONPath := filepath.Join(root, "package.json")
	before, err := os.ReadFile(packageJSONPath)
	if err != nil {
		t.Fatal(err)
	}

	bin := t.TempDir()
	launcher := filepath.Join(bin, "pnpm")
	script := "#!/bin/sh\nexec \"$VICEME_PREVIEW_HELPER_BINARY\" -test.run '^TestReplicaPreviewHelperProcess$'\n"
	if err := os.WriteFile(launcher, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("VICEME_PREVIEW_HELPER_BINARY", os.Args[0])
	t.Setenv("VICEME_PREVIEW_HELPER", "1")
	marker := filepath.Join(t.TempDir(), "stopped")
	t.Setenv("VICEME_PREVIEW_HELPER_MARKER", marker)
	t.Setenv("VICEME_ACCESS_TOKEN", "must-not-reach-project-dev-script")

	const samples = 20
	durations := make([]time.Duration, 0, samples)
	for sample := 0; sample < samples; sample++ {
		ctx, cancel := context.WithCancel(context.Background())
		session, startErr := Start(ctx, Options{
			ProjectPath:     root,
			StartupTimeout:  10 * time.Second,
			PerformanceGoal: 10 * time.Second,
			ErrOut:          io.Discard,
		})
		if startErr != nil {
			cancel()
			t.Fatalf("sample %d failed: %v", sample, startErr)
		}
		result := session.Result()
		if result.Reused || !result.StartedByCLI || result.ServiceKind != ServiceDevScript {
			t.Fatalf("sample %d returned unexpected ownership: %#v", sample, result)
		}
		if !result.Performance.Applicable {
			t.Fatalf("sample %d incorrectly excluded an installed-dependency startup", sample)
		}
		durations = append(durations, result.Performance.ReadyAfter)
		cancel()
		if waitErr := session.Wait(ctx); waitErr != nil {
			t.Fatalf("sample %d did not stop cleanly: %v", sample, waitErr)
		}
	}

	sort.Slice(durations, func(left, right int) bool { return durations[left] < durations[right] })
	p95 := durations[(samples*95+99)/100-1]
	t.Logf("installed-dependency preview startup P95 across %d samples: %s", samples, p95)
	if p95 >= 10*time.Second {
		t.Fatalf("baseline preview P95 exceeded ten seconds: %s (%#v)", p95, durations)
	}

	after, err := os.ReadFile(packageJSONPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("preview modified project source")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		data, readErr := os.ReadFile(marker)
		if readErr == nil && strings.Count(string(data), "stopped\n") == samples {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("owned preview processes were not all cleaned: data=%q err=%v", data, readErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestOwnedDevServerIsCleanedWhenStartupTimesOut(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the process-tree fixture uses a POSIX launcher")
	}
	root := t.TempDir()
	writePreviewFixture(t, root)
	bin := t.TempDir()
	launcher := filepath.Join(bin, "pnpm")
	script := "#!/bin/sh\nexec \"$VICEME_PREVIEW_HELPER_BINARY\" -test.run '^TestReplicaPreviewHelperProcess$'\n"
	if err := os.WriteFile(launcher, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("VICEME_PREVIEW_HELPER_BINARY", os.Args[0])
	t.Setenv("VICEME_PREVIEW_HELPER", "1")
	t.Setenv("VICEME_PREVIEW_HELPER_NO_URL", "1")
	marker := filepath.Join(t.TempDir(), "stopped")
	t.Setenv("VICEME_PREVIEW_HELPER_MARKER", marker)

	var stages []Stage
	_, err := Start(context.Background(), Options{
		ProjectPath:     root,
		StartupTimeout:  500 * time.Millisecond,
		PerformanceGoal: 50 * time.Millisecond,
		ErrOut:          io.Discard,
		Report: func(event Event) {
			stages = append(stages, event.Stage)
		},
	})
	assertStartError(t, err, "REPLICA_PREVIEW_START_TIMEOUT", StageSlowStartup)
	if !containsStage(stages, StageSlowStartup) {
		t.Fatalf("slow-startup stage was not reported: %#v", stages)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		data, readErr := os.ReadFile(marker)
		if readErr == nil && string(data) == "stopped\n" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("failed startup left its owned process running: data=%q err=%v", data, readErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestReplicaPreviewHelperProcess(t *testing.T) {
	if os.Getenv("VICEME_PREVIEW_HELPER") != "1" {
		return
	}
	if os.Getenv("VICEME_ACCESS_TOKEN") != "" {
		fmt.Fprintln(os.Stderr, "preview leaked the CLI access token to the project")
		os.Exit(9)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(10)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "fixture")
	})}
	go func() { _ = server.Serve(listener) }()
	if os.Getenv("VICEME_PREVIEW_HELPER_NO_URL") != "1" {
		fmt.Printf("Local: http://%s/\n", listener.Addr().String())
	}

	stopped := make(chan os.Signal, 1)
	signal.Notify(stopped, os.Interrupt, syscall.SIGTERM)
	<-stopped
	_ = server.Close()
	marker, err := os.OpenFile(os.Getenv("VICEME_PREVIEW_HELPER_MARKER"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err == nil {
		_, _ = io.WriteString(marker, "stopped\n")
		_ = marker.Close()
	}
}

func writePreviewFixture(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{
  "packageManager": "pnpm@9.15.0",
  "scripts": {"dev": "vite --host 127.0.0.1"}
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "node_modules"), 0o700); err != nil {
		t.Fatal(err)
	}
}

func assertStartError(t *testing.T, err error, code string, stage Stage) {
	t.Helper()
	previewErr, ok := err.(*StartError)
	if !ok || previewErr.Code != code || previewErr.Stage != stage {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func containsStage(stages []Stage, expected Stage) bool {
	for _, stage := range stages {
		if stage == expected {
			return true
		}
	}
	return false
}
