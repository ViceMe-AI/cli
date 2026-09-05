package replicapreview

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultPerformanceGoal = 10 * time.Second
)

type Stage string

const (
	StageInspect  Stage = "INSPECT"
	StageStarting Stage = "STARTING"
	StageReady    Stage = "READY"
)

type ServiceKind string

const (
	ServiceExisting ServiceKind = "EXISTING"
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

	return nil, startError("REPLICA_PREVIEW_URL_REQUIRED", StageInspect, "provide the actual loopback page URL selected and started by your agent", nil)
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

func performance(goal, readyAfter time.Duration) Performance {
	result := Performance{Applicable: readyAfter < goal, Goal: goal, ReadyAfter: readyAfter}
	if !result.Applicable {
		result.ExcludedReason = "SLOW_LOCAL_SERVICE_PROBE"
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

type reusedSession struct{ result Result }

func (session *reusedSession) Result() Result     { return session.result }
func (*reusedSession) Wait(context.Context) error { return nil }
func (*reusedSession) Close() error               { return nil }
