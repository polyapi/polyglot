package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/polyapi/polyglot/src/api"
	"github.com/polyapi/polyglot/src/glide"
	"github.com/spf13/cobra"
)

const (
	variCollection = "variables"
	redactedMark   = "********"
)

type variRef struct {
	ID         string
	Name       string
	Context    string
	Visibility string
	Secrecy    string
	Secret     bool
	Spec       any
}

func addVariCommands(root *cobra.Command) {
	vari := &cobra.Command{
		Use:   "vari",
		Short: "Manage variables",
		Example: examples(
			ex{"List variables in a context:", "polyapi vari list --context billing"},
			ex{"Scaffold a local variable file:", "polyapi vari init --name apiKey"},
			ex{"Create a variable:", `polyapi vari create --name apiKey --context billing --value '"sk-..."'`},
			ex{"Copy into another context:", "polyapi vari copy apiKey --context billing --to-context staging"},
		),
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List variables",
		Args:  cobra.NoArgs,
		RunE:  runVariList,
		Example: examples(
			ex{"List all variables:", "polyapi vari list"},
			ex{"Restrict to a context:", "polyapi vari list --context billing"},
		),
	}
	list.Flags().String("context", "", "Restrict results to this context prefix")

	get := &cobra.Command{
		Use:   "get <id-or-name>",
		Short: "Get a variable by ID, or by name with --context",
		Args:  cobra.ExactArgs(1),
		RunE:  runVariGet,
		Example: examples(
			ex{"Get by ID:", "polyapi vari get abc123"},
			ex{"Get by context and name:", "polyapi vari get apiKey --context billing"},
			ex{"Print only the value (non-secret):", "polyapi vari get apiKey --context billing --value"},
		),
	}
	get.Flags().String("context", "", "Context of the variable (required when looking up by name)")
	get.Flags().Bool("value", false, "Print only the stored value (not for SECRET variables)")

	create := &cobra.Command{
		Use:   "create",
		Short: "Create a variable",
		Args:  cobra.NoArgs,
		RunE:  runVariCreate,
		Example: examples(
			ex{"Create a string variable:", `polyapi vari create --name apiKey --context billing --value '"sk-live"'`},
			ex{"Create a JSON object:", `polyapi vari create --name config --context billing --value '{"region":"us"}'`},
			ex{"Create a secret:", `polyapi vari create --name apiKey --context billing --value '"sk-live"' --secret`},
		),
	}
	addVariWriteFlags(create, true)
	_ = create.MarkFlagRequired("name")
	_ = create.MarkFlagRequired("context")
	create.MarkFlagsOneRequired("value", "value-file")

	update := &cobra.Command{
		Use:   "update <id-or-name>",
		Short: "Update a variable by ID, or by name with --context",
		Args:  cobra.ExactArgs(1),
		RunE:  runVariUpdate,
		Example: examples(
			ex{"Update the value:", `polyapi vari update apiKey --context billing --value '"sk-new"'`},
			ex{"Rename:", "polyapi vari update apiKey --context billing --name stripeKey"},
		),
	}
	update.Flags().String("context", "", "Context of the variable (required when looking up by name)")
	addVariWriteFlags(update, false)
	update.Flags().String("new-context", "", "Move the variable to this context")
	update.Flags().String("otp", "", "One-time password when the instance requires MFA for updates")

	del := &cobra.Command{
		Use:   "delete <id-or-name>",
		Short: "Delete a variable by ID, or by name with --context",
		Args:  cobra.ExactArgs(1),
		RunE:  runVariDelete,
		Example: examples(
			ex{"Delete by ID:", "polyapi vari delete abc123"},
			ex{"Delete by context and name:", "polyapi vari delete apiKey --context billing"},
		),
	}
	del.Flags().String("context", "", "Context of the variable (required when looking up by name)")
	del.Flags().String("otp", "", "One-time password when the instance requires MFA for deletes")

	copyCmd := &cobra.Command{
		Use:   "copy <id-or-name>",
		Short: "Copy a variable to a new name and/or context",
		Args:  cobra.ExactArgs(1),
		RunE:  runVariCopy,
		Example: examples(
			ex{"Copy into another context:", "polyapi vari copy apiKey --context billing --to-context staging"},
			ex{"Copy a secret with a new value:", `polyapi vari copy apiKey --context billing --to-context staging --value '"sk-staging"'`},
		),
	}
	copyCmd.Flags().String("context", "", "Context of the source variable (required when looking up by name)")
	copyCmd.Flags().String("to-name", "", "Name for the copy (default: source name)")
	copyCmd.Flags().String("to-context", "", "Context for the copy (default: source context)")
	copyCmd.Flags().String("value", "", "Value for the copy (required when the source is SECRET)")
	copyCmd.Flags().String("value-file", "", "Read the copy value from a file")
	copyCmd.Flags().String("description", "", "Description for the copy (default: source description)")
	copyCmd.Flags().String("visibility", "", "Visibility for the copy (default: source visibility)")
	copyCmd.Flags().Bool("secret", false, "Store the copy as SECRET")
	copyCmd.Flags().String("secrecy", "", "Secrecy for the copy: NONE, SECRET, or OBSCURED")
	copyCmd.Flags().String("expires-at", "", "Expiration timestamp (ISO-8601)")

	initCmd := newResourceInitCommand(resourceInitKind{
		Use: "vari", Noun: "variable", Type: glide.TypeVariable, NeedsContext: true,
	})

	vari.AddCommand(list, get, initCmd, create, update, del, copyCmd)
	root.AddCommand(vari)
}

func addVariWriteFlags(cmd *cobra.Command, creating bool) {
	if creating {
		cmd.Flags().String("name", "", "Variable name")
		cmd.Flags().String("context", "", "Variable context")
	} else {
		cmd.Flags().String("name", "", "New variable name")
	}
	cmd.Flags().String("value", "", "Variable value (JSON, or a raw string)")
	cmd.Flags().String("value-file", "", "Read the value from a file")
	cmd.Flags().String("description", "", "Description")
	cmd.Flags().Bool("secret", false, "Store as a SECRET variable")
	cmd.Flags().String("secrecy", "", "Secrecy: NONE, SECRET, or OBSCURED")
	visDef := ""
	visHelp := "Visibility: PUBLIC, TENANT, or ENVIRONMENT"
	if creating {
		visDef = "ENVIRONMENT"
		visHelp += " (default ENVIRONMENT)"
	}
	cmd.Flags().String("visibility", visDef, visHelp)
	cmd.Flags().String("expires-at", "", "Expiration timestamp (ISO-8601)")
}

func runVariList(cmd *cobra.Command, _ []string) error {
	client, err := variClient(cmd)
	if err != nil {
		return err
	}
	context, _ := cmd.Flags().GetString("context")
	items, err := client.ListAll(variCollection)
	if err != nil {
		return fail(err)
	}
	var rows []variRef
	for _, item := range items {
		ref := variFromItem(item)
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
			fmt.Fprintln(out, "no variables found")
		}
		return nil
	}
	printVariList(out, rows)
	return nil
}

func printVariList(w io.Writer, rows []variRef) {
	nameW, visW := len("NAME"), len("VISIBILITY")
	for _, row := range rows {
		name := glide.DisplayName(row.Context, row.Name)
		if n := len(name); n > nameW {
			nameW = n
		}
		if n := len(visibilityLabel(row.Visibility)); n > visW {
			visW = n
		}
	}
	fmt.Fprintln(w, infoText(fmt.Sprintf("%-*s  %-*s  %s", nameW, "NAME", visW, "VISIBILITY", "ID")))
	for _, row := range rows {
		fmt.Fprintf(w, "%-*s  %-*s  %s\n", nameW, glide.DisplayName(row.Context, row.Name), visW, visibilityLabel(row.Visibility), row.ID)
	}
}

func runVariGet(cmd *cobra.Command, args []string) error {
	client, err := variClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveVari(cmd, client, args[0])
	if err != nil {
		return err
	}
	onlyValue, _ := cmd.Flags().GetBool("value")
	if onlyValue {
		if variIsSecret(ref) {
			return failUsage("SECRET variables have no readable value; use inject in a server or API function")
		}
		value, err := client.GetQuery(variCollection, joinAction(ref.ID, "value"), nil)
		if err != nil {
			return fail(err)
		}
		return writeJSON(cmd.OutOrStdout(), value)
	}
	return writeJSON(cmd.OutOrStdout(), displayVariable(ref.Spec))
}

func runVariCreate(cmd *cobra.Command, _ []string) error {
	client, err := variClient(cmd)
	if err != nil {
		return err
	}
	payload, err := variWritePayload(cmd, true)
	if err != nil {
		return err
	}
	created, err := client.Create(variCollection, payload)
	if err != nil {
		return fail(err)
	}
	id := mapString(created, "id")
	name := mapString(payload, "name")
	ctx := mapString(payload, "context")
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("created variable %s (%s)", glide.DisplayName(ctx, name), id))
	}
	return nil
}

func runVariUpdate(cmd *cobra.Command, args []string) error {
	client, err := variClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveVari(cmd, client, args[0])
	if err != nil {
		return err
	}
	payload, err := variWritePayload(cmd, false)
	if err != nil {
		return err
	}
	if ctx, _ := cmd.Flags().GetString("new-context"); cmd.Flags().Changed("new-context") {
		payload["context"] = ctx
	}
	if len(payload) == 0 {
		return failUsage("pass at least one field to update")
	}
	headers := otpHeaders(cmd)
	updated, err := client.Update(variCollection, ref.ID, payload, headers)
	if err != nil {
		return fail(err)
	}
	id := mapString(updated, "id")
	if id == "" {
		id = ref.ID
	}
	name := firstNonEmpty(mapString(updated, "name"), mapString(payload, "name"), ref.Name)
	ctx := firstNonEmpty(mapString(updated, "context"), mapString(payload, "context"), ref.Context)
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("updated variable %s (%s)", glide.DisplayName(ctx, name), id))
	}
	return nil
}

func runVariDelete(cmd *cobra.Command, args []string) error {
	client, err := variClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveVari(cmd, client, args[0])
	if err != nil {
		return err
	}
	if err := client.Delete(variCollection, ref.ID, otpHeaders(cmd)); err != nil {
		return fail(err)
	}
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("deleted variable %s (%s)", glide.DisplayName(ref.Context, ref.Name), ref.ID))
	}
	return nil
}

func runVariCopy(cmd *cobra.Command, args []string) error {
	client, err := variClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveVari(cmd, client, args[0])
	if err != nil {
		return err
	}
	toName, _ := cmd.Flags().GetString("to-name")
	toCtx, _ := cmd.Flags().GetString("to-context")
	if toName == "" {
		toName = ref.Name
	}
	if toCtx == "" {
		toCtx = ref.Context
	}
	if toName == ref.Name && toCtx == ref.Context {
		return failUsage("copy requires --to-name and/or --to-context different from the source")
	}

	src, _ := ref.Spec.(map[string]any)
	payload := map[string]any{
		"name":    toName,
		"context": toCtx,
	}
	if desc, _ := cmd.Flags().GetString("description"); cmd.Flags().Changed("description") {
		payload["description"] = desc
	} else if d := mapString(src, "description"); d != "" {
		payload["description"] = d
	}
	vis, err := copyVisibility(cmd, ref.Visibility)
	if err != nil {
		return err
	}
	if vis != "" {
		payload["visibility"] = vis
	}
	secrecy, secret, err := copySecrecy(cmd, ref)
	if err != nil {
		return err
	}
	payload["secrecy"] = secrecy
	payload["secret"] = secret
	if exp, _ := cmd.Flags().GetString("expires-at"); cmd.Flags().Changed("expires-at") {
		payload["expiresAt"] = exp
	} else if exp := mapString(src, "expiresAt"); exp != "" {
		payload["expiresAt"] = exp
	}

	value, err := copyValue(cmd, client, ref)
	if err != nil {
		return err
	}
	payload["value"] = value

	created, err := client.Create(variCollection, payload)
	if err != nil {
		return fail(err)
	}
	id := mapString(created, "id")
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("copied variable %s → %s (%s)", glide.DisplayName(ref.Context, ref.Name), glide.DisplayName(toCtx, toName), id))
	}
	return nil
}

func copyVisibility(cmd *cobra.Command, fallback string) (string, error) {
	if !cmd.Flags().Changed("visibility") {
		return fallback, nil
	}
	vis, _ := cmd.Flags().GetString("visibility")
	return parseVisibility(vis)
}

func copySecrecy(cmd *cobra.Command, ref variRef) (string, bool, error) {
	secrecy := ref.Secrecy
	if secrecy == "" && ref.Secret {
		secrecy = "SECRET"
	}
	if secrecy == "" {
		secrecy = "NONE"
	}
	if cmd.Flags().Changed("secret") && !cmd.Flags().Changed("secrecy") {
		if secret, _ := cmd.Flags().GetBool("secret"); secret {
			secrecy = "SECRET"
		} else {
			secrecy = "NONE"
		}
	}
	if cmd.Flags().Changed("secrecy") {
		raw, _ := cmd.Flags().GetString("secrecy")
		parsed, err := parseSecrecy(raw)
		if err != nil {
			return "", false, err
		}
		secrecy = parsed
	}
	return secrecy, secrecy == "SECRET", nil
}

func copyValue(cmd *cobra.Command, client *api.HTTPClient, ref variRef) (any, error) {
	if cmd.Flags().Changed("value") || cmd.Flags().Changed("value-file") {
		return readVariValue(cmd)
	}
	if variIsSecret(ref) {
		return nil, failUsage("cannot copy a SECRET variable without --value (the platform does not return secret values)")
	}
	value, err := client.GetQuery(variCollection, joinAction(ref.ID, "value"), nil)
	if err != nil {
		if isNotFound(err) {
			return nil, failUsage("cannot read the source value; pass --value")
		}
		return nil, fail(err)
	}
	return value, nil
}

func variWritePayload(cmd *cobra.Command, creating bool) (map[string]any, error) {
	payload := map[string]any{}
	if creating || cmd.Flags().Changed("name") {
		name, _ := cmd.Flags().GetString("name")
		if creating && name == "" {
			return nil, failUsage("name is required")
		}
		if name != "" {
			payload["name"] = name
		}
	}
	if creating {
		ctx, _ := cmd.Flags().GetString("context")
		payload["context"] = ctx
	}
	if cmd.Flags().Changed("value") && cmd.Flags().Changed("value-file") {
		return nil, failUsage("pass only one of --value or --value-file")
	}
	if cmd.Flags().Changed("value") || cmd.Flags().Changed("value-file") {
		v, err := readVariValue(cmd)
		if err != nil {
			return nil, err
		}
		payload["value"] = v
	}
	if cmd.Flags().Changed("description") {
		desc, _ := cmd.Flags().GetString("description")
		payload["description"] = desc
	}
	if creating || cmd.Flags().Changed("visibility") {
		vis, _ := cmd.Flags().GetString("visibility")
		if vis == "" && creating {
			vis = "ENVIRONMENT"
		}
		if vis != "" {
			parsed, err := parseVisibility(vis)
			if err != nil {
				return nil, err
			}
			payload["visibility"] = parsed
		}
	}
	secrecySet := cmd.Flags().Changed("secrecy")
	secretSet := cmd.Flags().Changed("secret")
	if creating || secrecySet || secretSet {
		secrecy := "NONE"
		if secret, _ := cmd.Flags().GetBool("secret"); secret {
			secrecy = "SECRET"
		}
		if secrecySet {
			raw, _ := cmd.Flags().GetString("secrecy")
			parsed, err := parseSecrecy(raw)
			if err != nil {
				return nil, err
			}
			secrecy = parsed
		}
		payload["secrecy"] = secrecy
		payload["secret"] = secrecy == "SECRET"
	}
	if cmd.Flags().Changed("expires-at") {
		exp, _ := cmd.Flags().GetString("expires-at")
		payload["expiresAt"] = exp
	}
	return payload, nil
}

func readVariValue(cmd *cobra.Command) (any, error) {
	if cmd.Flags().Changed("value-file") {
		path, _ := cmd.Flags().GetString("value-file")
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, failUsage(fmt.Sprintf("read %s: %v", path, err))
		}
		return parseVariValue(string(raw)), nil
	}
	raw, _ := cmd.Flags().GetString("value")
	return parseVariValue(raw), nil
}

func parseVariValue(raw string) any {
	s := strings.TrimSpace(raw)
	if s == "" {
		return raw
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		return v
	}
	return raw
}

func parseVisibility(s string) (string, error) {
	v := strings.ToUpper(strings.TrimSpace(s))
	switch v {
	case "PUBLIC", "TENANT", "ENVIRONMENT":
		return v, nil
	default:
		return "", failUsage("option `visibility` must be PUBLIC, TENANT, or ENVIRONMENT")
	}
}

func parseSecrecy(s string) (string, error) {
	v := strings.ToUpper(strings.TrimSpace(s))
	switch v {
	case "NONE", "SECRET", "OBSCURED":
		return v, nil
	case "PARTIAL":
		return "", failUsage("PARTIAL secrecy is deprecated; use SECRET or OBSCURED")
	default:
		return "", failUsage("option `secrecy` must be NONE, SECRET, or OBSCURED")
	}
}

func otpHeaders(cmd *cobra.Command) map[string]string {
	otp, _ := cmd.Flags().GetString("otp")
	if otp == "" {
		return nil
	}
	return map[string]string{"x-otp": otp}
}

func variClient(cmd *cobra.Command) (*api.HTTPClient, error) {
	g := globalsFrom(cmd)
	cfg, err := loadFromGlobal(g)
	if err != nil {
		return nil, fail(err)
	}
	if _, _, err := cfg.RequireCredentials(); err != nil {
		return nil, fail(err)
	}
	client, err := api.FromConfig(cfg)
	if err != nil {
		return nil, fail(err)
	}
	return client, nil
}

func resolveVari(cmd *cobra.Command, client *api.HTTPClient, idOrName string) (variRef, error) {
	context, _ := cmd.Flags().GetString("context")
	if context != "" {
		return lookupVariByName(client, context, idOrName)
	}
	return lookupVariByID(client, idOrName)
}

func lookupVariByID(client *api.HTTPClient, id string) (variRef, error) {
	spec, err := client.Get(variCollection, id)
	if err != nil {
		return variRef{}, fail(err)
	}
	ref := variFromItem(spec)
	if ref.ID == "" {
		ref.ID = id
	}
	ref.Spec = spec
	return ref, nil
}

func lookupVariByName(client *api.HTTPClient, context, name string) (variRef, error) {
	items, err := client.ListAll(variCollection)
	if err != nil {
		return variRef{}, fail(err)
	}
	var sameCase []variRef
	var anyCase []variRef
	for _, item := range items {
		ref := variFromItem(item)
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
		return variRef{}, failUsage(fmt.Sprintf("no variable named %q in context %q", name, context))
	case 1:
		ref := matches[0]
		if ref.ID == "" {
			return ref, nil
		}
		spec, err := client.Get(variCollection, ref.ID)
		if err != nil {
			return variRef{}, fail(err)
		}
		full := variFromItem(spec)
		full.ID = ref.ID
		full.Spec = spec
		return full, nil
	default:
		return variRef{}, failUsage(fmt.Sprintf("unclear variable reference %s; matches %d variables", glide.DisplayName(context, name), len(matches)))
	}
}

func variFromItem(item any) variRef {
	m, _ := item.(map[string]any)
	secret, _ := m["secret"].(bool)
	return variRef{
		ID:         firstMapString(m, "id"),
		Name:       mapString(m, "name"),
		Context:    mapString(m, "context"),
		Visibility: mapString(m, "visibility"),
		Secrecy:    mapString(m, "secrecy"),
		Secret:     secret,
		Spec:       item,
	}
}

func variIsSecret(ref variRef) bool {
	if ref.Secret {
		return true
	}
	return strings.EqualFold(ref.Secrecy, "SECRET")
}

func displayVariable(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	out := make(map[string]any, len(m))
	for k, val := range m {
		out[k] = val
	}
	if !variIsSecret(variFromItem(out)) && !variIsMasked(out) {
		return out
	}
	out["value"] = redactVariValue(out["value"])
	return out
}

func variIsMasked(m map[string]any) bool {
	if b, ok := m["secret"].(bool); ok && b {
		return true
	}
	switch strings.ToUpper(mapString(m, "secrecy")) {
	case "SECRET", "OBSCURED", "PARTIAL":
		return true
	}
	return false
}

func redactVariValue(v any) any {
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok {
		if strings.Contains(s, redactedMark) {
			return s
		}
		if len(s) < 10 {
			return redactedMark
		}
		n := len(s) / 10
		if n > 4 {
			n = 4
		}
		if n < 1 {
			n = 1
		}
		return redactedMark + s[len(s)-n:]
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return redactedMark
	}
	if strings.Contains(string(raw), redactedMark) {
		return v
	}
	return redactedMark
}
