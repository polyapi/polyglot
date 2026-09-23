package delegate

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
)

// Protocol is the integer advertised as POLY_DELEGATE_PROTOCOL and in every envelope.
const Protocol uint32 = 1

// Version is the same integer; named to match the RFC versioning note (POLY_DELEGATE_VERSION).
const Version = Protocol

// Op is an adapter operation name (`op` in the envelope).
type Op string

const (
	OpCapabilities  Op = "capabilities"
	OpGenerate      Op = "generate"
	OpDiscover      Op = "discover"
	OpInspect       Op = "inspect"
	OpExtract       Op = "extract"
	OpPrepare       Op = "prepare"
	OpWriteReceipt  Op = "write_receipt"
	OpParseFunction Op = "parse_function"
	OpClear         Op = "clear"
)

func (o Op) String() string { return string(o) }

// RequiredForTSPython are ops TypeScript and Python adapters must implement in v1.
func RequiredForTSPython() []Op {
	return []Op{
		OpCapabilities,
		OpGenerate,
		OpInspect,
		OpExtract,
		OpPrepare,
		OpParseFunction,
	}
}

// RequestConfig is the host-supplied config block on every request.
type RequestConfig struct {
	PolyPath  string `json:"poly_path"`
	DisableAI bool   `json:"disable_ai,omitempty"`
}

// Request is one JSON request document written to adapter stdin.
type Request struct {
	Protocol    uint32          `json:"protocol"`
	ID          string          `json:"id"`
	Op          Op              `json:"op"`
	ProjectRoot string          `json:"project_root"`
	Lang        string          `json:"lang"`
	Config      RequestConfig   `json:"config"`
	Params      json.RawMessage `json:"params"`
	Stream      bool            `json:"stream,omitempty"`
}

// NewRequest builds a protocol-1 request with a random UUID.
func NewRequest(op Op, projectRoot, lang string, cfg RequestConfig, params any) (Request, error) {
	raw, err := marshalParams(params)
	if err != nil {
		return Request{}, err
	}
	return Request{
		Protocol:    Protocol,
		ID:          newRequestID(),
		Op:          op,
		ProjectRoot: projectRoot,
		Lang:        lang,
		Config:      cfg,
		Params:      raw,
	}, nil
}

func marshalParams(params any) (json.RawMessage, error) {
	if params == nil {
		return json.RawMessage(`{}`), nil
	}
	if raw, ok := params.(json.RawMessage); ok {
		if len(raw) == 0 {
			return json.RawMessage(`{}`), nil
		}
		return raw, nil
	}
	b, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Response is one JSON response document read from adapter stdout.
type Response struct {
	Protocol uint32            `json:"protocol"`
	ID       string            `json:"id"`
	OK       bool              `json:"ok"`
	Op       Op                `json:"op"`
	Result   json.RawMessage   `json:"result"`
	Warnings []Warning         `json:"warnings"`
	Error    *AdapterErrorBody `json:"error"`
}

// Warning is attached to a response.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

// AdapterErrorBody is the failure object when ok is false.
type AdapterErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// Capabilities is the capabilities result.
type Capabilities struct {
	Protocol    uint32 `json:"protocol"`
	MinProtocol uint32 `json:"min_protocol"`
	Ops         []Op   `json:"ops"`
	Lang        string `json:"lang"`
	SDKVersion  string `json:"sdk_version"`
}

// CompatibleWithHost is true when the adapter covers the host protocol integer.
func (c Capabilities) CompatibleWithHost() bool {
	return c.Protocol >= Protocol && Protocol >= c.MinProtocol
}

// Supports reports whether the adapter implements op (capabilities is always supported).
func (c Capabilities) Supports(op Op) bool {
	if op == OpCapabilities {
		return true
	}
	for _, o := range c.Ops {
		if o == op {
			return true
		}
	}
	return false
}

// GenerateParams are generate params. Specs is the opaque GET /specs body (a JSON array).
type GenerateParams struct {
	Specs   json.RawMessage `json:"specs"`
	NoTypes bool            `json:"no_types,omitempty"`
}

// SpecsJSON marshals a spec list for GenerateParams. Nil becomes [].
func SpecsJSON(specs any) json.RawMessage {
	if specs == nil {
		return json.RawMessage("[]")
	}
	if raw, ok := specs.(json.RawMessage); ok {
		if len(raw) == 0 {
			return json.RawMessage("[]")
		}
		return raw
	}
	b, err := json.Marshal(specs)
	if err != nil {
		return json.RawMessage("[]")
	}
	return b
}

// GenerateResult is the generate result.
type GenerateResult struct {
	FilesWritten []string `json:"files_written"`
	Stats        any      `json:"stats,omitempty"`
}

// DiscoverParams are discover params.
type DiscoverParams struct {
	Paths        []string `json:"paths,omitempty"`
	ExcludePaths []string `json:"exclude_paths,omitempty"`
	Types        []string `json:"types,omitempty"`
}

// DiscoverResult is the discover result.
type DiscoverResult struct {
	Deployables []DeployableDesc `json:"deployables"`
}

// DeployableDesc is one deployable found via polyConfig.
type DeployableDesc struct {
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	Context     string   `json:"context"`
	File        string   `json:"file"`
	Export      string   `json:"export,omitempty"`
	ContentHash string   `json:"content_hash,omitempty"`
	Receipt     *Receipt `json:"receipt,omitempty"`
}

// Receipt is an in-file / cached deploy receipt.
type Receipt struct {
	Instance   string `json:"instance,omitempty"`
	ID         string `json:"id,omitempty"`
	Hash       string `json:"hash,omitempty"`
	DeployedAt string `json:"deployed_at,omitempty"`
}

// ExtractParams are extract params.
type ExtractParams struct {
	File string `json:"file"`
	Type string `json:"type"`
}

// ExtractResult is the extract result. Payload is the canonical JSON Glide diffs and pushes.
type ExtractResult struct {
	Payload     any    `json:"payload"`
	ContentHash string `json:"content_hash,omitempty"`
	Meta        any    `json:"meta,omitempty"`
}

// InspectParams are inspect params (batch parse for prepare).
type InspectParams struct {
	Files []string `json:"files"`
}

// InspectResult is the inspect result.
type InspectResult struct {
	Items []InspectItem `json:"items"`
}

// InspectItem is one code file’s signature and current docs.
type InspectItem struct {
	File        string       `json:"file"`
	Type        string       `json:"type"`
	Code        string       `json:"code"`
	Description string       `json:"description,omitempty"`
	Arguments   []InspectArg `json:"arguments,omitempty"`
	Returns     *InspectArg  `json:"returns,omitempty"`
	DisableAI   bool         `json:"disable_ai,omitempty"`
}

// InspectArg is a function parameter or return type for inspect/prepare docs.
type InspectArg struct {
	Name        string `json:"name,omitempty"`
	Type        any    `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
}

// NeedsDescription is true when the function or any argument is missing a description.
func (it InspectItem) NeedsDescription() bool {
	if strings.TrimSpace(it.Description) == "" {
		return true
	}
	for _, a := range it.Arguments {
		if strings.TrimSpace(a.Description) == "" {
			return true
		}
	}
	if it.Returns != nil && strings.TrimSpace(it.Returns.Description) == "" {
		return true
	}
	return false
}

// Docs returns the inspect item as prepare docs (after optional AI fill-in).
func (it InspectItem) Docs() PrepareDocs {
	return PrepareDocs{
		Description: it.Description,
		Arguments:   it.Arguments,
		Returns:     it.Returns,
	}
}

// PrepareDocs is host-supplied documentation for one file.
type PrepareDocs struct {
	Description string       `json:"description,omitempty"`
	Arguments   []InspectArg `json:"arguments,omitempty"`
	Returns     *InspectArg  `json:"returns,omitempty"`
}

// PrepareFile is one file for the write-only prepare op.
type PrepareFile struct {
	File string       `json:"file"`
	Docs *PrepareDocs `json:"docs,omitempty"`
}

// PrepareParams are prepare params. The host runs AI; the adapter only writes comments.
type PrepareParams struct {
	Files       []PrepareFile `json:"files,omitempty"`
	DisableDocs bool          `json:"disable_docs,omitempty"`
}

// PrepareResult is the prepare result.
type PrepareResult struct {
	ChangedFiles []string `json:"changed_files"`
	Skipped      []string `json:"skipped"`
}

// WriteReceiptParams are write_receipt params.
type WriteReceiptParams struct {
	File    string  `json:"file"`
	Receipt Receipt `json:"receipt"`
}

// OkResult is `{ ok: true }` results (write_receipt, clear).
type OkResult struct {
	OK bool `json:"ok"`
}

// ParseFunctionParams are parse_function params.
type ParseFunctionParams struct {
	File string       `json:"file"`
	Name string       `json:"name"`
	Kind FunctionKind `json:"kind"`
}

// FunctionKind is server vs client for parse_function.
type FunctionKind string

const (
	KindServer FunctionKind = "server"
	KindClient FunctionKind = "client"
)

func newRequestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
