package glide

import (
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/polyapi/polyglot/src/api"
	"github.com/polyapi/polyglot/src/delegate"
	"github.com/polyapi/polyglot/src/exitcode"
)

// Engine is one Glide run.
type Engine struct {
	Root            string
	PolyPath        string
	Lang            delegate.Language
	Client          api.Client
	Adapter         Adapter
	Describer       FunctionDescriber
	Env             map[string]string
	Scope           Scope
	Instance        string
	Receipts        bool
	Force           bool
	DeleteOrphans   bool
	Resume          bool
	AllowUnresolved bool
	Online          bool
	Strict          bool
	Only            []string
	DisableAI       bool
	DisableDocs     bool
	DryRun          bool
	Yes             bool
	NonInteractive  bool
	Production      bool
	PushAllowed     bool
	EvalStatus      string
	EvalMessage     string
	HasCredentials  bool
	Verbose         int
	Stdin           io.Reader
	Stderr          io.Writer
}

// Prepare always re-scans, then asks the adapter to rewrite code candidates.
// JSONC is a no-op. Exit code 1 when files changed (hook-friendly).
func (e *Engine) Prepare() (changed, skipped []string, err error) {
	cands, err := Find(e.Root, e.Scope, e.Lang)
	if err != nil {
		return nil, nil, wrap("discover", err)
	}
	if len(cands) == 0 {
		return nil, nil, e.nothingFound()
	}
	var code []string
	for _, c := range cands {
		if c.Kind == KindCode {
			code = append(code, c.Rel)
		} else {
			skipped = append(skipped, c.Rel)
		}
	}
	if len(code) == 0 {
		return nil, skipped, nil
	}
	if e.Adapter == nil {
		return nil, skipped, failCode(exitcode.AdapterMissing, "code deployables require a language adapter for prepare; install the project SDK or pass --adapter")
	}
	if e.DisableDocs {
		return nil, append(skipped, code...), nil
	}
	items, err := e.Adapter.Inspect(code)
	if err != nil {
		return nil, skipped, err
	}
	files, err := e.prepareFiles(items)
	if err != nil {
		return nil, skipped, err
	}
	changed, skipMore, err := e.Adapter.Prepare(files, false)
	if err != nil {
		return changed, skipped, err
	}
	skipped = append(skipped, skipMore...)
	if len(changed) > 0 {
		return changed, skipped, failCode(exitcode.Failure, "prepare updated "+strconv.Itoa(len(changed))+" file(s); review the diff and commit")
	}
	return changed, skipped, nil
}

func (e *Engine) prepareFiles(items []delegate.InspectItem) ([]delegate.PrepareFile, error) {
	out := make([]delegate.PrepareFile, 0, len(items))
	for _, it := range items {
		docs := it.Docs()
		if !e.DisableAI && e.Describer != nil && !it.DisableAI && it.NeedsDescription() {
			payload := map[string]any{
				"description": it.Description,
				"arguments":   inspectArgsPayload(it.Arguments),
				"code":        it.Code,
			}
			ai, err := e.Describer.DescribeCustomFunction(it.Type, payload)
			if err == nil && ai != nil {
				docs = mergePrepareDocs(docs, ai)
			}
		}
		d := docs
		out = append(out, delegate.PrepareFile{File: it.File, Docs: &d})
	}
	return out, nil
}

func inspectArgsPayload(args []delegate.InspectArg) []map[string]any {
	out := make([]map[string]any, 0, len(args))
	for _, a := range args {
		out = append(out, map[string]any{
			"name":        a.Name,
			"type":        a.Type,
			"description": a.Description,
		})
	}
	return out
}

func mergePrepareDocs(docs delegate.PrepareDocs, ai map[string]any) delegate.PrepareDocs {
	if s, ok := ai["description"].(string); ok && strings.TrimSpace(s) != "" && strings.TrimSpace(docs.Description) == "" {
		docs.Description = s
	}
	aiArgs, _ := ai["arguments"].([]any)
	if len(aiArgs) == 0 {
		return docs
	}
	byName := map[string]string{}
	for _, raw := range aiArgs {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		desc, _ := m["description"].(string)
		if name != "" && desc != "" {
			byName[name] = desc
		}
	}
	for i, a := range docs.Arguments {
		if strings.TrimSpace(a.Description) != "" {
			continue
		}
		if d, ok := byName[a.Name]; ok {
			docs.Arguments[i].Description = d
		}
	}
	return docs
}

// DisableAIFromEnv is true when DISABLE_AI is set.
func DisableAIFromEnv() bool {
	v := strings.TrimSpace(os.Getenv("DISABLE_AI"))
	return v != "" && v != "0" && strings.ToLower(v) != "false"
}
