package cli

import (
	"fmt"
	"image/color"
	"io"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/fang"
)

// Poly brand palette. Hex values match the product CSS tokens.
var (
	White = lipgloss.Color("#FFFFFF")
	Black = lipgloss.Color("#000000")

	// Stone (gray)
	Stone50  = lipgloss.Color("#F9F9FC")
	Stone100 = lipgloss.Color("#EFEFF3")
	Stone200 = lipgloss.Color("#E1E1E7")
	Stone300 = lipgloss.Color("#D1D1D7")
	Stone400 = lipgloss.Color("#BCBCC3")
	Stone500 = lipgloss.Color("#909098")
	Stone600 = lipgloss.Color("#676770")
	Stone700 = lipgloss.Color("#464650")
	Stone800 = lipgloss.Color("#3C3C47")
	Stone850 = lipgloss.Color("#212028")
	Stone900 = lipgloss.Color("#121219")

	// Jungle (green)
	Jungle25  = lipgloss.Color("#EBFFF0")
	Jungle50  = lipgloss.Color("#D0F5D7")
	Jungle100 = lipgloss.Color("#BAF1C5")
	Jungle200 = lipgloss.Color("#A1E7B0")
	Jungle300 = lipgloss.Color("#8AE59E")
	Jungle400 = lipgloss.Color("#5ED07B")
	Jungle500 = lipgloss.Color("#45B662")
	Jungle600 = lipgloss.Color("#3AAF58")
	Jungle700 = lipgloss.Color("#2F9E4D")
	Jungle800 = lipgloss.Color("#288540")
	Jungle900 = lipgloss.Color("#1D7334")

	// Macaw (red)
	Macaw25  = lipgloss.Color("#FFECEE")
	Macaw50  = lipgloss.Color("#FFDDE0")
	Macaw100 = lipgloss.Color("#FFC8CC")
	Macaw200 = lipgloss.Color("#FAA8A0")
	Macaw300 = lipgloss.Color("#F98D83")
	Macaw400 = lipgloss.Color("#F57A6C")
	Macaw500 = lipgloss.Color("#F05648")
	Macaw600 = lipgloss.Color("#E63F36")
	Macaw700 = lipgloss.Color("#DB3C28")
	Macaw800 = lipgloss.Color("#BF2814")
	Macaw900 = lipgloss.Color("#AA190F")

	// Sunray (yellow)
	Sunray25  = lipgloss.Color("#FFFAEA")
	Sunray50  = lipgloss.Color("#FFF3CE")
	Sunray100 = lipgloss.Color("#FFEBA8")
	Sunray200 = lipgloss.Color("#FEE07D")
	Sunray300 = lipgloss.Color("#FDD75B")
	Sunray400 = lipgloss.Color("#FCCD55")
	Sunray500 = lipgloss.Color("#FBC146")
	Sunray600 = lipgloss.Color("#F9B727")
	Sunray700 = lipgloss.Color("#EBA91E")
	Sunray800 = lipgloss.Color("#CC9218")
	Sunray900 = lipgloss.Color("#845C08")

	// River (blue)
	River25  = lipgloss.Color("#F2FDFF")
	River50  = lipgloss.Color("#DEF7FF")
	River100 = lipgloss.Color("#B3E6FF")
	River200 = lipgloss.Color("#86D8FF")
	River300 = lipgloss.Color("#6CD1FF")
	River400 = lipgloss.Color("#57C2FF")
	River500 = lipgloss.Color("#50B3FC")
	River600 = lipgloss.Color("#4793F7")
	River700 = lipgloss.Color("#3E76EF")
	River800 = lipgloss.Color("#2665E7")
	River900 = lipgloss.Color("#0C46E1")

	// Twilight (purple)
	Twilight25  = lipgloss.Color("#FFEBFF")
	Twilight50  = lipgloss.Color("#FAD7FF")
	Twilight100 = lipgloss.Color("#F5AFFF")
	Twilight200 = lipgloss.Color("#EB64FF")
	Twilight300 = lipgloss.Color("#D750FF")
	Twilight400 = lipgloss.Color("#BE37F8")
	Twilight500 = lipgloss.Color("#AF1EF0")
	Twilight600 = lipgloss.Color("#9B0FE1")
	Twilight700 = lipgloss.Color("#870AD2")
	Twilight800 = lipgloss.Color("#7305B9")
	Twilight900 = lipgloss.Color("#6400A5")
)

func colorEnabled() bool {
	if os.Getenv("CLICOLOR_FORCE") == "1" {
		return true
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("CLICOLOR") == "0" {
		return false
	}
	return true
}

func paint(c color.Color, bold bool, s string) string {
	if !colorEnabled() {
		return s
	}
	st := lipgloss.NewStyle().Foreground(c)
	if bold {
		st = st.Bold(true)
	}
	return st.Render(s)
}

func header(s string) string { return paint(River600, true, s) }

// Header paints a River brand header. Exported for tests.
func Header(s string) string { return header(s) }

func okText(s string) string   { return paint(Jungle600, true, s) }
func failText(s string) string { return paint(Macaw600, true, s) }
func warnText(s string) string { return paint(Sunray500, true, s) }
func infoText(s string) string { return paint(Stone500, true, s) }

func errorLine(msg string) string {
	return failText("✗ ERROR:") + " " + msg
}

func printError(w io.Writer, msg string) {
	fmt.Fprintln(w, errorLine(msg))
}

func printOk(w io.Writer, msg string) {
	fmt.Fprintln(w, okText("✓ OK:")+" "+msg)
}

// PrintOk writes a Jungle success line. Exported for tests.
func PrintOk(w io.Writer, msg string) { printOk(w, msg) }

func printWarning(w io.Writer, msg string) {
	fmt.Fprintln(w, warnText("! WARNING:")+" "+msg)
}

// PrintWarning writes a warning line. Exported for tests.
func PrintWarning(w io.Writer, msg string) { printWarning(w, msg) }

// polyColorScheme maps Poly brand tokens onto Fang's help/error styles.
func polyColorScheme(_ lipgloss.LightDarkFunc) fang.ColorScheme {
	return fang.ColorScheme{
		Base:           Stone500,
		Title:          River600,
		Description:    Stone500,
		Codeblock:      Stone850,
		Program:        River600,
		DimmedArgument: Stone500,
		Comment:        Stone500,
		Flag:           Sunray600,
		FlagDefault:    Stone500,
		Command:        Macaw500,
		QuotedString:   Sunray600,
		Argument:       River600,
		Help:           Stone500,
		Dash:           Stone500,
		ErrorHeader:    [2]color.Color{White, Macaw600},
		ErrorDetails:   Macaw600,
	}
}

// StatusTag is the plan/push/pull badge, matching poly-flow.
type StatusTag struct {
	Symbol string
	Label  string
	Color  color.Color
	Bold   bool
}

// LookupStatusTag returns the plan/push/pull badge for an action.
func LookupStatusTag(action string) (StatusTag, bool) { return statusTag(action) }

func statusTag(action string) (StatusTag, bool) {
	switch action {
	case "created", "updated", "deleted", "adopted":
		return StatusTag{"✓", title(action), Jungle600, true}, true
	case "skipped":
		return StatusTag{"⊘", "Skipped", Stone500, false}, true
	case "orphan":
		return StatusTag{"○", "Orphan", Stone500, false}, true
	case "blocked":
		return StatusTag{"⊡", "Blocked", Sunray600, true}, true
	case "failed":
		return StatusTag{"✗", "Failed", Macaw600, true}, true
	case "would_create":
		return StatusTag{"◆", "Would create", River600, false}, true
	case "would_update":
		return StatusTag{"◆", "Would update", River600, false}, true
	case "would_delete":
		return StatusTag{"◆", "Would delete", River600, false}, true
	default:
		return StatusTag{}, false
	}
}

func title(action string) string {
	switch action {
	case "created":
		return "Created"
	case "updated":
		return "Updated"
	case "deleted":
		return "Deleted"
	case "adopted":
		return "Adopted"
	case "skipped":
		return "Skipped"
	case "orphan":
		return "Orphan"
	default:
		return action
	}
}
