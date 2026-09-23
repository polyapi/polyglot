package glide

import (
	"strings"

	"github.com/polyapi/polyglot/src/delegate"
)

// Canonical deployable types (singular).
const (
	TypeVariable       = "variable"
	TypeTable          = "table"
	TypeSchema         = "schema"
	TypeAIFunction     = "ai-function"
	TypeAPIFunction    = "api-function"
	TypeClientFunction = "client-function"
	TypeServerFunction = "server-function"
	TypeWebhook        = "webhook"
	TypeTrigger        = "trigger"
	TypeJob            = "job"
	TypeSnippet        = "snippet"
	TypeSubscription   = "subscription"
	TypeApplication    = "application"
)

// GraphQL subscription kinds for `subscription init --type`.
const (
	SubscriptionTypeCustom = "CUSTOM"
	SubscriptionTypeOHIP   = "OHIP"
)

// Trigger source kinds for `trigger init --type`.
const (
	TriggerSourceWebhook      = "webhook"
	TriggerSourceErrorHandler = "error-handler"
)

const (
	KindJSON = "json"
	KindCode = "code"
)

const (
	ActionCreate      = "created"
	ActionUpdate      = "updated"
	ActionSkip        = "skipped"
	ActionFailed      = "failed"
	ActionBlocked     = "blocked"
	ActionDeleted     = "deleted"
	ActionOrphan      = "orphan"
	ActionWouldCreate = "would_create"
	ActionWouldUpdate = "would_update"
	ActionWouldDelete = "would_delete"
)

const (
	SeverityError = "error"
	SeverityWarn  = "warn"
	SeverityInfo  = "info"
)

const (
	CatEnv           = "env"
	CatMetadata      = "metadata"
	CatDiscovery     = "discovery"
	CatCrossResource = "cross-resource"
	CatConfig        = "config"
)

// DeployOrder is the baseline push order. Orphan cleanup runs reverse.
func DeployOrder() []string {
	return []string{
		TypeVariable, TypeTable, TypeSchema,
		TypeAPIFunction, TypeClientFunction, TypeAIFunction, TypeServerFunction,
		TypeWebhook, TypeTrigger, TypeJob, TypeSnippet, TypeSubscription, TypeApplication,
	}
}

// Collection is the REST path for typ.
func Collection(typ string) string {
	switch typ {
	case TypeVariable:
		return "variables"
	case TypeTable:
		return "tables"
	case TypeSchema:
		return "schemas"
	case TypeAIFunction:
		return "functions/ai"
	case TypeAPIFunction:
		return "functions/api"
	case TypeClientFunction:
		return "functions/client"
	case TypeServerFunction:
		return "functions/server"
	case TypeWebhook:
		return "webhooks"
	case TypeTrigger:
		return "triggers"
	case TypeJob:
		return "jobs"
	case TypeSnippet:
		return "snippets"
	case TypeSubscription:
		return "subscriptions/graphql"
	case TypeApplication:
		return "applications"
	default:
		return ""
	}
}

// IsFunction is true for the four function kinds.
func IsFunction(typ string) bool {
	switch typ {
	case TypeAIFunction, TypeAPIFunction, TypeClientFunction, TypeServerFunction:
		return true
	default:
		return false
	}
}

// IdentityKey is type:context.name, or type:name for jobs/triggers.
func IdentityKey(typ, context, name string) string {
	if typ == TypeJob || typ == TypeTrigger || typ == TypeApplication || context == "" {
		return typ + ":" + name
	}
	return typ + ":" + context + "." + name
}

// DisplayName is context.name, or name when context is empty.
func DisplayName(context, name string) string {
	if context == "" {
		return name
	}
	return context + "." + name
}

// NormalizeType maps user/config type names onto the canonical singular form.
// "functions" expands via ExpandTypes.
func NormalizeType(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "-")
	switch s {
	case "variable", "variables", "vari":
		return TypeVariable, true
	case "table", "tables", "tabi":
		return TypeTable, true
	case "schema", "schemas":
		return TypeSchema, true
	case "ai-function", "ai-functions", "ai", "functions/ai":
		return TypeAIFunction, true
	case "api-function", "api-functions", "api", "functions/api":
		return TypeAPIFunction, true
	case "client-function", "client-functions", "client", "functions/client":
		return TypeClientFunction, true
	case "server-function", "server-functions", "server", "functions/server":
		return TypeServerFunction, true
	case "webhook", "webhooks":
		return TypeWebhook, true
	case "trigger", "triggers":
		return TypeTrigger, true
	case "job", "jobs":
		return TypeJob, true
	case "snippet", "snippets":
		return TypeSnippet, true
	case "subscription", "subscriptions", "graphql-subscription", "graphql-subscriptions", "gql", "gql-subscription", "gql-subscriptions":
		return TypeSubscription, true
	case "application", "applications", "app", "apps", "canopy":
		return TypeApplication, true
	default:
		return "", false
	}
}

// ExpandTypes canonicalizes a user type list. Empty means all types.
func ExpandTypes(in []string) []string {
	if len(in) == 0 {
		return DeployOrder()
	}
	seen := map[string]bool{}
	var out []string
	add := func(t string) {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	for _, raw := range in {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			low := strings.ToLower(part)
			if low == "functions" || low == "function" {
				add(TypeAIFunction)
				add(TypeAPIFunction)
				add(TypeClientFunction)
				add(TypeServerFunction)
				continue
			}
			if t, ok := NormalizeType(part); ok {
				add(t)
			}
		}
	}
	return out
}

// Adapter is the language seam Glide calls for code modules.
type Adapter interface {
	Extract(file, typ string) (payload any, hash string, err error)
	Inspect(files []string) ([]delegate.InspectItem, error)
	Prepare(files []delegate.PrepareFile, disableDocs bool) (changed, skipped []string, err error)
}

// FunctionDescriber is host-side AI description generation (no adapter HTTP).
type FunctionDescriber interface {
	DescribeCustomFunction(kind string, payload any) (map[string]any, error)
}

// DelegateAdapter wraps a language delegate.
type DelegateAdapter struct {
	D delegate.Delegate
}

func (a DelegateAdapter) Extract(file, typ string) (any, string, error) {
	res, err := a.D.Extract(delegate.ExtractParams{File: file, Type: typ})
	if err != nil {
		return nil, "", err
	}
	return res.Payload, res.ContentHash, nil
}

func (a DelegateAdapter) Inspect(files []string) ([]delegate.InspectItem, error) {
	res, err := a.D.Inspect(delegate.InspectParams{Files: files})
	if err != nil {
		return nil, err
	}
	return res.Items, nil
}

func (a DelegateAdapter) Prepare(files []delegate.PrepareFile, disableDocs bool) ([]string, []string, error) {
	res, err := a.D.Prepare(delegate.PrepareParams{
		Files:       files,
		DisableDocs: disableDocs,
	})
	if err != nil {
		return nil, nil, err
	}
	return res.ChangedFiles, res.Skipped, nil
}

// Candidate is a file the host thinks might be a deployable.
type Candidate struct {
	Rel     string
	Abs     string
	Kind    string // json | code
	Guess   string // type guess from path / JSON
	Dialect Dialect
}

// Item is a loaded deployable (payload + hash).
type Item struct {
	Type       string
	Name       string
	Context    string
	File       string
	AbsFile    string
	Kind       string
	Payload    map[string]any
	Hash       string
	Unresolved []Token
	Receipt    *Receipt
	LoadError  string
}

func (i Item) Identity() string {
	return IdentityKey(i.Type, i.Context, i.Name)
}

func (i Item) Label() string {
	return DisplayName(i.Context, i.Name)
}

// Receipt is an optional per-instance deploy record.
type Receipt struct {
	Instance   string `json:"instance,omitempty"`
	ID         string `json:"id,omitempty"`
	Hash       string `json:"hash,omitempty"`
	DeployedAt string `json:"deployed_at,omitempty"`
	Type       string `json:"type,omitempty"`
	File       string `json:"file,omitempty"`
}

// Result is one plan/push/pull line.
type Result struct {
	Type    string
	Name    string
	Context string
	File    string
	Action  string
	ID      string
	Changed []string
	Error   string
}

func (r Result) Identity() string {
	return IdentityKey(r.Type, r.Context, r.Name)
}

func (r Result) Label() string {
	return DisplayName(r.Context, r.Name)
}

// Issue is one validate finding.
type Issue struct {
	Severity string
	Category string
	Path     string
	Identity string
	Message  string
}

// Report is the validate outcome.
type Report struct {
	Issues []Issue
	Items  []Item
	Eval   string // deploy eligibility message
}

func (r Report) Counts() (errors, warnings, infos int) {
	for _, i := range r.Issues {
		switch i.Severity {
		case SeverityError:
			errors++
		case SeverityWarn:
			warnings++
		default:
			infos++
		}
	}
	return
}

func (r Report) Failed(strict bool) bool {
	errN, warnN, _ := r.Counts()
	if errN > 0 {
		return true
	}
	return strict && warnN > 0
}
