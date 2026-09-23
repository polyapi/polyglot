package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode"

	"github.com/polyapi/polyglot/src/api"
	"github.com/polyapi/polyglot/src/glide"
	"github.com/spf13/cobra"
)

const appCollection = "applications"

type appRef struct {
	ID          string
	Name        string
	Subpath     string
	Visibility  string
	Description string
	Spec        any
}

func addAppCommands(root *cobra.Command) {
	app := &cobra.Command{
		Use:     "app",
		Aliases: []string{"canopy", "application", "applications"},
		Short:   "Manage Canopy applications",
		Example: examples(
			ex{"List applications:", "polyapi app list"},
			ex{"Scaffold a local application file:", "polyapi app init --name dashboard"},
			ex{"Create from a config file:", "polyapi app create --name dashboard --config-file ./app.json"},
			ex{"Print the Canopy URL:", "polyapi app url dashboard"},
		),
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List Canopy applications",
		Args:  cobra.NoArgs,
		RunE:  runAppList,
		Example: examples(
			ex{"List all applications:", "polyapi app list"},
			ex{"Only apps with a configured UI:", "polyapi app list --configured"},
		),
	}
	list.Flags().Bool("configured", false, "Only applications that have a Canopy config with collections")

	get := &cobra.Command{
		Use:   "get <id-or-name>",
		Short: "Get an application by ID, name, or subpath",
		Args:  cobra.ExactArgs(1),
		RunE:  runAppGet,
		Example: examples(
			ex{"Get by ID:", "polyapi app get abc123"},
			ex{"Get by name:", "polyapi app get dashboard"},
			ex{"Print only the Canopy config:", "polyapi app get dashboard --config"},
		),
	}
	get.Flags().Bool("config", false, "Print only the Canopy config object")

	create := &cobra.Command{
		Use:   "create",
		Short: "Create a Canopy application",
		Long:  "Create a Canopy application. Applications have no context; lookup is by ID, name, or subpath. `--config` / `--config-file` is the Canopy UI definition (`name`, `subpath`, `collections`, optional `login`). Without a config, create sends a minimal `{name, subpath, collections: []}` so you can fill the UI in later.",
		Args:  cobra.NoArgs,
		RunE:  runAppCreate,
		Example: examples(
			ex{"Name only (empty collections):", "polyapi app create --name dashboard"},
			ex{"From a config file:", "polyapi app create --name dashboard --config-file ./app.json --subpath quantum-quirk-dashboard"},
			ex{"Inline config:", `polyapi app create --name dashboard --config '{"name":"dashboard","subpath":"dashboard","collections":[]}'`},
		),
	}
	addAppWriteFlags(create, true)
	_ = create.MarkFlagRequired("name")

	update := &cobra.Command{
		Use:   "update <id-or-name>",
		Short: "Update an application by ID, name, or subpath",
		Args:  cobra.ExactArgs(1),
		RunE:  runAppUpdate,
		Example: examples(
			ex{"Replace the config:", "polyapi app update dashboard --config-file ./app.json"},
			ex{"Rename:", "polyapi app update dashboard --name DashboardV2"},
			ex{"Change the Canopy subpath:", "polyapi app update dashboard --subpath partner-portal"},
		),
	}
	addAppWriteFlags(update, false)

	del := &cobra.Command{
		Use:   "delete <id-or-name>",
		Short: "Delete an application by ID, name, or subpath",
		Args:  cobra.ExactArgs(1),
		RunE:  runAppDelete,
		Example: examples(
			ex{"Delete by name:", "polyapi app delete dashboard"},
		),
	}

	urlCmd := &cobra.Command{
		Use:   "url <id-or-name>",
		Short: "Print the public Canopy URL",
		Args:  cobra.ExactArgs(1),
		RunE:  runAppURL,
		Example: examples(
			ex{"Print the URL:", "polyapi app url dashboard"},
		),
	}

	initCmd := newResourceInitCommand(resourceInitKind{
		Use: "app", Noun: "application", Type: glide.TypeApplication, NeedsContext: false,
	})

	app.AddCommand(list, get, initCmd, create, update, del, urlCmd)
	root.AddCommand(app)
}

func addAppWriteFlags(cmd *cobra.Command, creating bool) {
	if creating {
		cmd.Flags().String("name", "", "Application name")
	} else {
		cmd.Flags().String("name", "", "New application name")
	}
	cmd.Flags().String("description", "", "Description")
	visDef := ""
	visHelp := "Visibility: PUBLIC, TENANT, or ENVIRONMENT"
	if creating {
		visDef = "ENVIRONMENT"
		visHelp += " (default ENVIRONMENT)"
	}
	cmd.Flags().String("visibility", visDef, visHelp)
	cmd.Flags().String("subpath", "", "Canopy URL subpath (`/canopy/<subpath>`). Stored on config.subpath")
	cmd.Flags().String("config", "", "Canopy config JSON object (or a `{config: …}` wrapper)")
	cmd.Flags().String("config-file", "", "Read the Canopy config from a file")
	cmd.Flags().String("owner", "", "Owner user ID (create defaults to the authenticated user)")
	cmd.MarkFlagsMutuallyExclusive("config", "config-file")
}

func runAppList(cmd *cobra.Command, _ []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	configured, _ := cmd.Flags().GetBool("configured")
	var items []any
	if configured {
		items, err = client.ListAll("applications/configured")
	} else {
		items, err = client.ListAll(appCollection)
	}
	if err != nil {
		return fail(err)
	}
	var rows []appRef
	for _, item := range items {
		rows = append(rows, appFromItem(item))
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Name != rows[j].Name {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].ID < rows[j].ID
	})
	out := cmd.OutOrStdout()
	if len(rows) == 0 {
		if !globalsFrom(cmd).Quiet {
			fmt.Fprintln(out, "no applications found")
		}
		return nil
	}
	printAppList(out, rows)
	return nil
}

func printAppList(w io.Writer, rows []appRef) {
	nameW, subW, visW := len("NAME"), len("SUBPATH"), len("VISIBILITY")
	for _, row := range rows {
		if n := len(row.Name); n > nameW {
			nameW = n
		}
		if n := len(appSubpathLabel(row.Subpath)); n > subW {
			subW = n
		}
		if n := len(visibilityLabel(row.Visibility)); n > visW {
			visW = n
		}
	}
	fmt.Fprintln(w, infoText(fmt.Sprintf("%-*s  %-*s  %-*s  %s", nameW, "NAME", subW, "SUBPATH", visW, "VISIBILITY", "ID")))
	for _, row := range rows {
		name := row.Name
		if name == "" {
			name = "-"
		}
		fmt.Fprintf(w, "%-*s  %-*s  %-*s  %s\n", nameW, name, subW, appSubpathLabel(row.Subpath), visW, visibilityLabel(row.Visibility), row.ID)
	}
}

func appSubpathLabel(v string) string {
	if v == "" {
		return "-"
	}
	return v
}

func runAppGet(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveApp(client, args[0])
	if err != nil {
		return err
	}
	configOnly, _ := cmd.Flags().GetBool("config")
	if configOnly {
		cfg := appConfigFromSpec(ref.Spec)
		if cfg == nil {
			return failUsage(fmt.Sprintf("application %q has no config", firstNonEmpty(ref.Name, ref.ID)))
		}
		return writeJSON(cmd.OutOrStdout(), cfg)
	}
	return writeJSON(cmd.OutOrStdout(), ref.Spec)
}

func runAppCreate(cmd *cobra.Command, _ []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	payload, err := appWritePayload(cmd, true, appRef{})
	if err != nil {
		return err
	}
	created, err := client.Create(appCollection, payload)
	if err != nil {
		return fail(err)
	}
	id := mapString(created, "id")
	name := firstNonEmpty(mapString(created, "name"), mapString(payload, "name"), id)
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("created application %s (%s)", name, id))
	}
	return nil
}

func runAppUpdate(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveApp(client, args[0])
	if err != nil {
		return err
	}
	payload, err := appWritePayload(cmd, false, ref)
	if err != nil {
		return err
	}
	if len(payload) == 0 {
		return failUsage("pass at least one field to update")
	}
	updated, err := client.Update(appCollection, ref.ID, payload, nil)
	if err != nil {
		return fail(err)
	}
	id := firstNonEmpty(mapString(updated, "id"), ref.ID)
	name := firstNonEmpty(mapString(updated, "name"), mapString(payload, "name"), ref.Name)
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("updated application %s (%s)", name, id))
	}
	return nil
}

func runAppDelete(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveApp(client, args[0])
	if err != nil {
		return err
	}
	if err := client.Delete(appCollection, ref.ID, nil); err != nil {
		return fail(err)
	}
	if !globalsFrom(cmd).Quiet {
		printOk(cmd.OutOrStdout(), fmt.Sprintf("deleted application %s (%s)", firstNonEmpty(ref.Name, ref.ID), ref.ID))
	}
	return nil
}

func runAppURL(cmd *cobra.Command, args []string) error {
	client, err := apiClient(cmd)
	if err != nil {
		return err
	}
	ref, err := resolveApp(client, args[0])
	if err != nil {
		return err
	}
	sub := firstNonEmpty(ref.Subpath, appSubpathFromSpec(ref.Spec))
	if sub == "" {
		return failUsage(fmt.Sprintf("application %q has no subpath; set config.subpath", firstNonEmpty(ref.Name, ref.ID)))
	}
	base := strings.TrimRight(client.BaseURL(), "/")
	fmt.Fprintln(cmd.OutOrStdout(), base+"/canopy/"+strings.Trim(sub, "/"))
	return nil
}

func appWritePayload(cmd *cobra.Command, creating bool, existing appRef) (map[string]any, error) {
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
	if creating || cmd.Flags().Changed("description") {
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
	if cmd.Flags().Changed("owner") {
		owner, _ := cmd.Flags().GetString("owner")
		if owner != "" {
			payload["ownerUserId"] = owner
		}
	}
	var cfg map[string]any
	var err error
	if cmd.Flags().Changed("config") || cmd.Flags().Changed("config-file") {
		cfg, err = readAppConfig(cmd)
		if err != nil {
			return nil, err
		}
	} else if creating {
		cfg = defaultAppConfig(mapString(payload, "name"), "")
	} else if cmd.Flags().Changed("subpath") {
		cfg = appConfigFromSpec(existing.Spec)
		if cfg == nil {
			cfg = defaultAppConfig(firstNonEmpty(mapString(payload, "name"), existing.Name), "")
		}
	}
	if cfg != nil {
		if name := mapString(payload, "name"); name != "" {
			if strings.TrimSpace(mapString(cfg, "name")) == "" {
				cfg["name"] = name
			}
		} else if existing.Name != "" && strings.TrimSpace(mapString(cfg, "name")) == "" {
			cfg["name"] = existing.Name
		}
		if cmd.Flags().Changed("subpath") {
			sub, _ := cmd.Flags().GetString("subpath")
			sub = strings.Trim(strings.TrimSpace(sub), "/")
			if sub == "" {
				return nil, failUsage("option `subpath` must be a non-empty URL segment")
			}
			cfg["subpath"] = sub
		} else if strings.TrimSpace(mapString(cfg, "subpath")) == "" {
			name := firstNonEmpty(mapString(cfg, "name"), mapString(payload, "name"), existing.Name)
			if slug := appSlug(name); slug != "" {
				cfg["subpath"] = slug
			}
		}
		if _, ok := cfg["collections"]; !ok {
			cfg["collections"] = []any{}
		}
		payload["config"] = cfg
	}
	return payload, nil
}

func defaultAppConfig(name, subpath string) map[string]any {
	if subpath == "" {
		subpath = appSlug(name)
	}
	cfg := map[string]any{
		"name":        name,
		"collections": []any{},
	}
	if subpath != "" {
		cfg["subpath"] = subpath
	}
	return cfg
}

func readAppConfig(cmd *cobra.Command) (map[string]any, error) {
	var raw []byte
	if cmd.Flags().Changed("config-file") {
		path, _ := cmd.Flags().GetString("config-file")
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, failUsage(fmt.Sprintf("read %s: %v", path, err))
		}
		raw = b
	} else {
		s, _ := cmd.Flags().GetString("config")
		raw = []byte(s)
	}
	return parseAppConfig(raw)
}

func parseAppConfig(raw []byte) (map[string]any, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return nil, failUsage("config JSON is empty")
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, failUsage("invalid config JSON: " + err.Error())
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, failUsage("config must be a JSON object")
	}
	if inner, ok := m["config"].(map[string]any); ok {
		return inner, nil
	}
	return m, nil
}

func resolveApp(client *api.HTTPClient, idOrName string) (appRef, error) {
	idOrName = strings.TrimSpace(idOrName)
	if looksLikeUUID(idOrName) {
		return lookupAppByID(client, idOrName)
	}
	ref, err := lookupAppByNameOrSubpath(client, idOrName)
	if err == nil {
		return ref, nil
	}
	byID, idErr := lookupAppByID(client, idOrName)
	if idErr == nil {
		return byID, nil
	}
	return appRef{}, err
}

func lookupAppByID(client *api.HTTPClient, id string) (appRef, error) {
	spec, err := client.Get(appCollection, id)
	if err != nil {
		return appRef{}, fail(err)
	}
	ref := appFromItem(spec)
	if ref.ID == "" {
		ref.ID = id
	}
	ref.Spec = spec
	return ref, nil
}

func lookupAppByNameOrSubpath(client *api.HTTPClient, key string) (appRef, error) {
	items, err := client.ListAll(appCollection)
	if err != nil {
		return appRef{}, fail(err)
	}
	var nameSame, nameAny, subSame, subAny []appRef
	for _, item := range items {
		ref := appFromItem(item)
		if strings.EqualFold(ref.Name, key) {
			nameAny = append(nameAny, ref)
			if ref.Name == key {
				nameSame = append(nameSame, ref)
			}
		}
		if strings.EqualFold(ref.Subpath, key) {
			subAny = append(subAny, ref)
			if ref.Subpath == key {
				subSame = append(subSame, ref)
			}
		}
	}
	matches := nameSame
	if len(matches) == 0 {
		matches = nameAny
	}
	if len(matches) == 0 {
		matches = subSame
	}
	if len(matches) == 0 {
		matches = subAny
	}
	switch len(matches) {
	case 0:
		return appRef{}, failUsage(fmt.Sprintf("no application named %q", key))
	case 1:
		ref := matches[0]
		if ref.ID == "" {
			return ref, nil
		}
		spec, err := client.Get(appCollection, ref.ID)
		if err != nil {
			return appRef{}, fail(err)
		}
		full := appFromItem(spec)
		full.ID = ref.ID
		full.Spec = spec
		return full, nil
	default:
		return appRef{}, failUsage(fmt.Sprintf("unclear application reference %q; matches %d applications", key, len(matches)))
	}
}

func appFromItem(item any) appRef {
	m, _ := item.(map[string]any)
	sub := firstMapString(m, "subpath")
	if sub == "" {
		sub = appSubpathFromSpec(item)
	}
	return appRef{
		ID:          firstMapString(m, "id"),
		Name:        mapString(m, "name"),
		Subpath:     sub,
		Visibility:  mapString(m, "visibility"),
		Description: mapString(m, "description"),
		Spec:        item,
	}
}

func appConfigFromSpec(spec any) map[string]any {
	m, _ := spec.(map[string]any)
	if m == nil {
		return nil
	}
	cfg, _ := m["config"].(map[string]any)
	return cfg
}

func appSubpathFromSpec(spec any) string {
	m, _ := spec.(map[string]any)
	if s := firstMapString(m, "subpath"); s != "" {
		return s
	}
	if cfg := appConfigFromSpec(spec); cfg != nil {
		return mapString(cfg, "subpath")
	}
	return ""
}

func appSlug(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if r > unicode.MaxASCII {
				continue
			}
			b.WriteRune(unicode.ToLower(r))
			lastDash = false
		case r == ' ' || r == '_' || r == '-' || r == '.':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
