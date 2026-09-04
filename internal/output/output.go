package output

import (
	"encoding/json"
	"errors"
	"io"
)

const (
	ExitValidation     = 2
	ExitAuthentication = 3
	ExitNetwork        = 4
	ExitInternal       = 5
	ExitPolicy         = 6
	ExitConfirmation   = 10
)

type Meta struct {
	// ExecutingCLIVersion identifies the process that emitted this envelope. A
	// self-update reports the installed target separately in its business data.
	ExecutingCLIVersion string          `json:"executingCliVersion,omitempty"`
	RequestID           string          `json:"requestId,omitempty"`
	WaitTimedOut        *bool           `json:"waitTimedOut,omitempty"`
	AutoUpdate          *AutoUpdateMeta `json:"autoUpdate,omitempty"`
}

// AutoUpdateMeta records the complete generation change that happened before
// the command was re-executed. It is emitted by the new CLI process, never by
// the process that performed the replacement.
type AutoUpdateMeta struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Status string `json:"status"`
}

type Error struct {
	Code          int    `json:"-"`
	Type          string `json:"type"`
	Subtype       string `json:"code"`
	Message       string `json:"message"`
	Retryable     bool   `json:"retryable"`
	Hint          string `json:"hint,omitempty"`
	PublicationID string `json:"publicationId,omitempty"`
	ConsoleURL    string `json:"consoleUrl,omitempty"`
	Details       any    `json:"details,omitempty"`
	RequestID     string `json:"-"`
	Cause         error  `json:"-"`
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Cause }

func (e *Error) WithCause(cause error) *Error {
	e.Cause = cause
	return e
}

func (e *Error) WithHint(hint string) *Error {
	e.Hint = hint
	return e
}

func (e *Error) WithDetails(details any) *Error {
	e.Details = details
	return e
}

func NewError(code int, typ, subtype, message string) *Error {
	return &Error{Code: code, Type: typ, Subtype: subtype, Message: message}
}

func Validation(subtype, message string) *Error {
	return NewError(ExitValidation, "validation", subtype, message)
}

func Authentication(subtype, message string) *Error {
	return NewError(ExitAuthentication, "authentication", subtype, message)
}

func Authorization(subtype, message string) *Error {
	return NewError(ExitAuthentication, "authorization", subtype, message)
}

func Network(subtype, message string, cause error) *Error {
	err := NewError(ExitNetwork, "network", subtype, message)
	err.Retryable = true
	err.Cause = cause
	return err
}

func Internal(subtype, message string, cause error) *Error {
	err := NewError(ExitInternal, "internal", subtype, message)
	err.Cause = cause
	return err
}

func Policy(subtype, message string) *Error {
	return NewError(ExitPolicy, "policy", subtype, message)
}

func Confirmation(subtype, message string) *Error {
	return NewError(ExitConfirmation, "confirmation", subtype, message)
}

func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	var cliErr *Error
	if errors.As(err, &cliErr) {
		return cliErr
	}
	return Internal("unexpected", err.Error(), err)
}

type successEnvelope struct {
	OK   bool  `json:"ok"`
	Data any   `json:"data"`
	Meta *Meta `json:"meta,omitempty"`
}

type errorEnvelope struct {
	OK    bool   `json:"ok"`
	Error *Error `json:"error"`
	Meta  *Meta  `json:"meta,omitempty"`
}

type Printer struct {
	Out                 io.Writer
	ErrOut              io.Writer
	ExecutingCLIVersion string
	AutoUpdate          *AutoUpdateMeta
}

func (p *Printer) Success(data any) error {
	return writeJSON(p.Out, successEnvelope{OK: true, Data: data, Meta: p.meta("", nil)})
}

func (p *Printer) SuccessWithMeta(data any, meta Meta) error {
	return writeJSON(p.Out, successEnvelope{OK: true, Data: data, Meta: p.meta(meta.RequestID, meta.WaitTimedOut)})
}

// Business is an alias retained for command call sites. Every command uses the
// same protocol envelope so Agents never need command-specific JSON parsing.
func (p *Printer) Business(data any) error {
	return p.Success(data)
}

func (p *Printer) Failure(err error) int {
	cliErr := AsError(err)
	if cliErr.Code == 0 {
		cliErr.Code = ExitInternal
	}
	// stdout is the single machine protocol stream for both success and failure.
	// Progress and human guidance belong on stderr and can never corrupt the
	// final JSON envelope consumed by an Agent.
	_ = writeJSON(p.Out, errorEnvelope{OK: false, Error: cliErr, Meta: p.meta(cliErr.RequestID, nil)})
	return cliErr.Code
}

func (p *Printer) meta(requestID string, waitTimedOut *bool) *Meta {
	meta := &Meta{ExecutingCLIVersion: p.ExecutingCLIVersion, RequestID: requestID, WaitTimedOut: waitTimedOut, AutoUpdate: p.AutoUpdate}
	if meta.ExecutingCLIVersion == "" && meta.RequestID == "" && meta.WaitTimedOut == nil && meta.AutoUpdate == nil {
		return nil
	}
	return meta
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
