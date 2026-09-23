package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/polyapi/polyglot/src/exitcode"
)

// Error is a failure talking to a PolyAPI instance.
type Error struct {
	Kind        string
	Msg         string
	StatusCode  int
	Method      string
	URL         string
	Body        string
	Action      string
	Permissions []string
	Err         error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Msg != "" {
		return e.Msg
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "api error"
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) Status() int {
	if e == nil {
		return 0
	}
	return e.StatusCode
}

func (e *Error) ExitCode() int {
	if e == nil {
		return exitcode.Failure
	}
	switch e.Kind {
	case "unauthorized", "forbidden", "config":
		return exitcode.Auth
	case "network":
		return exitcode.Network
	default:
		return exitcode.Failure
	}
}

// UserMessage is the short operator-facing text for an API error.
// Status errors prefer a JSON `message` field from the body when present.
func UserMessage(err error) string {
	if err == nil {
		return ""
	}
	var e *Error
	if !errors.As(err, &e) || e == nil {
		return err.Error()
	}
	switch e.Kind {
	case "unauthorized", "forbidden", "network", "config":
		return e.Error()
	}
	if msg := jsonMessage(e.Body); msg != "" {
		return msg
	}
	return e.Error()
}

func jsonMessage(body string) string {
	body = strings.TrimSpace(body)
	if body == "" || body[0] != '{' {
		return ""
	}
	var m struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return ""
	}
	return strings.TrimSpace(m.Message)
}

func unauthorized(body string) *Error {
	return &Error{
		Kind:       "unauthorized",
		StatusCode: 401,
		Body:       body,
		Msg:        "API key rejected (HTTP 401). Run `polyapi auth login` with a valid key.",
	}
}

// Forbidden is a TS-style HTTP 403 error.
func Forbidden(action string, permissions []string, body string) *Error {
	return forbidden(action, permissions, body)
}

func forbidden(action string, permissions []string, body string) *Error {
	return &Error{
		Kind:        "forbidden",
		StatusCode:  403,
		Action:      action,
		Permissions: permissions,
		Body:        body,
		Msg:         forbiddenMessage(action, permissions),
	}
}

func statusErr(method, url string, status int, body string) *Error {
	return &Error{
		Kind:       "status",
		StatusCode: status,
		Method:     method,
		URL:        url,
		Body:       body,
		Msg:        fmt.Sprintf("%s %s failed with HTTP %d: %s", method, url, status, body),
	}
}

func invalidResource(resource string) *Error {
	return &Error{
		Kind: "invalid_resource",
		Msg:  fmt.Sprintf("invalid resource path %q; use a relative collection such as variables or functions/server", resource),
	}
}

func unexpectedList(url string) *Error {
	return &Error{Kind: "unexpected_list", Msg: "unexpected list payload from " + url}
}

func networkErr(msg string) *Error {
	return &Error{Kind: "network", Msg: "network error: " + msg}
}

func jsonErr(err error) *Error {
	return &Error{Kind: "json", Msg: "invalid JSON: " + err.Error(), Err: err}
}

func configErr(err error) *Error {
	return &Error{Kind: "config", Msg: err.Error(), Err: err}
}

// FromStatus maps HTTP status to a typed error.
func FromStatus(method, url string, status int, body string) *Error {
	switch status {
	case 401:
		return unauthorized(body)
	case 403:
		return forbidden("perform this action", nil, body)
	default:
		return statusErr(method, url, status, body)
	}
}

func forbiddenMessage(action string, permissions []string) string {
	if len(permissions) == 0 {
		return fmt.Sprintf("You don't have permission to %s (HTTP 403).", action)
	}
	missing := strings.Join(permissions, ", ")
	if len(permissions) == 1 {
		return fmt.Sprintf("You don't have permission to %s. Missing permission: %s.", action, missing)
	}
	return fmt.Sprintf("You don't have permissions to %s. Missing permissions: %s.", action, missing)
}
