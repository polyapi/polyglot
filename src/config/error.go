package config

import (
	"fmt"

	"github.com/polyapi/polyglot/src/exitcode"
)

// Error is a config/auth failure with a process exit code.
type Error struct {
	Msg  string
	Code int
	Err  error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return e.Msg + ": " + e.Err.Error()
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

func invalidURL(input string) *Error {
	return &Error{
		Code: exitcode.Auth,
		Msg:  fmt.Sprintf("invalid Poly instance or URL %q; use na1, eu1, na2, dev, local, or an http(s) URL", input),
	}
}

func missingCredentials() *Error {
	return &Error{
		Code: exitcode.Auth,
		Msg:  "no API key or base URL configured; run `polyapi auth login` or set POLY_API_KEY and POLY_API_BASE_URL",
	}
}

func decryptError() *Error {
	return &Error{
		Code: exitcode.Auth,
		Msg:  "API key was encrypted on another device; re-run `polyapi auth login` or set POLY_API_KEY",
	}
}

func nonInteractive() *Error {
	return &Error{
		Code: exitcode.Auth,
		Msg:  "cannot prompt in non-interactive mode; pass the base URL and API key",
	}
}

func ioError(path string, err error) *Error {
	return &Error{
		Code: exitcode.Failure,
		Msg:  fmt.Sprintf("failed to read %s", path),
		Err:  err,
	}
}

func tomlError(path string, err error) *Error {
	return &Error{
		Code: exitcode.Failure,
		Msg:  fmt.Sprintf("invalid config TOML in %s", path),
		Err:  err,
	}
}

func message(msg string) *Error {
	return &Error{Code: exitcode.Failure, Msg: msg}
}

func unknownSetting(key string) *Error {
	return &Error{
		Code: exitcode.Usage,
		Msg:  fmt.Sprintf("unknown config key %q; use instance, base_url, api_version, language, adapter, api_key, url_source, or key_source", key),
	}
}

func secretSetting(key string) *Error {
	return &Error{
		Code: exitcode.Usage,
		Msg:  fmt.Sprintf("%s cannot be set with `polyapi config set`; run `polyapi auth login`", key),
	}
}
