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
	CLIVersion            string `json:"cli_version"`
	SkillVersion          string `json:"skill_version"`
	FullSkillBundleDigest string `json:"full_skill_bundle_digest"`
	EmbeddedContentDigest string `json:"embedded_content_digest"`
	WaitTimedOut          *bool  `json:"wait_timed_out,omitempty"`
}

type Error struct {
	Code          int    `json:"-"`
	Type          string `json:"type"`
	Subtype       string `json:"subtype"`
	Message       string `json:"message"`
	Retryable     bool   `json:"retryable"`
	Hint          string `json:"hint,omitempty"`
	PublicationID string `json:"publication_id,omitempty"`
	ConsoleURL    string `json:"console_url,omitempty"`
	Details       any    `json:"details,omitempty"`
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
	OK   bool `json:"ok"`
	Data any  `json:"data"`
	Meta Meta `json:"meta"`
}

type errorEnvelope struct {
	OK    bool   `json:"ok"`
	Error *Error `json:"error"`
	Meta  Meta   `json:"meta"`
}

type Printer struct {
	Out    io.Writer
	ErrOut io.Writer
	Meta   Meta
}

func (p *Printer) Success(data any) error {
	return p.SuccessWithMeta(data, p.Meta)
}

func (p *Printer) SuccessWithMeta(data any, meta Meta) error {
	return writeJSON(p.Out, successEnvelope{OK: true, Data: data, Meta: meta})
}

func (p *Printer) Failure(err error) int {
	cliErr := AsError(err)
	if cliErr.Code == 0 {
		cliErr.Code = ExitInternal
	}
	_ = writeJSON(p.ErrOut, errorEnvelope{OK: false, Error: cliErr, Meta: p.Meta})
	return cliErr.Code
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
