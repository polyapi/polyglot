package glide

import (
	"strings"

	"github.com/polyapi/polyglot/src/exitcode"
)

const kindNothingFound = "nothing_found"

// Error is a Glide failure with a process exit code.
type Error struct {
	Kind string
	Msg  string
	Code int
	Err  error
}

// IsNothingFound is true when a deploy command found no local (or pullable) deployables.
func IsNothingFound(err error) bool {
	if err == nil {
		return false
	}
	ge, ok := err.(*Error)
	return ok && ge.Kind == kindNothingFound
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil && e.Msg != "" {
		return e.Msg + ": " + e.Err.Error()
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Msg
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) ExitCode() int {
	if e == nil || e.Code == 0 {
		return exitcode.Failure
	}
	return e.Code
}

func fail(msg string) *Error {
	return &Error{Msg: msg, Code: exitcode.Failure}
}

func failCode(code int, msg string) *Error {
	return &Error{Msg: msg, Code: code}
}

func wrap(msg string, err error) *Error {
	if err == nil {
		return fail(msg)
	}
	if ge, ok := err.(*Error); ok {
		if msg == "" {
			return ge
		}
		return &Error{Msg: msg, Code: ge.ExitCode(), Err: ge}
	}
	type coder interface {
		ExitCode() int
		Error() string
	}
	if c, ok := err.(coder); ok {
		return &Error{Msg: msg, Code: c.ExitCode(), Err: err}
	}
	return &Error{Msg: msg, Code: exitcode.Failure, Err: err}
}

func (e *Engine) nothingFound() *Error {
	msg := "no deployables found"
	if hint := e.scopeHint(); hint != "" {
		msg += " (" + hint + ")"
	}
	msg += "; expected a polyConfig binding or a JSON/JSONC artifact"
	return &Error{Kind: kindNothingFound, Msg: msg, Code: exitcode.Failure}
}

func (e *Engine) scopeHint() string {
	if e == nil {
		return ""
	}
	var parts []string
	if n := len(e.Scope.Paths); n > 0 {
		parts = append(parts, "path="+strings.Join(e.Scope.Paths, ","))
	}
	if n := len(e.Scope.ExcludePaths); n > 0 {
		parts = append(parts, "exclude-path="+strings.Join(e.Scope.ExcludePaths, ","))
	}
	if n := len(e.Scope.Types); n > 0 && n < len(DeployOrder()) {
		parts = append(parts, "types="+strings.Join(e.Scope.Types, ","))
	}
	if n := len(e.Scope.Contexts); n > 0 {
		parts = append(parts, "contexts="+strings.Join(e.Scope.Contexts, ","))
	}
	return strings.Join(parts, "; ")
}
