package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/polyapi/polyglot/src/delegate"
	"github.com/polyapi/polyglot/src/glide"
	"github.com/spf13/cobra"
)

type initTypeMode int

const (
	initTypeNone initTypeMode = iota
	initTypeFunction
	initTypeTrigger
	initTypeSubscription
)

type resourceInitKind struct {
	// Use is the command path segment (vari, table, function, …).
	Use string
	// Noun is the human name in help ("variable", "table").
	Noun string
	// Type is the canonical Glide type. Empty when TypeMode is initTypeFunction.
	Type string
	// NeedsContext is true when the platform resource has a context field.
	NeedsContext bool
	// TypeMode selects the required --type flag (function, trigger, or none).
	TypeMode initTypeMode
}

func newResourceInitCommand(k resourceInitKind) *cobra.Command {
	prefix := "polyapi " + k.Use
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a local " + k.Noun + " file with field placeholders",
		Long:  resourceInitLong(k),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runResourceInit(cmd, k)
		},
		Example: resourceInitExamples(k, prefix),
	}
	cmd.Flags().String("name", "", k.Noun+" name")
	_ = cmd.MarkFlagRequired("name")
	if k.NeedsContext {
		cmd.Flags().String("context", "", k.Noun+" context (optional; used in the default path)")
	}
	switch k.TypeMode {
	case initTypeFunction:
		cmd.Flags().Var(&functionTypeValue{}, "type", "Function type (server|client|api|ai)")
		_ = cmd.MarkFlagRequired("type")
	case initTypeTrigger:
		cmd.Flags().Var(&triggerTypeValue{}, "type", "Trigger source (webhook|error-handler)")
		_ = cmd.MarkFlagRequired("type")
	case initTypeSubscription:
		cmd.Flags().Var(&subscriptionTypeValue{}, "type", "Subscription type (CUSTOM|OHIP)")
		_ = cmd.MarkFlagRequired("type")
	}
	pathHelp := "File or directory to write (default: src/<context>/artifacts/…, or src/artifacts/… without --context)"
	if k.TypeMode == initTypeFunction {
		pathHelp = "File or directory to write (default: src/<context>/server|<client>/<name>.ts, or .py with --lang python)"
	}
	cmd.Flags().String("path", "", pathHelp)
	cmd.Flags().Bool("force", false, "Overwrite the file if it already exists")
	cmd.Flags().String("snippet", "", "PolyAPI snippet to copy (id, context.name, or unique name). Fetched and used as the file contents; --name and --context overwrite those in the snippet")
	return cmd
}

func resourceInitLong(k resourceInitKind) string {
	if k.TypeMode == initTypeFunction {
		return "Write a local function file. `--type server` and `--type client` produce a TypeScript or Python module with a typed polyConfig and a function whose identifier is exactly `--name` (the same string as polyConfig.name). `--name` must be a valid identifier for the language — not a reserved word, and not punctuation such as hyphens. Language is `--lang`, else the `--path` extension, else the project (default typescript). `--type api` and `--type ai` still write JSONC. `--snippet` fetches a PolyAPI snippet and uses its code as the file (name and context from this command overwrite those in the snippet); the snippet language must match the file being generated. Without `--snippet` this does not call the API."
	}
	s := "Write a local JSONC artifact with placeholders for every " + k.Noun + " field. Edit the file, then `polyapi deploy plan` / `push`. Without `--snippet` this does not call the API — use `" + k.Use + " create` (or `add`) to create the resource immediately on the instance. `--snippet` fetches a PolyAPI snippet and uses its code as the file (name and context from this command overwrite those in the snippet); the snippet must be JSON/JSONC."
	if k.TypeMode == initTypeTrigger {
		s += " `--name` and `--type webhook|error-handler` are required. `--snippet` must match `--type` (webhook handle vs error-handler path)."
	} else if k.TypeMode == initTypeSubscription {
		s += " `--name` and `--type CUSTOM|OHIP` are required. `--snippet` must match `--type` when the snippet JSON has a type field."
	} else {
		s += " Only `--name` is required."
	}
	if k.NeedsContext {
		s += " `--context` is optional; without it the file lands under `src/artifacts/" + glide.ArtifactDir(k.Type) + "/`."
	} else {
		s += " " + strings.TrimSpace(strings.ToUpper(k.Noun[:1])+k.Noun[1:]) + "s have no context; the default path is `src/artifacts/" + glide.ArtifactDir(k.Type) + "/`."
	}
	return s
}

func resourceInitExamples(k resourceInitKind, prefix string) string {
	switch {
	case k.TypeMode == initTypeFunction:
		return examples(
			ex{"Scaffold a TypeScript server function:", prefix + " init --name helloWorld --type server"},
			ex{"Python, with a context:", prefix + " init --name hello_world --type server --lang python --context billing"},
			ex{"From a PolyAPI snippet:", prefix + " init --name helloWorld --type server --snippet templates.helloWorld"},
			ex{"Client function:", prefix + " init --name greet --type client"},
		)
	case k.TypeMode == initTypeTrigger:
		return examples(
			ex{"Webhook trigger:", prefix + " init --name weekly --type webhook"},
			ex{"Error-handler trigger:", prefix + " init --name on-error --type error-handler"},
			ex{"From a PolyAPI snippet:", prefix + " init --name weekly --type webhook --snippet templates.example"},
			ex{"Write to an explicit path:", prefix + " init --name weekly --type webhook --path ./src/artifacts/triggers/weekly.jsonc"},
		)
	case k.TypeMode == initTypeSubscription:
		return examples(
			ex{"CUSTOM subscription:", prefix + " init --name ordersStream --type CUSTOM"},
			ex{"OHIP subscription with a context:", prefix + " init --name operaEvents --type OHIP --context opera"},
			ex{"From a PolyAPI snippet:", prefix + " init --name ordersStream --type CUSTOM --snippet templates.example"},
			ex{"Write to an explicit path:", prefix + " init --name ordersStream --type CUSTOM --path ./src/artifacts/subscriptions/ordersStream.jsonc"},
		)
	case !k.NeedsContext:
		return examples(
			ex{"Scaffold a " + k.Noun + ":", prefix + " init --name example"},
			ex{"From a PolyAPI snippet:", prefix + " init --name example --snippet templates.example"},
			ex{"Write to an explicit path:", prefix + " init --name example --path ./src/artifacts/" + glide.ArtifactDir(k.Type) + "/example.jsonc"},
		)
	default:
		return examples(
			ex{"Scaffold a " + k.Noun + ":", prefix + " init --name example"},
			ex{"Set a context (default path uses it):", prefix + " init --name example --context billing"},
			ex{"From a PolyAPI snippet:", prefix + " init --name example --snippet templates.example --context billing"},
		)
	}
}

func runResourceInit(cmd *cobra.Command, k resourceInitKind) error {
	name, _ := cmd.Flags().GetString("name")
	name = strings.TrimSpace(name)
	if err := validateInitName(name); err != nil {
		return failUsage(err.Error())
	}
	context := ""
	if k.NeedsContext {
		context, _ = cmd.Flags().GetString("context")
		context = strings.TrimSpace(context)
		if context != "" {
			if err := validateInitContext(context); err != nil {
				return failUsage(err.Error())
			}
		}
	}
	if k.TypeMode == initTypeSubscription {
		if err := glide.ValidSubscriptionName(name); err != nil {
			return failUsage(err.Error())
		}
	}
	typ := k.Type
	if k.TypeMode == initTypeFunction {
		raw := initTypeFlag(cmd)
		mapped, ok := glide.NormalizeType(raw)
		if !ok || !glide.IsFunction(mapped) {
			return failUsage(fmt.Sprintf("invalid function type %q (server|client|api|ai)", raw))
		}
		typ = mapped
	}

	pathFlag, _ := cmd.Flags().GetString("path")
	pathFlag = strings.TrimSpace(pathFlag)
	snippetRef, _ := cmd.Flags().GetString("snippet")
	snippetRef = strings.TrimSpace(snippetRef)

	var body, rel string
	var err error
	if typ == glide.TypeServerFunction || typ == glide.TypeClientFunction {
		var lang string
		lang, err = resolveInitLanguage(cmd, pathFlag)
		if err != nil {
			return err
		}
		if err := glide.ValidFunctionName(lang, name); err != nil {
			return failUsage(err.Error())
		}
		if snippetRef != "" {
			body, err = bodyFromSnippet(cmd, typ, name, context, lang, k.Noun, snippetRef)
		} else {
			body, err = glide.ScaffoldFunctionCode(typ, name, context, lang)
			if err != nil {
				err = fail(err)
			}
		}
		if err != nil {
			return err
		}
		rel, err = resolveInitPath(pathFlag, typ, context, name, glide.FunctionCodeExt(lang))
		if err != nil {
			return failUsage(err.Error())
		}
		ext := strings.ToLower(filepath.Ext(rel))
		if ext == ".json" || ext == ".jsonc" {
			return failUsage("server and client functions are TypeScript or Python source, not JSONC; pass a .ts or .py path")
		}
	} else {
		if snippetRef != "" {
			body, err = bodyFromSnippet(cmd, typ, name, context, "json", k.Noun, snippetRef)
			if err == nil && k.TypeMode == initTypeTrigger {
				if err = glide.CheckTriggerJSONCSource(body, initTypeFlag(cmd)); err != nil {
					err = failUsage(err.Error())
				}
			}
			if err == nil && k.TypeMode == initTypeSubscription {
				if err = glide.CheckSubscriptionJSONCType(body, initTypeFlag(cmd)); err != nil {
					err = failUsage(err.Error())
				}
			}
		} else if k.TypeMode == initTypeTrigger {
			body, err = glide.ScaffoldTriggerJSONC(name, initTypeFlag(cmd))
			if err != nil {
				err = failUsage(err.Error())
			}
		} else if k.TypeMode == initTypeSubscription {
			body, err = glide.ScaffoldSubscriptionJSONC(name, context, initTypeFlag(cmd))
			if err != nil {
				err = failUsage(err.Error())
			}
		} else {
			body, err = glide.ScaffoldJSONC(typ, name, context)
			if err != nil {
				err = fail(err)
			}
		}
		if err != nil {
			return err
		}
		rel, err = resolveInitPath(pathFlag, typ, context, name, ".jsonc")
		if err != nil {
			return failUsage(err.Error())
		}
	}

	cwd := projectRoot()
	abs := rel
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(cwd, rel)
	}
	abs, err = filepath.Abs(abs)
	if err != nil {
		return fail(err)
	}

	force, _ := cmd.Flags().GetBool("force")
	if st, err := os.Stat(abs); err == nil {
		if st.IsDir() {
			return failUsage(fmt.Sprintf("%s is a directory; pass a file path or omit --path", displayInitPath(cwd, abs)))
		}
		if !force {
			return fail(fmt.Errorf("%s already exists; pass --force to overwrite", displayInitPath(cwd, abs)))
		}
	} else if !os.IsNotExist(err) {
		return fail(err)
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
		return fail(err)
	}

	shown := displayInitPath(cwd, abs)
	out := cmd.OutOrStdout()
	if globalsFrom(cmd).Quiet {
		fmt.Fprintln(out, shown)
		return nil
	}
	printOk(out, "created "+shown)
	return nil
}

func bodyFromSnippet(cmd *cobra.Command, typ, name, context, targetLang, noun, ref string) (string, error) {
	client, err := apiClient(cmd)
	if err != nil {
		return "", err
	}
	snip, err := fetchSnippetByRef(client, ref)
	if err != nil {
		return "", err
	}
	label := snippetInitLabel(snip, ref)
	got := glide.NormalizeSnippetLanguage(snip.Language)
	want := glide.TargetInitLanguage(typ, targetLang)
	if got == "" {
		return "", fail(fmt.Errorf("snippet %s has no language; expected %s for this %s", label, want, noun))
	}
	if !glide.SnippetLanguageMatches(got, want) {
		return "", fail(fmt.Errorf("snippet %s is %s, not %s required for this %s", label, got, want, noun))
	}
	if strings.TrimSpace(snip.Code) == "" {
		return "", fail(fmt.Errorf("snippet %s has no code", label))
	}
	if typ == glide.TypeServerFunction || typ == glide.TypeClientFunction {
		return glide.OverlayFunctionIdentity(snip.Code, targetLang, name, context, snip.Name), nil
	}
	out, err := glide.OverlayJSONCIdentity(snip.Code, typ, name, context)
	if err != nil {
		return "", fail(err)
	}
	return out, nil
}

func initTypeFlag(cmd *cobra.Command) string {
	if f := cmd.Flags().Lookup("type"); f != nil {
		return f.Value.String()
	}
	return ""
}

func snippetInitLabel(snip snippetRef, ref string) string {
	if snip.Name != "" {
		return glide.DisplayName(snip.Context, snip.Name)
	}
	if snip.ID != "" {
		return snip.ID
	}
	return ref
}

func validateInitName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("name %q is not a valid file name", name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("name must not contain path separators")
	}
	return nil
}

func validateInitContext(context string) error {
	for _, part := range strings.Split(context, ".") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("context %q is not a valid path", context)
		}
		if strings.ContainsAny(part, `/\`) {
			return fmt.Errorf("context must not contain path separators")
		}
	}
	return nil
}

func resolveInitPath(path, typ, context, name, ext string) (string, error) {
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	if path == "" {
		if ext == ".jsonc" || ext == ".json" {
			return glide.ArtifactRel(typ, context, name), nil
		}
		return glide.FunctionCodeRel(typ, context, name, ext), nil
	}
	got := strings.ToLower(filepath.Ext(path))
	switch got {
	case ".json", ".jsonc", ".ts", ".tsx", ".js", ".jsx", ".mts", ".cts", ".mjs", ".cjs", ".py":
		return filepath.ToSlash(path), nil
	}
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return filepath.ToSlash(filepath.Join(path, name+ext)), nil
	}
	if got == "" {
		return filepath.ToSlash(filepath.Join(path, name+ext)), nil
	}
	return filepath.ToSlash(path), nil
}

func resolveInitLanguage(cmd *cobra.Command, path string) (string, error) {
	extLang := languageFromExt(filepath.Ext(path))
	flagLang := globalsFrom(cmd).Lang
	if flagLang == "java" {
		return "", failUsage("function init supports typescript and python; pass --lang typescript or --lang python")
	}
	if flagLang != "" && extLang != "" && flagLang != extLang {
		return "", failUsage(fmt.Sprintf("--lang %s does not match path extension %s", flagLang, filepath.Ext(path)))
	}
	if flagLang != "" {
		return flagLang, nil
	}
	if extLang != "" {
		return extLang, nil
	}
	g := globalsFrom(cmd)
	if cfg, err := loadFromGlobal(g); err == nil {
		if lang, ok := delegate.ParseLanguage(cfg.Language); ok {
			if lang == delegate.LangTypeScript || lang == delegate.LangPython {
				return string(lang), nil
			}
		}
	}
	loc := delegate.LocateProject(projectRoot(), g.PolyPath)
	var found []string
	for _, l := range loc.Languages {
		if l == delegate.LangTypeScript || l == delegate.LangPython {
			found = append(found, string(l))
		}
	}
	switch len(found) {
	case 0:
		return "typescript", nil
	case 1:
		return found[0], nil
	default:
		return "", failUsage("multiple languages detected (" + strings.Join(found, ", ") + "); pass --lang typescript|python")
	}
}

func languageFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".py":
		return "python"
	case ".ts", ".tsx", ".js", ".jsx", ".mts", ".cts", ".mjs", ".cjs":
		return "typescript"
	default:
		return ""
	}
}

func displayInitPath(cwd, abs string) string {
	rel, err := filepath.Rel(cwd, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(abs)
	}
	return filepath.ToSlash(rel)
}
