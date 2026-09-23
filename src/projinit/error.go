package projinit

import (
	"fmt"

	"github.com/polyapi/polyglot/src/exitcode"
)

// Error is an init failure with a process exit code.
type Error struct {
	Msg  string
	Code int
	Err  error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil && e.Msg != "" {
		return e.Msg + ": " + e.Err.Error()
	}
	if e.Msg != "" {
		return e.Msg
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "init error"
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) ExitCode() int {
	if e == nil || e.Code == 0 {
		return exitcode.Failure
	}
	return e.Code
}

func usage(msg string) *Error {
	return &Error{Code: exitcode.Usage, Msg: msg}
}

func fail(msg string) *Error {
	return &Error{Code: exitcode.Failure, Msg: msg}
}

func wrap(code int, msg string, err error) *Error {
	return &Error{Code: code, Msg: msg, Err: err}
}

func network(msg string, err error) *Error {
	if err == nil {
		return &Error{Code: exitcode.Network, Msg: msg}
	}
	return wrap(exitcode.Network, msg, err)
}

func fmtFail(format string, args ...any) *Error {
	return fail(fmt.Sprintf(format, args...))
}
