package delegate

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// AdapterCreds used to be injected as POLY_* for adapter-side generate HTTP.
// Protocol 1 freeze: the host fetches /specs and description-generation itself,
// so credentials are not passed into the adapter process.
type AdapterCreds struct {
	APIKey  string
	BaseURL string
}

func (c AdapterCreds) String() string {
	key := ""
	if c.APIKey != "" {
		key = "********"
	}
	return fmt.Sprintf("AdapterCreds{api_key:%s base_url:%s}", key, c.BaseURL)
}

// Delegate is a resolved adapter plus spawn settings.
type Delegate struct {
	ProjectRoot string
	Lang        Language
	Argv        []string
	PolyPath    string
	DisableAI   bool
	Creds       AdapterCreds
	TimeoutSecs uint64
	Runner      Runner
}

func (d Delegate) String() string {
	return fmt.Sprintf("Delegate{lang:%s argv:%v poly_path:%s}", d.Lang, d.Argv, d.PolyPath)
}

// New builds a Delegate that spawns a real subprocess.
func New(projectRoot string, lang Language, argv []string, polyPath string) Delegate {
	return Delegate{
		ProjectRoot: projectRoot,
		Lang:        lang,
		Argv:        argv,
		PolyPath:    polyPath,
		TimeoutSecs: timeoutOverrideFromEnv(),
		Runner:      ProcessRunner{},
	}
}

func (d Delegate) WithCreds(creds AdapterCreds) Delegate {
	d.Creds = creds
	return d
}

func (d Delegate) WithRunner(runner Runner) Delegate {
	d.Runner = runner
	return d
}

func (d Delegate) CommandDisplay() string {
	return strings.Join(d.Argv, " ")
}

func (d Delegate) Capabilities() (Capabilities, error) {
	caps, _, err := invoke[Capabilities](d, OpCapabilities, map[string]any{})
	if err != nil {
		return Capabilities{}, err
	}
	if !caps.CompatibleWithHost() {
		return Capabilities{}, protocolMismatch(caps)
	}
	return caps, nil
}

func (d Delegate) Generate(params GenerateParams) (GenerateResult, error) {
	if len(params.Specs) == 0 {
		params.Specs = SpecsJSON(nil)
	}
	return checked[GenerateResult](d, OpGenerate, params)
}

func (d Delegate) Inspect(params InspectParams) (InspectResult, error) {
	return checked[InspectResult](d, OpInspect, params)
}

func (d Delegate) Discover(params DiscoverParams) (DiscoverResult, error) {
	return checked[DiscoverResult](d, OpDiscover, params)
}

func (d Delegate) Extract(params ExtractParams) (ExtractResult, error) {
	return checked[ExtractResult](d, OpExtract, params)
}

func (d Delegate) Prepare(params PrepareParams) (PrepareResult, error) {
	return checked[PrepareResult](d, OpPrepare, params)
}

func (d Delegate) WriteReceipt(params WriteReceiptParams) (OkResult, error) {
	return checked[OkResult](d, OpWriteReceipt, params)
}

func (d Delegate) ParseFunction(params ParseFunctionParams) (json.RawMessage, error) {
	return checked[json.RawMessage](d, OpParseFunction, params)
}

func (d Delegate) Clear() (OkResult, error) {
	return checked[OkResult](d, OpClear, map[string]any{})
}

func checked[R any](d Delegate, op Op, params any) (R, error) {
	var zero R
	caps, err := d.Capabilities()
	if err != nil {
		return zero, err
	}
	if !caps.Supports(op) {
		return zero, protocolErr(fmt.Sprintf(
			"adapter %s (sdk %s) does not support `%s`; update the language SDK",
			caps.Lang, caps.SDKVersion, op,
		))
	}
	result, _, err := invoke[R](d, op, params)
	return result, err
}

func invoke[R any](d Delegate, op Op, params any) (R, []Warning, error) {
	var zero R
	req, err := NewRequest(op, d.ProjectRoot, d.Lang.String(), RequestConfig{
		PolyPath:  d.PolyPath,
		DisableAI: d.DisableAI,
	}, params)
	if err != nil {
		return zero, nil, jsonErr(err.Error())
	}
	stdin, err := json.Marshal(req)
	if err != nil {
		return zero, nil, jsonErr(err.Error())
	}
	timeout := timeoutFor(op, d.TimeoutSecs)
	runner := d.Runner
	if runner == nil {
		runner = ProcessRunner{}
	}
	output, err := runner.Run(Job{
		Argv:     d.Argv,
		Cwd:      d.ProjectRoot,
		Stdin:    string(stdin),
		Timeout:  timeout,
		ExtraEnv: nil,
	})
	if err != nil {
		return zero, nil, err
	}
	if output.TimedOut {
		secs := uint64(timeout.Seconds())
		if secs < 1 {
			secs = 1
		}
		return zero, nil, timeoutErr(op, secs)
	}
	if err := missingAfterSpawn(d.Argv, d.Lang, output); err != nil {
		return zero, nil, err
	}
	return interpret[R](op, output)
}

func protocolMismatch(caps Capabilities) *Error {
	if caps.Protocol < Protocol {
		return protocolErr(fmt.Sprintf(
			"adapter protocol %d (min %d) is too old for polyapi protocol %d; update the language SDK",
			caps.Protocol, caps.MinProtocol, Protocol,
		))
	}
	return protocolErr(fmt.Sprintf(
		"adapter protocol %d (min %d) is not compatible with polyapi protocol %d; update polyapi or pin the language SDK",
		caps.Protocol, caps.MinProtocol, Protocol,
	))
}

func interpret[R any](op Op, output RawOutput) (R, []Warning, error) {
	var zero R
	status := 1
	if output.Status != nil {
		status = *output.Status
	}
	value, err := ParseJSONDocument(output.Stdout)
	if err != nil {
		if looksLikeMissingModule(output.Stderr) {
			return zero, nil, missingErr(fmt.Sprintf("adapter not found. %s; stderr: %s", err.Error(), trimStderr(output.Stderr)))
		}
		return zero, nil, mapNonzero(op, status, nil, fmt.Sprintf("adapter `%s` did not write a JSON response (%s). %s", op, err.Error(), trimStderr(output.Stderr)), nil)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return zero, nil, protocolErr("adapter response missing envelope fields: " + err.Error())
	}
	var envelope Response
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return zero, nil, protocolErr("adapter response missing envelope fields: " + err.Error())
	}

	if envelope.Protocol != Protocol && envelope.OK && op != OpCapabilities {
		return zero, nil, protocolErr(fmt.Sprintf(
			"adapter response protocol %d does not match host %d",
			envelope.Protocol, Protocol,
		))
	}

	if status == 12 || (!envelope.OK && status == 12) {
		msg := fmt.Sprintf("adapter `%s` completed with partial failure", op)
		if envelope.Error != nil && envelope.Error.Message != "" {
			msg = envelope.Error.Message
		}
		return zero, nil, partialErr(msg, envelope.Warnings)
	}

	if !envelope.OK {
		return zero, nil, mapAdapterError(op, status, envelope.Error, envelope.Warnings)
	}

	if status != 0 && status != 12 {
		return zero, nil, mapNonzero(op, status, envelope.Error, fmt.Sprintf("adapter `%s` exited %d", op, status), envelope.Warnings)
	}

	if len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return zero, nil, protocolErr(fmt.Sprintf("adapter `%s` returned ok without a result", op))
	}
	var typed R
	if err := json.Unmarshal(envelope.Result, &typed); err != nil {
		return zero, nil, jsonErr(err.Error())
	}
	return typed, envelope.Warnings, nil
}

func mapAdapterError(op Op, status int, errorBody *AdapterErrorBody, warnings []Warning) *Error {
	message := fmt.Sprintf("adapter `%s` failed", op)
	if errorBody != nil {
		if errorBody.Code == "" {
			message = errorBody.Message
		} else {
			message = errorBody.Code + ": " + errorBody.Message
		}
	}
	return mapNonzero(op, status, errorBody, message, warnings)
}

func mapNonzero(op Op, status int, errorBody *AdapterErrorBody, message string, warnings []Warning) *Error {
	codeHint := ""
	if errorBody != nil {
		codeHint = strings.ToLower(errorBody.Code)
	}
	if status == 12 || codeHint == "partial" {
		return partialErr(message, warnings)
	}
	if status == 11 || strings.Contains(codeHint, "protocol") {
		return protocolErr(message)
	}
	if status == 10 {
		return missingErr(message)
	}
	if status == 4 || strings.Contains(codeHint, "network") {
		return networkErr(message)
	}
	if status == 3 || strings.Contains(codeHint, "auth") {
		return authErr(message)
	}
	if status == 2 {
		return usageErr(message)
	}
	if op == OpGenerate && (strings.Contains(codeHint, "network") || strings.Contains(codeHint, "http")) {
		return networkErr(message)
	}
	exit := status
	if exit == 0 {
		exit = 1
	}
	return adapterErr(message, exit)
}

// ParseJSONDocument parses a single JSON document from stdout, or the last `{...}` object.
func ParseJSONDocument(stdout string) (any, error) {
	trimmed := strings.TrimSpace(stdout)
	trimmed = strings.TrimPrefix(trimmed, "\ufeff")
	if trimmed == "" {
		return nil, jsonErr("adapter stdout was empty")
	}
	var v any
	if err := json.Unmarshal([]byte(trimmed), &v); err == nil {
		return v, nil
	}
	if idx := strings.LastIndex(trimmed, "{"); idx >= 0 {
		if err := json.Unmarshal([]byte(trimmed[idx:]), &v); err == nil {
			return v, nil
		}
	}
	return nil, jsonErr(fmt.Sprintf("could not parse JSON from adapter stdout (%d bytes)", len(stdout)))
}

func trimStderr(s string) string {
	t := strings.TrimSpace(s)
	if t == "" {
		return "(no stderr)"
	}
	line := t
	if i := strings.IndexByte(t, '\n'); i >= 0 {
		line = t[:i]
	}
	if utf8.RuneCountInString(line) > 400 || len(line) > 400 {
		if len(line) > 400 {
			line = line[:400] + "…"
		}
	}
	return line
}
