package replicapreview

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	defaultPerformanceGoal = 10 * time.Second
	defaultStartupTimeout  = 2 * time.Minute
)

type Stage string

const (
	StageInspect     Stage = "INSPECT"
	StageColdInstall Stage = "COLD_INSTALL"
	StageStarting    Stage = "STARTING"
	StageSlowStartup Stage = "SLOW_STARTUP"
	StageReady       Stage = "READY"
)

type ServiceKind string

const (
	ServiceExisting  ServiceKind = "EXISTING"
	ServiceDevScript ServiceKind = "DEV_SCRIPT"
)

type Event struct {
	Stage   Stage
	Message string
}

type Performance struct {
	Applicable     bool
	Goal           time.Duration
	ReadyAfter     time.Duration
	ExcludedReason string
}

type Result struct {
	TargetURL    string
	Reused       bool
	StartedByCLI bool
	ServiceKind  ServiceKind
	Performance  Performance
}

type Options struct {
	ExistingURL     string
	ProjectPath     string
	StartupTimeout  time.Duration
	PerformanceGoal time.Duration
	Report          func(Event)
	ErrOut          io.Writer
}

type Running interface {
	Result() Result
	Wait(context.Context) error
	Close() error
}

type StartError struct {
	Code    string
	Stage   Stage
	Message string
	Cause   error
}

func (err *StartError) Error() string {
	if err.Message != "" {
		return err.Message
	}
	return err.Code
}

func (err *StartError) Unwrap() error { return err.Cause }

type projectPlan struct {
	root                  string
	command               string
	arguments             []string
	dependenciesInstalled bool
}

type packageManifest struct {
	PackageManager string            `json:"packageManager"`
	Scripts        map[string]string `json:"scripts"`
}

var (
	previewURLPattern = regexp.MustCompile(`https?://[^\s<>"']+`)
	ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
)

func Start(ctx context.Context, options Options) (Running, error) {
	startedAt := time.Now()
	goal := options.PerformanceGoal
	if goal <= 0 {
		goal = defaultPerformanceGoal
	}
	if options.ExistingURL != "" && options.ProjectPath != "" {
		return nil, startError("REPLICA_PREVIEW_OPTIONS_INVALID", StageInspect, "--url and --path cannot be used together", nil)
	}

	report(options, StageInspect, "Inspecting the local preview target")
	if options.ExistingURL != "" {
		target, err := normalizeLoopbackURL(options.ExistingURL)
		if err != nil {
			return nil, startError("REPLICA_PREVIEW_URL_INVALID", StageInspect, "--url must be an HTTP or HTTPS loopback address", err)
		}
		if !probeService(ctx, target) {
			return nil, startError("REPLICA_PREVIEW_URL_UNAVAILABLE", StageInspect, "the requested local service did not respond", nil)
		}
		readyAfter := time.Since(startedAt)
		report(options, StageReady, "Reusing the running local service")
		return &reusedSession{result: Result{
			TargetURL:    target,
			Reused:       true,
			StartedByCLI: false,
			ServiceKind:  ServiceExisting,
			Performance:  performance(goal, readyAfter),
		}}, nil
	}

	root := options.ProjectPath
	if root == "" {
		root = "."
	}
	plan, err := inspectProject(root)
	if err != nil {
		return nil, startError("REPLICA_PREVIEW_PROJECT_INVALID", StageInspect, "the project does not expose a supported dev script", err)
	}
	if !plan.dependenciesInstalled {
		report(options, StageColdInstall, "Project dependencies are not installed; cold installation is outside the preview performance target")
		return nil, startError(
			"REPLICA_PREVIEW_DEPENDENCIES_MISSING",
			StageColdInstall,
			"project dependencies are missing; install them with the declared package manager before previewing",
			nil,
		)
	}
	if _, err := exec.LookPath(plan.command); err != nil {
		return nil, startError("REPLICA_PREVIEW_PACKAGE_MANAGER_MISSING", StageInspect, "the project's declared package manager is not available", err)
	}

	command := exec.Command(plan.command, plan.arguments...)
	command.Dir = plan.root
	command.Env = previewEnvironment(os.Environ())
	configureProcess(command)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, startError("REPLICA_PREVIEW_START_FAILED", StageStarting, "could not capture the project dev script output", err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return nil, startError("REPLICA_PREVIEW_START_FAILED", StageStarting, "could not capture the project dev script output", err)
	}

	report(options, StageStarting, fmt.Sprintf("Starting the existing dev script with %s", plan.command))
	if err := command.Start(); err != nil {
		return nil, startError("REPLICA_PREVIEW_START_FAILED", StageStarting, "could not start the project dev script", err)
	}
	session := newOwnedSession(command)
	go session.watchContext(ctx)

	addresses := make(chan string, 8)
	writer := &lockedWriter{writer: options.ErrOut}
	go relayOutput(stdout, writer, addresses)
	go relayOutput(stderr, writer, addresses)

	timeout := options.StartupTimeout
	if timeout <= 0 {
		timeout = defaultStartupTimeout
	}
	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()
	slowTimer := time.NewTimer(goal)
	defer slowTimer.Stop()
	slow := slowTimer.C
	probeTicker := time.NewTicker(50 * time.Millisecond)
	defer probeTicker.Stop()

	var candidate string
	for {
		select {
		case raw := <-addresses:
			normalized, normalizeErr := normalizeLoopbackURL(raw)
			if normalizeErr == nil {
				candidate = normalized
			}
		case <-probeTicker.C:
			if candidate == "" || !probeService(ctx, candidate) {
				continue
			}
			readyAfter := time.Since(startedAt)
			session.result = Result{
				TargetURL:    candidate,
				Reused:       false,
				StartedByCLI: true,
				ServiceKind:  ServiceDevScript,
				Performance:  performance(goal, readyAfter),
			}
			report(options, StageReady, "Local project preview is ready")
			return session, nil
		case <-slow:
			report(options, StageSlowStartup, "The project is still building or starting; this wait is reported outside the ten-second baseline")
			slow = nil
		case <-session.done:
			message := "project dev script exited before preview became ready"
			failure := startError("REPLICA_PREVIEW_START_FAILED", StageStarting, message, session.rawWaitError())
			return nil, stopAfterStartFailure(session, failure)
		case <-timeoutTimer.C:
			failure := startError("REPLICA_PREVIEW_START_TIMEOUT", StageSlowStartup, "project dev script did not expose a local preview URL before the startup timeout", nil)
			return nil, stopAfterStartFailure(session, failure)
		case <-ctx.Done():
			failure := startError("REPLICA_PREVIEW_CANCELLED", StageStarting, "local preview startup was cancelled", ctx.Err())
			return nil, stopAfterStartFailure(session, failure)
		}
	}
}

func inspectProject(root string) (projectPlan, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return projectPlan{}, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return projectPlan{}, err
	}
	if !info.IsDir() {
		return projectPlan{}, errors.New("project path is not a directory")
	}
	manifestData, err := os.ReadFile(filepath.Join(absolute, "package.json"))
	if err != nil {
		return projectPlan{}, fmt.Errorf("read package.json: %w", err)
	}
	var manifest packageManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return projectPlan{}, fmt.Errorf("decode package.json: %w", err)
	}
	if strings.TrimSpace(manifest.Scripts["dev"]) == "" {
		return projectPlan{}, errors.New("package.json does not declare scripts.dev")
	}
	manager, err := detectPackageManager(absolute, manifest.PackageManager)
	if err != nil {
		return projectPlan{}, err
	}
	dependencies, err := os.Stat(filepath.Join(absolute, "node_modules"))
	installed := err == nil && dependencies.IsDir()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return projectPlan{}, fmt.Errorf("inspect node_modules: %w", err)
	}
	return projectPlan{
		root: absolute, command: manager,
		arguments: []string{"run", "dev"}, dependenciesInstalled: installed,
	}, nil
}

func detectPackageManager(root, declared string) (string, error) {
	if declared != "" {
		manager := strings.TrimSpace(strings.SplitN(declared, "@", 2)[0])
		switch manager {
		case "npm", "pnpm", "yarn", "bun":
			return manager, nil
		default:
			return "", fmt.Errorf("unsupported package manager %q", manager)
		}
	}
	for _, candidate := range []struct {
		filename string
		manager  string
	}{
		{"pnpm-lock.yaml", "pnpm"},
		{"yarn.lock", "yarn"},
		{"bun.lock", "bun"},
		{"bun.lockb", "bun"},
		{"package-lock.json", "npm"},
		{"npm-shrinkwrap.json", "npm"},
	} {
		if _, err := os.Stat(filepath.Join(root, candidate.filename)); err == nil {
			return candidate.manager, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return "npm", nil
}

func normalizeLoopbackURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("preview URL must be an uncredentialed HTTP(S) origin without a fragment")
	}
	hostname := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	address := net.ParseIP(hostname)
	if hostname != "localhost" && (address == nil || !address.IsLoopback()) {
		return "", errors.New("preview URL host is not loopback")
	}
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String(), nil
}

func probeService(parent context.Context, target string) bool {
	ctx, cancel := context.WithTimeout(parent, 500*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
	if err != nil {
		return false
	}
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:             nil,
			DialContext:       (&net.Dialer{Timeout: 300 * time.Millisecond}).DialContext,
			DisableKeepAlives: true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       500 * time.Millisecond,
	}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	_ = response.Body.Close()
	return true
}

func previewEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment))
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(name, "VICEME_ACCESS_TOKEN") {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func relayOutput(reader io.Reader, output io.Writer, addresses chan<- string) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if output != nil {
			_, _ = fmt.Fprintln(output, line)
		}
		plainLine := ansiEscapePattern.ReplaceAllString(line, "")
		for _, candidate := range previewURLPattern.FindAllString(plainLine, -1) {
			candidate = strings.TrimRight(candidate, ".,;)]}")
			select {
			case addresses <- candidate:
			default:
			}
		}
	}
}

func performance(goal, readyAfter time.Duration) Performance {
	result := Performance{Applicable: readyAfter < goal, Goal: goal, ReadyAfter: readyAfter}
	if !result.Applicable {
		result.ExcludedReason = "FIRST_BUILD_OR_SLOW_PROJECT_STARTUP"
	}
	return result
}

func report(options Options, stage Stage, message string) {
	if options.Report != nil {
		options.Report(Event{Stage: stage, Message: message})
	}
}

func startError(code string, stage Stage, message string, cause error) *StartError {
	return &StartError{Code: code, Stage: stage, Message: message, Cause: cause}
}

func stopAfterStartFailure(session *ownedSession, failure *StartError) error {
	if err := session.Close(); err != nil {
		return startError(
			"REPLICA_PREVIEW_CLEANUP_FAILED",
			failure.Stage,
			"the CLI-owned dev server could not be stopped after preview startup failed",
			errors.Join(failure, err),
		)
	}
	return failure
}

type reusedSession struct{ result Result }

func (session *reusedSession) Result() Result     { return session.result }
func (*reusedSession) Wait(context.Context) error { return nil }
func (*reusedSession) Close() error               { return nil }

type ownedSession struct {
	command  *exec.Cmd
	done     chan struct{}
	result   Result
	stopOnce sync.Once

	mu       sync.Mutex
	waitErr  error
	stopping bool
	stopErr  error
}

func newOwnedSession(command *exec.Cmd) *ownedSession {
	session := &ownedSession{command: command, done: make(chan struct{})}
	go func() {
		err := command.Wait()
		session.mu.Lock()
		session.waitErr = err
		session.mu.Unlock()
		close(session.done)
	}()
	return session
}

func (session *ownedSession) Result() Result { return session.result }

func (session *ownedSession) Wait(ctx context.Context) error {
	if ctx.Err() != nil {
		if err := session.Close(); err != nil {
			return startError("REPLICA_PREVIEW_CLEANUP_FAILED", StageStarting, "the CLI-owned dev server could not be stopped", err)
		}
		return nil
	}
	select {
	case <-session.done:
		if ctx.Err() != nil {
			if err := session.Close(); err != nil {
				return startError("REPLICA_PREVIEW_CLEANUP_FAILED", StageStarting, "the CLI-owned dev server could not be stopped", err)
			}
			return nil
		}
		return session.terminalError()
	case <-ctx.Done():
		if err := session.Close(); err != nil {
			return startError("REPLICA_PREVIEW_CLEANUP_FAILED", StageStarting, "the CLI-owned dev server could not be stopped", err)
		}
		return nil
	}
}

func (session *ownedSession) Close() error {
	session.stopOnce.Do(func() {
		session.mu.Lock()
		session.stopping = true
		session.mu.Unlock()
		terminateErr := terminateProcessTree(session.command)
		stopped, gracefulWaitErr := session.waitForProcessTreeStop(2 * time.Second)
		if stopped {
			return
		}
		killErr := killProcessTree(session.command)
		stopped, forceWaitErr := session.waitForProcessTreeStop(2 * time.Second)
		if stopped {
			return
		}
		session.mu.Lock()
		session.stopErr = errors.Join(
			errors.New("process tree did not stop after forced termination"),
			terminateErr,
			gracefulWaitErr,
			killErr,
			forceWaitErr,
		)
		session.mu.Unlock()
	})
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.stopErr
}

func (session *ownedSession) waitForProcessTreeStop(timeout time.Duration) (bool, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		parentDone := false
		select {
		case <-session.done:
			parentDone = true
		default:
		}
		running, err := processTreeRunning(session.command, parentDone)
		if err != nil {
			return false, err
		}
		if parentDone && !running {
			return true, nil
		}
		select {
		case <-timer.C:
			return false, nil
		case <-ticker.C:
		}
	}
}

func (session *ownedSession) watchContext(ctx context.Context) {
	select {
	case <-ctx.Done():
		_ = session.Close()
	case <-session.done:
	}
}

func (session *ownedSession) rawWaitError() error {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.waitErr
}

func (session *ownedSession) terminalError() error {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.stopping {
		return nil
	}
	return startError("REPLICA_PREVIEW_PROCESS_EXITED", StageStarting, "project dev script exited while the local preview was open", session.waitErr)
}

type lockedWriter struct {
	mu     sync.Mutex
	writer io.Writer
}

func (writer *lockedWriter) Write(data []byte) (int, error) {
	if writer.writer == nil {
		return len(data), nil
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.writer.Write(data)
}
