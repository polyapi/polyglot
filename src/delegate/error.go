package delegate

import (
	"fmt"
	"strings"

	"github.com/polyapi/polyglot/src/exitcode"
)

// Error is a failure resolving, spawning, or talking to a language adapter.
type Error struct {
	Kind     string
	Msg      string
	Code     int
	Langs    []Language
	Warnings []Warning
	Err      error
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
	return "delegate error"
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) ExitCode() int {
	if e == nil {
		return exitcode.Failure
	}
	if e.Code != 0 {
		return e.Code
	}
	return exitcode.Failure
}

func undetected() *Error {
	return &Error{
		Kind: "undetected",
		Code: exitcode.Usage,
		Msg:  "could not detect the project language; pass --lang typescript|python or set language in .poly/config.toml",
	}
}

func ambiguous(langs []Language) *Error {
	names := make([]string, len(langs))
	for i, l := range langs {
		names[i] = l.String()
	}
	return &Error{
		Kind:  "ambiguous",
		Code:  exitcode.Usage,
		Langs: langs,
		Msg:   fmt.Sprintf("multiple languages detected (%s); pass --lang", strings.Join(names, ", ")),
	}
}

func usageErr(msg string) *Error {
	return &Error{Kind: "usage", Code: exitcode.Usage, Msg: msg}
}

func missingErr(msg string) *Error {
	return &Error{Kind: "missing", Code: exitcode.AdapterMissing, Msg: msg}
}

func protocolErr(msg string) *Error {
	return &Error{Kind: "protocol", Code: exitcode.Protocol, Msg: msg}
}

func authErr(msg string) *Error {
	return &Error{Kind: "auth", Code: exitcode.Auth, Msg: msg}
}

func networkErr(msg string) *Error {
	return &Error{Kind: "network", Code: exitcode.Network, Msg: msg}
}

func partialErr(msg string, warnings []Warning) *Error {
	return &Error{Kind: "partial", Code: exitcode.Partial, Msg: msg, Warnings: warnings}
}

func adapterErr(msg string, exit int) *Error {
	return &Error{Kind: "adapter", Code: mapAdapterExit(exit), Msg: msg}
}

func timeoutErr(op Op, seconds uint64) *Error {
	return &Error{
		Kind: "timeout",
		Code: exitcode.Failure,
		Msg:  fmt.Sprintf("adapter timed out after %ds (%s)", seconds, op),
	}
}

func javaUnsupported() *Error {
	return &Error{
		Kind: "java",
		Code: exitcode.AdapterMissing,
		Msg:  "Java adapter is not implemented yet; pass --adapter when one exists",
	}
}

func jsonErr(msg string) *Error {
	return &Error{Kind: "json", Code: exitcode.Failure, Msg: "invalid JSON from adapter: " + msg}
}

func ioErr(msg string) *Error {
	return &Error{Kind: "io", Code: exitcode.Failure, Msg: "failed to run adapter: " + msg}
}

func wrapConfig(err error) *Error {
	code := exitcode.Failure
	type coder interface{ ExitCode() int }
	if c, ok := err.(coder); ok {
		code = c.ExitCode()
	}
	return &Error{Kind: "config", Code: code, Msg: err.Error(), Err: err}
}

func mapAdapterExit(code int) int {
	switch code {
	case 0, 1, 2, 3, 4, 10, 11, 12:
		return code
	default:
		return exitcode.Failure
	}
}

// MissingCommand is the exit-10 error when an adapter binary or SDK module is missing.
func MissingCommand(argv []string, lang Language) *Error {
	return missingCommand(argv, lang)
}

func missingCommand(argv []string, lang Language) *Error {
	cmd := strings.Join(argv, " ")
	hint := "Install the PolyAPI language SDK in this project, or pass --adapter."
	if lang != "" {
		hint = lang.SDKInstallHint()
	}
	return missingErr(fmt.Sprintf("adapter not found (%s). %s", cmd, hint))
}

func sdkNotInstalled(lang Language) *Error {
	return missingErr(fmt.Sprintf("PolyAPI %s SDK is not installed in this project. %s", lang, lang.SDKInstallHint()))
}

func adapterBinaryMissing(lang Language, expected string) *Error {
	return missingErr(fmt.Sprintf(
		"PolyAPI %s SDK is installed but has no adapter at %s. Update the SDK or pass --adapter.",
		lang, expected,
	))
}
