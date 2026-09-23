package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/polyapi/polyglot/src/api"
	"github.com/polyapi/polyglot/src/glide"
	"github.com/spf13/cobra"
)

const snippetCollection = "snippets"

type snippetRef struct {
	ID          string
	Name        string
	Context     string
	Language    string
	Visibility  string
	Description string
	Code        string
	Spec        any
}

func addSnippetCommands(root *cobra.Command) {
	snippet := &cobra.Command{
		Use:   "snippet",
		Short: "Manage snippets",
		Example: examples(
			ex{"List snippets in a context:", "polyapi snippet list --context billing"},
			ex{"Scaffold a local snippet file:", "polyapi snippet init --name header"},
			ex{"Add a snippet from a file:", "polyapi snippet add header snippets/header.ts --context billing"},
			ex{"Get snippet source:", "polyapi snippet get billing.header --code"},
		),
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List snippets",
		Args:  cobra.NoArgs,
		RunE:  runSnippetList,
		Example: examples(
			ex{"List all snippets:", "polyapi snippet list"},
			ex{"Restrict to a context:", "polyapi snippet list --context billing"},
		),
	}
	list.Flags().String("context", "", "Restrict results to this context prefix")

	get := &cobra.Command{
		Use:   "get <id-or-name>",
		Short: "Get a snippet by ID, or by name with --context",
		Args:  cobra.ExactArgs(1),
		RunE:  runSnippetGet,
		Example: examples(
			ex{"Get by ID:", "polyapi snippet get abc123"},
			ex{"Get by context and name:", "polyapi snippet get header --context billing"},
			ex{"Print only the source:", "polyapi snippet get billing.header --code"},
		),
	}
	get.Flags().String("context", "", "Context of the snippet (required when looking up by name)")
	get.Flags().Bool("code", false, "Print only the snippet source")

	add := &cobra.Command{
		Use:     "add <name> <path>",
		Aliases: []string{"create"},
		Short:   "Add a snippet from a source file (create or replace)",
		Long:    "Upsert a snippet from a file (PUT /snippets), matching `npx poly snippet add`. Language is inferred from the file extension unless `--language` is set. `.ts`/`.tsx` are `typescript`; `.js` is `javascript`. Snippets are copy-paste source, not executed remotely.",
		Args:    cobra.ExactArgs(2),
		RunE:    runSnippetAdd,
		Example: examples(
			ex{"Add a TypeScript snippet:", "polyapi snippet add header snippets/header.ts --context billing"},
			ex{"Override the inferred language:", "polyapi snippet add note notes.md --context billing --language markdown"},
			ex{"Include a description:", "polyapi snippet add header snippets/header.ts --context billing --description \"Shared header\""},
		),
	}
	add.Flags().String("context", "", "Snippet context")
	add.Flags().String("description", "", "Description")
	add.Flags().String("language", "", "Language (lowercase). Inferred from the file extension when omitted")
	add.Flags().String("visibility", "ENVIRONMENT", "Visibility: PUBLIC, TENANT, or ENVIRONMENT (default ENVIRONMENT)")
	_ = add.MarkFlagRequired("context")

	update := &cobra.Command{
		Use:   "update <id-or-name>",
		Short: "Update a snippet by ID, or by name with --context",
		Args:  cobra.ExactArgs(1),
		RunE:  runSnippetUpdate,
		Example: examples(
			ex{"Replace the source:", "polyapi snippet update header --context billing --code-file snippets/header.ts"},
			ex{"Change the description:", "polyapi snippet update header --context billing --description \"Shared header\""},
		),
	}
	update.Flags().String("context", "", "Context of the snippet (required when looking up by name)")
	update.Flags().String("name", "", "New snippet name")
	update.Flags().String("new-context", "", "Move the snippet to this context")
	update.Flags().String("description", "", "Description")
	update.Flags().String("language", "", "Language (lowercase)")
	update.Flags().String("visibility", "", "Visibility: PUBLIC, TENANT, or ENVIRONMENT")
	update.Flags().String("code", "", "Replacement source")
	update.Flags().String("code-file", "", "Read replacement source from a file")

	del := &cobra.Command{
		Use:   "delete <id-or-name>",
		Short: "Delete a snippet by ID, or by name with --context",
		Args:  cobra.ExactArgs(1),
		RunE:  runSnippetDelete,
		Example: examples(
			ex{"Delete by ID:", "polyapi snippet delete abc123"},
			ex{"Delete by context and name:", "polyapi snippet delete header --context billing"},
		),
	}
	del.Flags().String("context", "", "Context of the snippet (required when looking up by name)")

	initCmd := newResourceInitCommand(resourceInitKind{
		Use: "snippet", Noun: "snippet", Type: glide.TypeSnippet, NeedsContext: true,
	})

	snippet.AddCommand(list, get, initCmd, add, update, del)
	root.AddCommand(snippet)
}

func runSnippetList(cmd *cobra.Command, _ []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	context, _ := cmd.Flags().GetString("context")
	items, err := client.ListAll(snippetCollection)
	if err != nil {
		return fail(err)
	}
	var rows []snippetRef
	for _, item := range items {
		ref := snippetFromItem(item)
		if context != "" && !strings.HasPrefix(ref.Context, context) {
			continue
		}
		rows = append(rows, ref)
	}
	sort.Slice(rows, func(i, j int) bool {
		li := glide.DisplayName(rows[i].Context, rows[i].Name)
		lj := glide.DisplayName(rows[j].Context, rows[j].Name)
		if li != lj {
			return li < lj
		}
		return rows[i].ID < rows[j].ID
	})
	out := cmd.OutOrStdout()
	if len(rows) == 0 {
		if !globalsFrom(cmd).Quiet {
			fmt.Fprintln(out, "no snippets found")
		}
		return nil
	}
	printSnippetList(out, rows)
	return nil
}

func printSnippetList(w io.Writer, rows []snippetRef) {
	nameW, langW, visW := len("NAME"), len("LANGUAGE"), len("VISIBILITY")
	for _, row := range rows {
		name := glide.DisplayName(row.Context, row.Name)
		if n := len(name); n > nameW {
			nameW = n
		}
		if n := len(snippetLanguageLabel(row.Language)); n > langW {
			langW = n
		}
		if n := len(visibilityLabel(row.Visibility)); n > visW {
			visW = n
		}
	}
	fmt.Fprintln(w, infoText(fmt.Sprintf("%-*s  %-*s  %-*s  %s", nameW, "NAME", langW, "LANGUAGE", visW, "VISIBILITY", "ID")))
	for _, row := range rows {
		fmt.Fprintf(w, "%-*s  %-*s  %-*s  %s\n", nameW, glide.DisplayName(row.Context, row.Name), langW, snippetLanguageLabel(row.Language), visW, visibilityLabel(row.Visibility), row.ID)
	}
}

func snippetLanguageLabel(v string) string {
	if v == "" {
		return "-"
	}
	return v
}

func runSnippetGet(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveSnippet(cmd, client, args[0])
	if err != nil {
		return err
	}
	codeOnly, _ := cmd.Flags().GetBool("code")
	if codeOnly {
		fmt.Fprint(cmd.OutOrStdout(), ref.Code)
		if ref.Code != "" && !strings.HasSuffix(ref.Code, "\n") {
			fmt.Fprintln(cmd.OutOrStdout())
		}
		return nil
	}
	return writeJSON(cmd.OutOrStdout(), ref.Spec)
}

func runSnippetAdd(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	name := strings.TrimSpace(args[0])
	if name == "" {
		return failUsage("name is required")
	}
	path := args[1]
	code, err := os.ReadFile(path)
	if err != nil {
		return failUsage(fmt.Sprintf("read %s: %v", path, err))
	}
	ctx, _ := cmd.Flags().GetString("context")
	lang, _ := cmd.Flags().GetString("language")
	if !cmd.Flags().Changed("language") {
		lang = snippetLanguageFromPath(path)
	}
	lang, err = parseSnippetLanguage(lang)
	if err != nil {
		return err
	}
	if lang == "" {
		return failUsage("could not infer language from the file extension; pass --language")
	}
	visRaw, _ := cmd.Flags().GetString("visibility")
	if visRaw == "" {
		visRaw = "ENVIRONMENT"
	}
	vis, err := parseVisibility(visRaw)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"name":       name,
		"context":    ctx,
		"code":       string(code),
		"language":   lang,
		"visibility": vis,
	}
	if cmd.Flags().Changed("description") {
		desc, _ := cmd.Flags().GetString("description")
		payload["description"] = desc
	}
	created, err := client.Put(snippetCollection, payload)
	if err != nil {
		return fail(err)
	}
	id := firstNonEmpty(mapString(created, "id"))
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("added snippet %s (%s)", glide.DisplayName(ctx, name), id))
	}
	return nil
}

func runSnippetUpdate(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveSnippet(cmd, client, args[0])
	if err != nil {
		return err
	}
	payload, err := snippetUpdatePayload(cmd)
	if err != nil {
		return err
	}
	if len(payload) == 0 {
		return failUsage("pass at least one field to update")
	}
	updated, err := client.Update(snippetCollection, ref.ID, payload, nil)
	if err != nil {
		return fail(err)
	}
	id := firstNonEmpty(mapString(updated, "id"), ref.ID)
	name := firstNonEmpty(mapString(updated, "name"), mapString(payload, "name"), ref.Name)
	ctx := firstNonEmpty(mapString(updated, "context"), mapString(payload, "context"), ref.Context)
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("updated snippet %s (%s)", glide.DisplayName(ctx, name), id))
	}
	return nil
}

func snippetUpdatePayload(cmd *cobra.Command) (map[string]any, error) {
	payload := map[string]any{}
	if cmd.Flags().Changed("name") {
		name, _ := cmd.Flags().GetString("name")
		if name != "" {
			payload["name"] = name
		}
	}
	if cmd.Flags().Changed("new-context") {
		ctx, _ := cmd.Flags().GetString("new-context")
		payload["context"] = ctx
	}
	if cmd.Flags().Changed("description") {
		desc, _ := cmd.Flags().GetString("description")
		payload["description"] = desc
	}
	if cmd.Flags().Changed("visibility") {
		raw, _ := cmd.Flags().GetString("visibility")
		vis, err := parseVisibility(raw)
		if err != nil {
			return nil, err
		}
		payload["visibility"] = vis
	}
	if cmd.Flags().Changed("code") && cmd.Flags().Changed("code-file") {
		return nil, failUsage("pass only one of --code or --code-file")
	}
	codePath := ""
	if cmd.Flags().Changed("code-file") {
		path, _ := cmd.Flags().GetString("code-file")
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, failUsage(fmt.Sprintf("read %s: %v", path, err))
		}
		payload["code"] = string(b)
		codePath = path
	} else if cmd.Flags().Changed("code") {
		code, _ := cmd.Flags().GetString("code")
		payload["code"] = code
	}
	if cmd.Flags().Changed("language") {
		raw, _ := cmd.Flags().GetString("language")
		lang, err := parseSnippetLanguage(raw)
		if err != nil {
			return nil, err
		}
		if lang == "" {
			return nil, failUsage("language is required")
		}
		payload["language"] = lang
	} else if codePath != "" {
		if lang := snippetLanguageFromPath(codePath); lang != "" {
			payload["language"] = lang
		}
	}
	return payload, nil
}

func runSnippetDelete(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveSnippet(cmd, client, args[0])
	if err != nil {
		return err
	}
	if err := client.Delete(snippetCollection, ref.ID, nil); err != nil {
		return fail(err)
	}
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("deleted snippet %s (%s)", glide.DisplayName(ref.Context, ref.Name), ref.ID))
	}
	return nil
}

func resolveSnippet(cmd *cobra.Command, client *api.HTTPClient, idOrName string) (snippetRef, error) {
	context, _ := cmd.Flags().GetString("context")
	if context != "" {
		return lookupSnippetByName(client, context, idOrName)
	}
	if !looksLikeUUID(idOrName) {
		if ctx, name, ok := splitContextName(idOrName); ok {
			return lookupSnippetByName(client, ctx, name)
		}
	}
	return lookupSnippetByID(client, idOrName)
}

// fetchSnippetByRef loads a snippet for `init --snippet`. The init command's
// --context is the new resource context, so it is not used here: pass an id,
// context.name, or a name that is unique on the instance.
func fetchSnippetByRef(client *api.HTTPClient, ref string) (snippetRef, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return snippetRef{}, failUsage("snippet reference is empty")
	}
	if looksLikeUUID(ref) {
		return lookupSnippetByID(client, ref)
	}
	if ctx, name, ok := splitContextName(ref); ok {
		return lookupSnippetByName(client, ctx, name)
	}
	return lookupSnippetByUniqueName(client, ref)
}

func lookupSnippetByUniqueName(client *api.HTTPClient, name string) (snippetRef, error) {
	items, err := client.ListAll(snippetCollection)
	if err != nil {
		return snippetRef{}, fail(err)
	}
	var sameCase []snippetRef
	var anyCase []snippetRef
	for _, item := range items {
		ref := snippetFromItem(item)
		if !strings.EqualFold(ref.Name, name) {
			continue
		}
		anyCase = append(anyCase, ref)
		if ref.Name == name {
			sameCase = append(sameCase, ref)
		}
	}
	matches := sameCase
	if len(matches) == 0 {
		matches = anyCase
	}
	switch len(matches) {
	case 0:
		return snippetRef{}, failUsage(fmt.Sprintf("no snippet named %q", name))
	case 1:
		ref := matches[0]
		if ref.ID == "" {
			if strings.TrimSpace(ref.Code) == "" {
				return snippetRef{}, fail(fmt.Errorf("snippet %s has no id; cannot fetch code", glide.DisplayName(ref.Context, ref.Name)))
			}
			return ref, nil
		}
		spec, err := client.Get(snippetCollection, ref.ID)
		if err != nil {
			return snippetRef{}, fail(err)
		}
		full := snippetFromItem(spec)
		full.ID = ref.ID
		full.Spec = spec
		return full, nil
	default:
		parts := make([]string, 0, len(matches))
		for _, m := range matches {
			parts = append(parts, glide.DisplayName(m.Context, m.Name))
		}
		sort.Strings(parts)
		return snippetRef{}, failUsage(fmt.Sprintf("unclear snippet reference %q; matches %d snippets (%s); pass context.name or an id", name, len(matches), strings.Join(parts, ", ")))
	}
}

func lookupSnippetByID(client *api.HTTPClient, id string) (snippetRef, error) {
	spec, err := client.Get(snippetCollection, id)
	if err != nil {
		return snippetRef{}, fail(err)
	}
	ref := snippetFromItem(spec)
	if ref.ID == "" {
		ref.ID = id
	}
	ref.Spec = spec
	return ref, nil
}

func lookupSnippetByName(client *api.HTTPClient, context, name string) (snippetRef, error) {
	items, err := client.ListAll(snippetCollection)
	if err != nil {
		return snippetRef{}, fail(err)
	}
	var sameCase []snippetRef
	var anyCase []snippetRef
	for _, item := range items {
		ref := snippetFromItem(item)
		if !strings.EqualFold(ref.Name, name) || !strings.EqualFold(ref.Context, context) {
			continue
		}
		anyCase = append(anyCase, ref)
		if ref.Name == name && ref.Context == context {
			sameCase = append(sameCase, ref)
		}
	}
	matches := sameCase
	if len(matches) == 0 {
		matches = anyCase
	}
	switch len(matches) {
	case 0:
		return snippetRef{}, failUsage(fmt.Sprintf("no snippet named %q in context %q", name, context))
	case 1:
		ref := matches[0]
		if ref.ID == "" {
			return ref, nil
		}
		spec, err := client.Get(snippetCollection, ref.ID)
		if err != nil {
			return snippetRef{}, fail(err)
		}
		full := snippetFromItem(spec)
		full.ID = ref.ID
		full.Spec = spec
		return full, nil
	default:
		return snippetRef{}, failUsage(fmt.Sprintf("unclear snippet reference %s; matches %d snippets", glide.DisplayName(context, name), len(matches)))
	}
}

func snippetFromItem(item any) snippetRef {
	m, _ := item.(map[string]any)
	return snippetRef{
		ID:          firstMapString(m, "id"),
		Name:        mapString(m, "name"),
		Context:     mapString(m, "context"),
		Language:    mapString(m, "language"),
		Visibility:  mapString(m, "visibility"),
		Description: mapString(m, "description"),
		Code:        mapString(m, "code"),
		Spec:        item,
	}
}

func snippetLanguageFromPath(path string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	switch ext {
	case "ts", "tsx", "mts", "cts":
		return "typescript"
	case "js", "mjs", "cjs", "jsx":
		return "javascript"
	case "py", "pyi":
		return "python"
	case "java":
		return "java"
	case "go":
		return "go"
	case "json":
		return "json"
	case "yaml", "yml":
		return "yaml"
	case "md", "markdown":
		return "markdown"
	case "html", "htm":
		return "html"
	case "css":
		return "css"
	case "sql":
		return "sql"
	case "xml":
		return "xml"
	case "txt":
		return "text"
	case "sh", "bash":
		return "bash"
	case "rb":
		return "ruby"
	case "rs":
		return "rust"
	case "kt":
		return "kotlin"
	default:
		return ""
	}
}

func parseSnippetLanguage(s string) (string, error) {
	lang := strings.ToLower(strings.TrimSpace(s))
	if lang == "" {
		return "", nil
	}
	return lang, nil
}
