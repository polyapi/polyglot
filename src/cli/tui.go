package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/polyapi/polyglot/src/version"
	"github.com/spf13/cobra"
)

type tuiMode int

const (
	modeBrowse tuiMode = iota
	modeEdit
	modeRunning
	modeError
)

type tuiModel struct {
	cmd      *cobra.Command
	state    *Interactive
	mode     tuiMode
	editRow  Row
	input    textinput.Model
	width    int
	height   int
	offset   int
	output   string
	runLine  string
	status   string
	err      error
	result   *runDoneMsg
	streamed bool
}

// tuiExec runs a Cobra command on the real terminal while the TUI is paused.
type tuiExec struct {
	argv    []string
	display []string
	outW    io.Writer
	errW    io.Writer
	stdout  bytes.Buffer
	stderr  bytes.Buffer
	code    int
}

func (t *tuiExec) SetStdin(io.Reader)    {}
func (t *tuiExec) SetStdout(w io.Writer) { t.outW = w }
func (t *tuiExec) SetStderr(w io.Writer) { t.errW = w }

func (t *tuiExec) Run() error {
	reprintInvokedCommand(t.outW, t.display)
	outW := io.Writer(&t.stdout)
	if t.outW != nil {
		outW = io.MultiWriter(&t.stdout, t.outW)
	}
	errW := io.Writer(&t.stderr)
	if t.errW != nil {
		errW = io.MultiWriter(&t.stderr, t.errW)
	}
	args := append([]string{"--non-interactive"}, t.argv...)
	t.code = runIO(args, outW, errW, bytes.NewReader(nil))
	return nil
}

var tuiRewrotePrompt bool

func invokedProgram() string {
	if len(os.Args) > 0 && os.Args[0] != "" {
		return os.Args[0]
	}
	return "polyapi"
}

// FormatInvokedCommand is program + args, shell-quoted. Exported for tests.
func FormatInvokedCommand(program string, argv []string) string {
	parts := make([]string, 0, 1+len(argv))
	if program != "" {
		parts = append(parts, program)
	}
	parts = append(parts, argv...)
	return shellJoin(parts)
}

// ExtraArgv is the display argv minus tokens already on the original process command line.
func ExtraArgv(osArgs, display []string) []string {
	orig := osArgs
	if len(orig) > 0 {
		orig = orig[1:]
	}
	pairs := map[string]bool{}
	bools := map[string]bool{}
	for i := 0; i < len(orig); i++ {
		tok := orig[i]
		if isCLIFlag(tok) && i+1 < len(orig) && !isCLIFlag(orig[i+1]) {
			pairs[tok+"\x00"+orig[i+1]] = true
			i++
			continue
		}
		bools[tok] = true
	}
	var out []string
	for i := 0; i < len(display); i++ {
		tok := display[i]
		if isCLIFlag(tok) && i+1 < len(display) && !isCLIFlag(display[i+1]) {
			if pairs[tok+"\x00"+display[i+1]] {
				i++
				continue
			}
			out = append(out, tok, display[i+1])
			i++
			continue
		}
		if bools[tok] {
			continue
		}
		out = append(out, tok)
	}
	return out
}

func isCLIFlag(s string) bool {
	return strings.HasPrefix(s, "-")
}

func shellJoin(parts []string) string {
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = shellQuote(p)
	}
	return strings.Join(out, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n'\"\\$`&|;<>()*?[]{}#!") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// reprintInvokedCommand moves up onto the original prompt line and appends
// only the extra argv (not os.Args[0], which is already on that line).
func reprintInvokedCommand(w io.Writer, argv []string) {
	if w == nil {
		return
	}
	extra := ExtraArgv(os.Args, argv)
	if len(extra) == 0 {
		return
	}
	tuiRewrotePrompt = true
	joined := shellJoin(extra)
	orig := invokedProgram()
	out, closer := reprintOutput(w)
	defer closer()
	if width := detectPromptLineWidth(orig); width > 0 {
		fmt.Fprint(out, FormatPromptLineAppend(width, joined))
		return
	}
	fmt.Fprintf(w, "%s\n", FormatInvokedCommand(orig, argv))
}

func reprintOutput(w io.Writer) (io.Writer, func()) {
	if f, ok := w.(*os.File); ok && isCharDevice(f) {
		return f, func() {}
	}
	if isCharDevice(os.Stdout) {
		return os.Stdout, func() {}
	}
	f, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return w, func() {}
	}
	return f, func() { _ = f.Close() }
}

func isCharDevice(f *os.File) bool {
	if f == nil {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// FormatPromptLineAppend moves up one line, to the end of that line's text,
// and writes a space plus extra (the TUI argv without the original program).
func FormatPromptLineAppend(lineCols int, extra string) string {
	if lineCols < 0 {
		lineCols = 0
	}
	return fmt.Sprintf("\x1b[1A\x1b[%dG %s\x1b[K\n", lineCols+1, extra)
}

func detectPromptLineWidth(orig string) int {
	if line := capturePromptLine(); line != "" {
		line = strings.TrimRight(line, " \t")
		if promptLineHasOrig(line, orig) {
			return lipgloss.Width(line)
		}
	}
	if w := measureRenderedPromptWidth(); w > 0 {
		return w + lipgloss.Width(orig)
	}
	return 0
}

func promptLineHasOrig(line, orig string) bool {
	if orig != "" && strings.HasSuffix(line, orig) {
		return true
	}
	if orig == "" {
		return false
	}
	base := filepath.Base(orig)
	return base != orig && strings.HasSuffix(line, base)
}

func capturePromptLine() string {
	try := func(timeout time.Duration, name string, args ...string) string {
		return lastNonEmptyLine(runCapture(timeout, name, args...))
	}
	switch {
	case os.Getenv("TMUX") != "":
		if s := try(150*time.Millisecond, "tmux", "capture-pane", "-p", "-J"); s != "" {
			return s
		}
	case os.Getenv("KITTY_WINDOW_ID") != "":
		if s := try(150*time.Millisecond, "kitty", "@", "get-text", "--extent", "screen"); s != "" {
			return s
		}
	case os.Getenv("WEZTERM_PANE") != "":
		if s := try(150*time.Millisecond, "wezterm", "cli", "get-text"); s != "" {
			return s
		}
	}
	if runtime.GOOS == "darwin" {
		return captureMacPromptLine()
	}
	return ""
}

func captureMacPromptLine() string {
	switch os.Getenv("TERM_PROGRAM") {
	case "iTerm.app":
		for _, app := range []string{"iTerm2", "iTerm"} {
			script := `tell application "` + app + `" to tell current session of current window to get contents`
			if s := lastNonEmptyLine(runCapture(400*time.Millisecond, "osascript", "-e", script)); s != "" {
				return s
			}
		}
	case "Apple_Terminal":
		script := `tell application "Terminal" to get contents of selected tab of front window`
		return lastNonEmptyLine(runCapture(400*time.Millisecond, "osascript", "-e", script))
	}
	return ""
}

func measureRenderedPromptWidth() int {
	if _, err := exec.LookPath("starship"); err == nil {
		if out := runCapture(250*time.Millisecond, "starship", "prompt"); out != "" {
			if w := lastLineWidth(out); w > 0 {
				return w
			}
		}
	}
	if os.Getenv("POSH_PID") != "" || os.Getenv("POSH_THEME") != "" {
		if out := runCapture(250*time.Millisecond, "oh-my-posh", "print", "primary"); out != "" {
			if w := lastLineWidth(out); w > 0 {
				return w
			}
		}
	}
	return 0
}

func runCapture(timeout time.Duration, name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = nil
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func lastNonEmptyLine(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimRight(lines[i], " \t")
		if line != "" {
			return line
		}
	}
	return ""
}

func lastLineWidth(s string) int {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return 0
	}
	lines := strings.Split(s, "\n")
	return lipgloss.Width(lines[len(lines)-1])
}

type runDoneMsg struct {
	argv   []string
	stdout string
	stderr string
	code   int
}

func runInteractive(cmd *cobra.Command) error {
	tuiRewrotePrompt = false
	g := globalsFrom(cmd)
	sess := LoadSessionContext(g)
	st := NewInteractive(cmd.Root(), sess)
	st.HydrateFromCommand(cmd)
	st.FocusFirstIncomplete()
	m := tuiModel{
		cmd:   cmd,
		state: st,
		input: newValueInput(),
		width: 80, height: 24,
	}
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return fail(err)
	}
	done, ok := final.(tuiModel)
	if !ok {
		return nil
	}
	if done.err != nil {
		return fail(done.err)
	}
	if done.result != nil && done.result.code == 0 && !done.streamed {
		if done.result.stdout != "" {
			fmt.Fprint(cmd.OutOrStdout(), done.result.stdout)
		}
		if done.result.stderr != "" {
			fmt.Fprint(cmd.ErrOrStderr(), done.result.stderr)
		}
	}
	return nil
}

func newValueInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "value"
	ti.CharLimit = 2048
	return ti
}

func (m tuiModel) Init() tea.Cmd { return nil }

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case runDoneMsg:
		done := msg
		m.result = &done
		if done.code == 0 {
			return m, tea.Quit
		}
		m.mode = modeError
		m.output = formatRunOutput(done)
		m.state.sess = LoadSessionContext(globalsFrom(m.cmd))
		m.state.hydrateSession(m.state.sess)
		return m, nil
	case tea.PasteMsg:
		if m.mode == modeBrowse {
			m.state.TypeFilter(msg.Content)
			m.ensureVisible()
			return m, nil
		}
		if m.mode == modeEdit {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	if m.mode == modeEdit {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m tuiModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.mode {
	case modeRunning:
		return m, nil
	case modeError:
		if key == "enter" || key == "esc" {
			m.mode = modeBrowse
			m.output = ""
			m.result = nil
			return m, nil
		}
		return m, nil
	case modeEdit:
		switch key {
		case "esc":
			m.mode = modeBrowse
			m.input.Blur()
			m.status = ""
			return m, nil
		case "enter":
			m.state.Commit(m.editRow, m.input.Value())
			m.mode = modeBrowse
			m.input.Blur()
			m.status = "saved " + m.editRow.Label
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	switch key {
	case "up":
		m.state.Move(-1)
		m.ensureVisible()
	case "down":
		m.state.Move(1)
		m.ensureVisible()
	case "pgup":
		m.state.Move(-10)
		m.ensureVisible()
	case "pgdown":
		m.state.Move(10)
		m.ensureVisible()
	case "esc":
		if m.state.Filter != "" {
			m.state.SetFilter("")
			m.ensureVisible()
			return m, nil
		}
		if m.state.Back() {
			m.ensureVisible()
			return m, nil
		}
		return m, tea.Quit
	case "enter":
		edit, run := m.state.Activate()
		if run {
			argv := m.state.Argv()
			if len(m.state.Selected()) == 0 {
				m.status = "select a command first"
				return m, nil
			}
			display := m.state.DisplayArgv()
			m.mode = modeRunning
			m.runLine = "polyapi " + strings.Join(display, " ")
			m.output = ""
			m.result = nil
			m.streamed = true
			ex := &tuiExec{argv: argv, display: display}
			return m, tea.Exec(ex, func(error) tea.Msg {
				return runDoneMsg{
					argv:   display,
					stdout: ex.stdout.String(),
					stderr: ex.stderr.String(),
					code:   ex.code,
				}
			})
		}
		if edit.ID != "" {
			m.mode = modeEdit
			m.editRow = edit
			m.input = newValueInput()
			if edit.Secret {
				m.input.EchoMode = textinput.EchoPassword
				m.input.EchoCharacter = '•'
			}
			if !edit.Secret {
				m.input.SetValue(edit.Value)
			}
			m.status = "overwrite " + edit.Label
			return m, m.input.Focus()
		}
		m.ensureVisible()
	case "backspace":
		m.state.BackspaceFilter()
		m.ensureVisible()
	default:
		if text := msg.Text; text != "" {
			m.state.TypeFilter(text)
			m.ensureVisible()
		}
	}
	return m, nil
}

func (m *tuiModel) ensureVisible() {
	body := m.bodyHeight()
	if m.state.Cursor < m.offset {
		m.offset = m.state.Cursor
	}
	if m.state.Cursor >= m.offset+body {
		m.offset = m.state.Cursor - body + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m tuiModel) bodyHeight() int {
	h := m.height - 6
	if m.mode == modeEdit && strings.TrimSpace(m.editRow.Usage) != "" {
		h--
	}
	if h < 8 {
		h = 8
	}
	return h
}

func (m tuiModel) View() tea.View {
	var v tea.View
	v.AltScreen = true
	v.WindowTitle = "polyapi"
	if m.mode == modeEdit {
		cur := m.input.Cursor()
		if cur != nil {
			v.Cursor = cur
		}
	}
	v.SetContent(m.render())
	return v
}

func (m tuiModel) render() string {
	switch m.mode {
	case modeRunning:
		return m.renderRunning()
	case modeError:
		return m.renderError()
	}
	w := m.width
	if w <= 0 {
		w = 80
	}
	var b strings.Builder
	b.WriteString(m.renderHeader(w))
	b.WriteByte('\n')
	rows := m.state.Rows()
	m.clampOffset(len(rows))
	body := m.bodyHeight()
	end := m.offset + body
	if end > len(rows) {
		end = len(rows)
	}
	sawUnfilled := false
	for i := m.offset; i < end; i++ {
		r := rows[i]
		if !r.Filled && !sawUnfilled {
			if i > 0 && rows[i-1].Filled {
				b.WriteString(paint(Stone700, false, strings.Repeat("─", max(8, w-2))))
				b.WriteByte('\n')
			}
			sawUnfilled = true
		}
		b.WriteString(m.renderRow(r, i == m.state.Cursor, w))
		b.WriteByte('\n')
	}
	b.WriteString(m.renderFooter(w))
	return b.String()
}

func (m *tuiModel) clampOffset(n int) {
	body := m.bodyHeight()
	if m.state.Cursor < m.offset {
		m.offset = m.state.Cursor
	}
	if m.state.Cursor >= m.offset+body {
		m.offset = m.state.Cursor - body + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
	if n > 0 && m.offset > n-1 {
		m.offset = n - 1
	}
}

func (m tuiModel) renderHeader(w int) string {
	sess := m.state.Session()
	left := header("polyapi " + version.Version)
	right := paint(Stone500, false, FormatBanner(sess))
	gap := w - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if gap < 1 {
		return left + "\n" + right
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m tuiModel) renderRow(r Row, selected bool, w int) string {
	indent := strings.Repeat("  ", r.Depth)
	marker := "  "
	if selected {
		marker = paint(River600, true, "▸ ")
	}
	label := r.Label
	switch r.Kind {
	case KindCommand:
		label = paint(Macaw500, selected, r.Label)
	case KindFlag:
		if r.Required && !r.Filled {
			label = paint(Sunray600, true, r.Label) + paint(Sunray500, true, " *")
		} else if !r.Filled {
			label = paint(Stone500, selected, r.Label)
		} else {
			label = paint(Sunray600, selected, r.Label)
		}
	case KindArg:
		if r.Required && !r.Filled {
			label = paint(River600, true, r.Label) + paint(Sunray500, true, " *")
		} else if !r.Filled {
			label = paint(Stone500, selected, r.Label)
		} else {
			label = paint(River600, selected, r.Label)
		}
	case KindRun:
		if m.state.ReadyToRun() {
			label = paint(Jungle600, true, r.Label)
		} else {
			label = paint(Stone500, false, r.Label)
		}
	}
	value := r.Display
	if r.Secret && r.Display != "" {
		value = paint(Stone500, false, r.Display)
	} else if value != "" {
		value = paint(River600, false, value)
	}
	src := ""
	if r.Filled && r.Source != "" {
		src = paint(Stone500, false, r.Source)
	}
	usage := ""
	switch {
	case r.Kind == KindRun && !m.state.ReadyToRun():
		usage = paint(Stone500, false, "complete required options first")
	case !r.Filled && r.Required && (r.Kind == KindFlag || r.Kind == KindArg):
		usage = paint(Sunray500, false, "required")
		if r.Usage != "" {
			usage += "  " + paint(Stone500, false, r.Usage)
		}
	case !r.Filled && r.Kind != KindRun:
		usage = paint(Stone500, false, r.Usage)
	}
	line := indent + marker + label
	if value != "" {
		line += "  " + value
	}
	if src != "" {
		line += "  " + src
	} else if usage != "" {
		line += "  " + usage
	}
	if selected {
		line = lipgloss.NewStyle().Background(Stone850).Width(max(1, w)).Render(line)
	}
	return line
}

// FormatEditPrompt is the TUI chrome for editing a flag or argument: the
// usage/description in list-view gray, then the label and typed value.
func FormatEditPrompt(label, usage, inputView string, width int) string {
	var b strings.Builder
	if u := strings.TrimSpace(usage); u != "" {
		if width > 0 {
			u = lipgloss.Wrap(u, width, "")
		}
		b.WriteString(paint(Stone500, false, u))
		b.WriteByte('\n')
	}
	b.WriteString(paint(Sunray600, true, label))
	b.WriteString("  ")
	b.WriteString(inputView)
	return b.String()
}

func (m tuiModel) renderFooter(w int) string {
	var b strings.Builder
	b.WriteString(paint(Stone700, false, strings.Repeat("─", max(8, w))))
	b.WriteByte('\n')
	switch m.mode {
	case modeEdit:
		b.WriteString(FormatEditPrompt(m.editRow.Label, m.editRow.Usage, m.input.View(), w))
		b.WriteByte('\n')
		b.WriteString(paint(Stone500, false, "enter save   esc cancel   filled values can be overwritten"))
	default:
		if m.state.Filter != "" {
			b.WriteString(paint(Sunray600, true, m.state.Filter))
			b.WriteByte('\n')
		}
		help := "type to filter   ↑↓ move   enter select   esc back   ctrl+c quit"
		if m.status != "" {
			help = m.status + "   " + help
		}
		b.WriteString(paint(Stone500, false, help))
	}
	_ = w
	return b.String()
}

func (m tuiModel) renderRunning() string {
	w := m.width
	if w <= 0 {
		w = 80
	}
	var b strings.Builder
	b.WriteString(m.renderHeader(w))
	b.WriteByte('\n')
	b.WriteString(paint(Stone700, false, strings.Repeat("─", max(8, w))))
	b.WriteByte('\n')
	b.WriteString(paint(River600, true, "$ "+m.runLine))
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString(paint(Sunray500, false, "running…"))
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString(paint(Stone500, false, "ctrl+c abort"))
	return b.String()
}

func (m tuiModel) renderError() string {
	w := m.width
	if w <= 0 {
		w = 80
	}
	var b strings.Builder
	b.WriteString(m.renderHeader(w))
	b.WriteByte('\n')
	b.WriteString(paint(Stone700, false, strings.Repeat("─", max(8, w))))
	b.WriteByte('\n')
	b.WriteString(paint(Macaw600, true, "command failed"))
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString(m.output)
	if !strings.HasSuffix(m.output, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(paint(Stone700, false, strings.Repeat("─", max(8, w))))
	b.WriteByte('\n')
	b.WriteString(paint(Stone500, false, "esc/enter back to adjust   ctrl+c quit"))
	return b.String()
}

func formatRunOutput(msg runDoneMsg) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", FormatInvokedCommand(invokedProgram(), msg.argv))
	if msg.stdout != "" {
		b.WriteString(msg.stdout)
		if !strings.HasSuffix(msg.stdout, "\n") {
			b.WriteByte('\n')
		}
	}
	if msg.stderr != "" {
		b.WriteString(msg.stderr)
		if !strings.HasSuffix(msg.stderr, "\n") {
			b.WriteByte('\n')
		}
	}
	if msg.code != 0 {
		fmt.Fprintf(&b, "\nexit %d\n", msg.code)
	}
	return b.String()
}
