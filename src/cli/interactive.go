package cli

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// RowKind is a node in the interactive command/flag tree.
type RowKind int

const (
	KindFlag RowKind = iota
	KindArg
	KindCommand
	KindRun
)

const (
	rankBaseURL  = 10
	rankAPIKey   = 20
	rankEnv      = 30
	rankLang     = 40
	rankAdapter  = 50
	rankPolyPath = 60
	rankCommand  = 100
	rankArg      = 200
	rankLocal    = 300
	rankRun      = 900
)

// Row is one line in the interactive tree.
type Row struct {
	ID       string
	Kind     RowKind
	Rank     int
	Path     []string
	Name     string
	Label    string
	Usage    string
	Value    string
	Display  string
	Source   string
	Filled   bool
	Required bool
	Boolean  bool
	Secret   bool
	Depth    int
}

type stored struct {
	Raw    string
	Source string
}

type globalSpec struct {
	Name        string
	Rank        int
	Required    bool
	Secret      bool
	HideDefault string
	Usage       string
}

var globalFlagOrder = []globalSpec{
	{Name: "base-url", Rank: rankBaseURL, Required: true, Usage: "Instance base URL"},
	{Name: "api-key", Rank: rankAPIKey, Required: true, Secret: true, Usage: "API key (never printed)"},
	{Name: "env", Rank: rankEnv, Usage: "Named environment from [environments.*]"},
	{Name: "lang", Rank: rankLang, Usage: "Language adapter override"},
	{Name: "adapter", Rank: rankAdapter, Usage: "Adapter launch command override"},
	{Name: "poly-path", Rank: rankPolyPath, HideDefault: ".poly", Usage: "Path to the project .poly directory"},
}

type level struct {
	selected []string
	focusID  string
}

// Interactive is the command/flag tree the TUI drives.
type Interactive struct {
	root     *cobra.Command
	sess     SessionContext
	values   map[string]stored
	selected []string
	stack    []level
	Cursor   int
	Filter   string
}

// NewInteractive builds tree state from the Cobra root and a session snapshot.
func NewInteractive(root *cobra.Command, sess SessionContext) *Interactive {
	s := &Interactive{
		root:   root,
		sess:   sess,
		values: map[string]stored{},
	}
	s.hydrateSession(sess)
	s.FocusFirstIncomplete()
	return s
}

func (s *Interactive) hydrateSession(sess SessionContext) {
	if sess.BaseURL != "" {
		s.set(gKey("base-url"), sess.BaseURL, sourceOr(sess.URLSource, "config"), false)
	}
	if sess.APIKey != "" {
		s.set(gKey("api-key"), sess.APIKey, sourceOr(sess.KeySource, "config"), false)
	}
	if sess.Env != "" {
		s.set(gKey("env"), sess.Env, "flag", false)
	}
	if sess.Language != "" {
		s.set(gKey("lang"), sess.Language, sourceOr(sess.LanguageSource, "detected"), false)
	}
	if sess.Adapter != "" {
		src := "flag"
		if sess.LanguageSource == "config" {
			src = "config"
		}
		s.set(gKey("adapter"), sess.Adapter, src, false)
	}
	if sess.PolyPath != "" && sess.PolyPath != ".poly" {
		s.set(gKey("poly-path"), sess.PolyPath, "flag", false)
	}
}

// HydrateFromCommand copies flags already set on this invocation (and ENV-backed session).
func (s *Interactive) HydrateFromCommand(cmd *cobra.Command) {
	root := cmd.Root()
	flags := root.PersistentFlags()
	for _, spec := range globalFlagOrder {
		f := flags.Lookup(spec.Name)
		if f == nil || !f.Changed {
			continue
		}
		val := f.Value.String()
		if val == "" || (spec.HideDefault != "" && val == spec.HideDefault) {
			continue
		}
		s.set(gKey(spec.Name), val, "flag", true)
	}
}

func sourceOr(s, fallback string) string {
	if s == "" || s == "(unset)" {
		return fallback
	}
	return s
}

func (s *Interactive) set(key, raw, source string, overwrite bool) {
	if raw == "" && !overwrite {
		return
	}
	cur, ok := s.values[key]
	if ok && !overwrite && cur.Raw != "" {
		return
	}
	if raw == "" {
		delete(s.values, key)
		return
	}
	s.values[key] = stored{Raw: raw, Source: source}
}

func (s *Interactive) get(key string) stored {
	return s.values[key]
}

// Session returns the snapshot the tree was seeded from.
func (s *Interactive) Session() SessionContext { return s.sess }

// Selected is the currently chosen command path.
func (s *Interactive) Selected() []string { return append([]string{}, s.selected...) }

// SetFilter restricts the unfilled command tree (filled rows always stay visible).
func (s *Interactive) SetFilter(q string) {
	prev := s.currentID()
	s.Filter = q
	s.relocateAfterFilter(prev)
}

// TypeFilter appends typed text to the live filter.
func (s *Interactive) TypeFilter(text string) {
	if text == "" {
		return
	}
	s.SetFilter(s.Filter + text)
}

// BackspaceFilter removes the last filter character.
func (s *Interactive) BackspaceFilter() {
	if s.Filter == "" {
		return
	}
	runes := []rune(s.Filter)
	s.SetFilter(string(runes[:len(runes)-1]))
}

func (s *Interactive) currentID() string {
	rows := s.Rows()
	if s.Cursor >= 0 && s.Cursor < len(rows) {
		return rows[s.Cursor].ID
	}
	return ""
}

func (s *Interactive) relocateAfterFilter(prev string) {
	rows := s.Rows()
	if prev != "" {
		for i, r := range rows {
			if r.ID == prev {
				s.Cursor = i
				return
			}
		}
	}
	for i, r := range rows {
		if !r.Filled {
			s.Cursor = i
			return
		}
	}
	if len(rows) > 0 {
		s.Cursor = 0
	}
}

// Rows is the visible tree: filled flags first (global → narrow), then incomplete items.
func (s *Interactive) Rows() []Row {
	var filled, unfilled []Row
	for _, spec := range globalFlagOrder {
		r := s.globalRow(spec)
		if r.ID == "" {
			continue
		}
		if r.Filled {
			filled = append(filled, r)
		} else {
			unfilled = append(unfilled, r)
		}
	}
	if len(s.selected) > 0 {
		cmd := s.lookup(s.selected)
		if cmd != nil {
			cr := commandRow(cmd, s.selected, 0)
			cr.Filled = true
			cr.Value = strings.Join(s.selected, " ")
			cr.Display = cr.Value
			cr.Source = "entry"
			filled = append(filled, cr)
			if isRunnable(cmd) {
				for i, arg := range positionalArgs(cmd) {
					r := s.argRow(s.selected, arg, i, 0)
					if r.Filled {
						filled = append(filled, r)
					} else {
						unfilled = append(unfilled, r)
					}
				}
				for i, f := range localFlags(cmd) {
					r := s.flagRow(s.selected, f, i, 0)
					if r.Filled {
						filled = append(filled, r)
					} else {
						unfilled = append(unfilled, r)
					}
				}
			}
			unfilled = append(unfilled, s.childCommands(cmd, s.selected)...)
			if isRunnable(cmd) {
				unfilled = append(unfilled, runRow(s.selected, cmd))
			}
		}
	} else {
		unfilled = append(unfilled, s.childCommands(s.root, nil)...)
	}

	sort.SliceStable(filled, func(i, j int) bool {
		if filled[i].Rank != filled[j].Rank {
			return filled[i].Rank < filled[j].Rank
		}
		return filled[i].ID < filled[j].ID
	})
	unfilled = s.filterUnfilled(unfilled)
	return append(filled, unfilled...)
}

func (s *Interactive) filterUnfilled(rows []Row) []Row {
	q := strings.ToLower(s.Filter)
	if q == "" {
		return rows
	}
	keep := map[string]bool{}
	for _, r := range rows {
		blob := strings.ToLower(strings.Join([]string{r.Label, r.Name, strings.Join(r.Path, " ")}, " "))
		if strings.Contains(blob, q) {
			keep[r.ID] = true
			for i := 1; i <= len(r.Path); i++ {
				keep["cmd:"+strings.Join(r.Path[:i], "/")] = true
			}
		}
	}
	var out []Row
	for _, r := range rows {
		if keep[r.ID] {
			out = append(out, r)
		}
	}
	return out
}

func (s *Interactive) globalRow(spec globalSpec) Row {
	st := s.get(gKey(spec.Name))
	if spec.HideDefault != "" && st.Raw == spec.HideDefault && st.Source != "entry" && st.Source != "flag" {
		return Row{}
	}
	if spec.Name == "adapter" && st.Raw == "" {
		return Row{}
	}
	if spec.Name == "poly-path" && (st.Raw == "" || st.Raw == spec.HideDefault) {
		return Row{}
	}
	r := Row{
		ID:       "flag:--" + spec.Name,
		Kind:     KindFlag,
		Rank:     spec.Rank,
		Name:     spec.Name,
		Label:    "--" + spec.Name,
		Usage:    spec.Usage,
		Value:    st.Raw,
		Source:   st.Source,
		Filled:   st.Raw != "",
		Required: spec.Required,
		Secret:   spec.Secret,
	}
	r.Display = displayValue(r)
	return r
}

func (s *Interactive) childCommands(parent *cobra.Command, path []string) []Row {
	var rows []Row
	for _, c := range parent.Commands() {
		if skipCommand(c) {
			continue
		}
		p := append(append([]string{}, path...), c.Name())
		rows = append(rows, commandRow(c, p, 0))
	}
	return rows
}

func commandRow(c *cobra.Command, path []string, depth int) Row {
	return Row{
		ID:    "cmd:" + strings.Join(path, "/"),
		Kind:  KindCommand,
		Rank:  rankCommand + depth,
		Path:  path,
		Name:  c.Name(),
		Label: c.Name(),
		Usage: c.Short,
		Depth: depth,
	}
}

func runRow(path []string, c *cobra.Command) Row {
	return Row{
		ID:    "run:" + strings.Join(path, "/"),
		Kind:  KindRun,
		Rank:  rankRun,
		Path:  path,
		Name:  "run",
		Label: "run  " + strings.Join(path, " "),
		Usage: c.Short,
	}
}

type argSpec struct {
	Name     string
	Optional bool
	Variadic bool
}

func (s *Interactive) argRow(path []string, arg argSpec, i, depth int) Row {
	st := s.get(aKey(path, arg.Name))
	usage := "positional argument"
	if cmd := s.lookup(path); cmd != nil {
		if short := strings.TrimSpace(cmd.Short); short != "" {
			usage = short
		}
	}
	r := Row{
		ID:       "arg:" + strings.Join(path, "/") + ":" + arg.Name,
		Kind:     KindArg,
		Rank:     rankArg + i,
		Path:     path,
		Name:     arg.Name,
		Label:    argLabel(arg),
		Usage:    usage,
		Value:    st.Raw,
		Source:   st.Source,
		Filled:   st.Raw != "",
		Required: !arg.Optional,
		Depth:    depth,
	}
	r.Display = displayValue(r)
	return r
}

func argLabel(arg argSpec) string {
	if arg.Optional {
		if arg.Variadic {
			return "[" + arg.Name + "]..."
		}
		return "[" + arg.Name + "]"
	}
	return "<" + arg.Name + ">"
}

func (s *Interactive) flagRow(path []string, f *pflag.Flag, i, depth int) Row {
	st := s.get(fKey(path, f.Name))
	boolean := f.Value.Type() == "bool"
	filled := st.Raw != ""
	if boolean {
		filled = st.Raw == "true"
	}
	r := Row{
		ID:       "flag:" + strings.Join(path, "/") + "/--" + f.Name,
		Kind:     KindFlag,
		Rank:     rankLocal + i,
		Path:     path,
		Name:     f.Name,
		Label:    "--" + f.Name,
		Usage:    f.Usage,
		Value:    st.Raw,
		Source:   st.Source,
		Filled:   filled,
		Required: s.flagRequired(path, f),
		Boolean:  boolean,
		Secret:   f.Name == "api-key" || f.Name == "execution-api-key",
		Depth:    depth,
	}
	r.Display = displayValue(r)
	return r
}

func displayValue(r Row) string {
	if r.Boolean {
		if r.Value == "true" {
			return "true"
		}
		return ""
	}
	if r.Secret && r.Value != "" {
		return redactForDisplay(r.Value)
	}
	return r.Value
}

func redactForDisplay(v string) string {
	if len(v) <= 4 {
		return "********"
	}
	return "********" + v[len(v)-4:]
}

func positionalArgs(cmd *cobra.Command) []argSpec {
	fields := strings.Fields(cmd.Use)
	if len(fields) < 2 {
		return nil
	}
	var out []argSpec
	for _, f := range fields[1:] {
		if strings.HasPrefix(f, "-") {
			continue
		}
		variadic := strings.Contains(f, "...")
		optional := strings.HasPrefix(f, "[") || variadic
		name := strings.Trim(f, "[]<>.")
		name = strings.TrimSuffix(name, "...")
		if name == "" {
			continue
		}
		out = append(out, argSpec{Name: name, Optional: optional, Variadic: variadic})
	}
	return out
}

func localFlags(cmd *cobra.Command) []*pflag.Flag {
	var out []*pflag.Flag
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if skipFlag(f) {
			return
		}
		out = append(out, f)
	})
	return out
}

func skipFlag(f *pflag.Flag) bool {
	return f.Hidden || f.Name == "help"
}

const cobraOneRequiredAnnotation = "cobra_annotation_one_required"

func flagMarkedRequired(f *pflag.Flag) bool {
	vals := f.Annotations[cobra.BashCompOneRequiredFlag]
	return len(vals) > 0 && vals[0] == "true"
}

func (s *Interactive) flagRequired(path []string, f *pflag.Flag) bool {
	if flagMarkedRequired(f) {
		return true
	}
	groups := f.Annotations[cobraOneRequiredAnnotation]
	if len(groups) == 0 {
		return false
	}
	for _, g := range groups {
		if !s.oneRequiredSatisfied(path, strings.Fields(g)) {
			return true
		}
	}
	return false
}

func (s *Interactive) oneRequiredSatisfied(path []string, names []string) bool {
	for _, name := range names {
		if s.flagValueFilled(path, name) {
			return true
		}
	}
	return false
}

func (s *Interactive) flagValueFilled(path []string, name string) bool {
	st := s.get(fKey(path, name))
	if st.Raw == "" || st.Raw == "false" {
		return false
	}
	return true
}

// ReadyToRun is true when a runnable command is selected and every required option is filled.
func (s *Interactive) ReadyToRun() bool {
	if len(s.selected) == 0 {
		return false
	}
	cmd := s.lookup(s.selected)
	if cmd == nil || !isRunnable(cmd) {
		return false
	}
	for _, r := range s.Rows() {
		if !r.Filled && r.Required {
			return false
		}
	}
	return true
}

func skipCommand(c *cobra.Command) bool {
	if c.Hidden {
		return true
	}
	switch c.Name() {
	case "help", "completion", "man":
		return true
	default:
		return false
	}
}

func isRunnable(c *cobra.Command) bool {
	return c.Run != nil || c.RunE != nil
}

func (s *Interactive) lookup(path []string) *cobra.Command {
	if s.root == nil || len(path) == 0 {
		return nil
	}
	cmd, _, err := s.root.Find(path)
	if err != nil {
		return nil
	}
	return cmd
}

func gKey(name string) string { return "g:" + name }

func aKey(path []string, name string) string {
	return "a:" + strings.Join(path, ",") + ":" + name
}

func fKey(path []string, name string) string {
	return "f:" + strings.Join(path, ",") + ":" + name
}

// Current is the row under the cursor.
func (s *Interactive) Current() (Row, bool) {
	rows := s.Rows()
	if s.Cursor < 0 || s.Cursor >= len(rows) {
		return Row{}, false
	}
	return rows[s.Cursor], true
}

// FocusFirstIncomplete puts the cursor on the first required gap, else Run, else the first command.
func (s *Interactive) FocusFirstIncomplete() {
	s.Cursor = s.firstFocusIndex(s.Rows())
}

func (s *Interactive) firstFocusIndex(rows []Row) int {
	for i, r := range rows {
		if !r.Filled && r.Required && len(r.Path) == 0 {
			return i
		}
	}
	if len(s.selected) > 0 {
		for i, r := range rows {
			if !r.Filled && r.Required && (r.Kind == KindArg || r.Kind == KindFlag) && pathEqual(r.Path, s.selected) {
				return i
			}
		}
		if s.ReadyToRun() {
			for i, r := range rows {
				if r.Kind == KindRun {
					return i
				}
			}
		}
	}
	for i, r := range rows {
		if !r.Filled && r.Kind == KindCommand {
			return i
		}
	}
	for i, r := range rows {
		if !r.Filled {
			return i
		}
	}
	if len(rows) == 0 {
		return 0
	}
	return 0
}

func pathEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (s *Interactive) keepID(id string) {
	rows := s.Rows()
	for i, r := range rows {
		if r.ID == id {
			s.Cursor = i
			return
		}
	}
	if s.Cursor >= len(rows) {
		s.Cursor = len(rows) - 1
	}
	if s.Cursor < 0 {
		s.Cursor = 0
	}
}

func (s *Interactive) Move(delta int) {
	rows := s.Rows()
	if len(rows) == 0 {
		s.Cursor = 0
		return
	}
	s.Cursor += delta
	if s.Cursor < 0 {
		s.Cursor = 0
	}
	if s.Cursor >= len(rows) {
		s.Cursor = len(rows) - 1
	}
}

// Back returns one command level and restores the cursor to the item that was left.
// It is a no-op (false) at the root.
func (s *Interactive) Back() bool {
	if len(s.stack) == 0 && len(s.selected) == 0 {
		return false
	}
	if len(s.stack) == 0 {
		id := "cmd:" + strings.Join(s.selected, "/")
		s.selected = nil
		s.keepID(id)
		return true
	}
	frame := s.stack[len(s.stack)-1]
	s.stack = s.stack[:len(s.stack)-1]
	s.selected = append([]string{}, frame.selected...)
	s.keepID(frame.focusID)
	return true
}

func (s *Interactive) push(focusID string) {
	s.stack = append(s.stack, level{
		selected: append([]string{}, s.selected...),
		focusID:  focusID,
	})
	s.Filter = ""
}

// Activate is Enter on the current row.
// Filled flags/args start an overwrite; unfilled commands drill one level; Run executes.
// Selecting an item clears the live filter so the full level is visible again.
func (s *Interactive) Activate() (edit Row, run bool) {
	r, ok := s.Current()
	if !ok {
		return Row{}, false
	}
	if s.Filter != "" {
		id := r.ID
		s.SetFilter("")
		s.keepID(id)
		if next, ok := s.Current(); ok {
			r = next
		}
	}
	switch r.Kind {
	case KindCommand:
		if r.Filled {
			s.Back()
			return Row{}, false
		}
		s.push(r.ID)
		s.selected = append([]string{}, r.Path...)
		s.FocusFirstIncomplete()
		return Row{}, false
	case KindFlag:
		if r.Boolean {
			s.toggle(r)
			s.keepID(r.ID)
			return Row{}, false
		}
		s.keepID(r.ID)
		return r, false
	case KindArg:
		s.keepID(r.ID)
		return r, false
	case KindRun:
		if !s.ReadyToRun() {
			return Row{}, false
		}
		return Row{}, true
	}
	return Row{}, false
}

func (s *Interactive) toggle(r Row) {
	key := s.keyFor(r)
	cur := s.get(key)
	next := "true"
	if cur.Raw == "true" {
		next = "false"
	}
	if next == "false" {
		delete(s.values, key)
		return
	}
	s.set(key, next, "entry", true)
}

func (s *Interactive) keyFor(r Row) string {
	switch r.Kind {
	case KindArg:
		return aKey(r.Path, r.Name)
	case KindFlag:
		if len(r.Path) == 0 {
			return gKey(r.Name)
		}
		return fKey(r.Path, r.Name)
	default:
		return r.ID
	}
}

// Commit writes an edited flag/arg (overwrite) and focuses the next incomplete item.
func (s *Interactive) Commit(r Row, raw string) {
	raw = strings.TrimSpace(raw)
	s.set(s.keyFor(r), raw, "entry", true)
	if s.Filter != "" {
		s.SetFilter("")
		s.keepID(r.ID)
		return
	}
	s.FocusFirstIncomplete()
}

func (s *Interactive) sessionDefault(name string) string {
	switch name {
	case "base-url":
		return s.sess.BaseURL
	case "api-key":
		return s.sess.APIKey
	case "env":
		return s.sess.Env
	case "lang":
		return s.sess.Language
	case "adapter":
		return s.sess.Adapter
	case "poly-path":
		if s.sess.PolyPath != "" {
			return s.sess.PolyPath
		}
		return ".poly"
	default:
		return ""
	}
}

func (s *Interactive) includeGlobal(spec globalSpec, st stored) bool {
	if st.Raw == "" {
		return false
	}
	if spec.HideDefault != "" && st.Raw == spec.HideDefault {
		return false
	}
	if def := s.sessionDefault(spec.Name); def != "" && st.Raw == def {
		return false
	}
	return true
}

// Argv is the Cobra argument list that would run the current selection.
func (s *Interactive) Argv() []string {
	var argv []string
	for _, spec := range globalFlagOrder {
		st := s.get(gKey(spec.Name))
		if !s.includeGlobal(spec, st) {
			continue
		}
		argv = append(argv, "--"+spec.Name, st.Raw)
	}
	if len(s.selected) == 0 {
		return argv
	}
	argv = append(argv, s.selected...)
	cmd := s.lookup(s.selected)
	if cmd == nil {
		return argv
	}
	for _, arg := range positionalArgs(cmd) {
		st := s.get(aKey(s.selected, arg.Name))
		if st.Raw == "" {
			continue
		}
		if arg.Variadic {
			argv = append(argv, strings.Fields(st.Raw)...)
		} else {
			argv = append(argv, st.Raw)
		}
	}
	for _, f := range localFlags(cmd) {
		st := s.get(fKey(s.selected, f.Name))
		if f.Value.Type() == "bool" {
			if st.Raw == "true" {
				argv = append(argv, "--"+f.Name)
			}
			continue
		}
		if st.Raw == "" {
			continue
		}
		argv = append(argv, "--"+f.Name, st.Raw)
	}
	return argv
}

// DisplayArgv is Argv with secrets redacted, for the output overlay.
func (s *Interactive) DisplayArgv() []string {
	argv := s.Argv()
	out := make([]string, len(argv))
	copy(out, argv)
	for i := 0; i < len(out)-1; i++ {
		if out[i] == "--api-key" || out[i] == "--execution-api-key" {
			out[i+1] = redactForDisplay(out[i+1])
		}
	}
	return out
}
