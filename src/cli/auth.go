package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/polyapi/polyglot/src/config"
	"github.com/spf13/cobra"
)

func runLogin(cmd *cobra.Command, args []string) error {
	g := globalsFrom(cmd)
	var argURL, argKey string
	if len(args) > 0 {
		argURL = args[0]
	}
	if len(args) > 1 {
		argKey = args[1]
	}
	apiVersion, _ := cmd.Flags().GetString("api-version")
	if apiVersion == "" {
		apiVersion = "1"
	}
	return loginInner(cmd, g, argURL, argKey, apiVersion)
}

func runLogout(cmd *cobra.Command, _ []string) error {
	g := globalsFrom(cmd)
	if os.Getenv("POLY_API_KEY") != "" {
		printWarning(cmd.OutOrStdout(), "POLY_API_KEY is set in the environment; unset it to stop using that key")
	}
	if err := config.ClearStoredKeys(projectRoot(), g.PolyPath, config.ProductionSecrets()); err != nil {
		return fail(err)
	}
	printOk(cmd.OutOrStdout(), "cleared stored API keys")
	return nil
}

func runWhoami(cmd *cobra.Command, _ []string) error {
	g := globalsFrom(cmd)
	cfg, err := loadFromGlobal(g)
	if err != nil {
		return fail(err)
	}
	if _, _, err := cfg.RequireCredentials(); err != nil {
		return fail(err)
	}
	printIdentity(cmd.OutOrStdout(), cfg)
	printProjectContext(cmd.OutOrStdout(), LoadSessionContext(g))
	return nil
}

func runConfigShow(cmd *cobra.Command, _ []string) error {
	g := globalsFrom(cmd)
	cfg, err := loadFromGlobal(g)
	if err != nil {
		return fail(err)
	}
	fmt.Fprint(cmd.OutOrStdout(), cfg.RedactedTOML())
	return nil
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	g := globalsFrom(cmd)
	cfg, err := loadFromGlobal(g)
	if err != nil {
		return fail(err)
	}
	value, err := cfg.Setting(args[0])
	if err != nil {
		return fail(err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), value)
	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	g := globalsFrom(cmd)
	if err := config.SetProjectSetting(projectRoot(), g.PolyPath, args[0], args[1]); err != nil {
		return fail(err)
	}
	printOk(cmd.OutOrStdout(), fmt.Sprintf("set %s", args[0]))
	return nil
}

func loginInner(cmd *cobra.Command, g Globals, argURL, argKey, apiVersion string) error {
	secrets := config.ProductionSecrets()
	existing, err := config.Load(projectRoot(), g.PolyPath, overlays(g), secrets)
	if err != nil {
		return fail(err)
	}
	url, err := takeURL(cmd, g, argURL, existing)
	if err != nil {
		return fail(err)
	}
	key, err := takeKey(cmd, g, argKey, existing)
	if err != nil {
		return fail(err)
	}
	if _, err := config.SaveProjectCredentials(projectRoot(), g.PolyPath, url, key, apiVersion, secrets); err != nil {
		return fail(err)
	}
	switch act, err := config.EnsurePolyGitignored(projectRoot()); {
	case err != nil:
		return fail(err)
	case act == config.GitignoreCreated:
		printOk(cmd.OutOrStdout(), "created .gitignore with .poly/")
	case act == config.GitignoreAppended:
		printOk(cmd.OutOrStdout(), "added .poly/ to .gitignore")
	case act == config.GitignoreSkippedNotGit:
		printWarning(cmd.OutOrStdout(), "not a git repository; skipped .gitignore (.poly/ holds encrypted credentials)")
	}
	saved, err := config.Load(projectRoot(), g.PolyPath, config.Overlays{}, secrets)
	if err != nil {
		return fail(err)
	}
	printOk(cmd.OutOrStdout(), "logged in")
	printIdentity(cmd.OutOrStdout(), saved)
	printProjectContext(cmd.OutOrStdout(), LoadSessionContext(g))
	return nil
}

func takeURL(cmd *cobra.Command, g Globals, argURL string, existing config.Resolved) (string, error) {
	if s := firstNonEmpty(argURL, g.BaseURL); s != "" {
		return s, nil
	}
	if existing.BaseURL != "" && skipPrompt(g, argURL != "", false) {
		return existing.BaseURL, nil
	}
	def := existing.BaseURL
	if def == "" {
		def = "na1"
	}
	return promptString(cmd, "Poly API Base URL", def, g)
}

func takeKey(cmd *cobra.Command, g Globals, argKey string, existing config.Resolved) (string, error) {
	if s := firstNonEmpty(argKey, g.APIKey); s != "" && !strings.HasPrefix(s, "*") {
		return s, nil
	}
	if existing.APIKey != "" && skipPrompt(g, false, argKey != "") {
		return existing.APIKey, nil
	}
	def := ""
	if existing.APIKey != "" {
		def = config.RedactSecret(existing.APIKey)
	}
	entered, err := promptString(cmd, "Poly App Key or User Key", def, g)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(entered, "*") {
		if existing.APIKey == "" {
			return "", missingCreds()
		}
		return existing.APIKey, nil
	}
	return entered, nil
}

func skipPrompt(g Globals, hasURL, hasKey bool) bool {
	return g.NonInteractive || (hasURL && hasKey)
}

func promptString(cmd *cobra.Command, label, def string, g Globals) (string, error) {
	in := cmd.InOrStdin()
	if g.NonInteractive || !isInteractive(in) {
		return "", &config.Error{
			Code: 3,
			Msg:  "cannot prompt in non-interactive mode",
		}
	}
	out := cmd.OutOrStdout()
	if def != "" {
		fmt.Fprintf(out, "%s %s: ", label, infoText("["+def+"]"))
	} else {
		fmt.Fprintf(out, "%s: ", label)
	}
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		return "", &config.Error{Code: 3, Msg: "cannot prompt in non-interactive mode"}
	}
	line := strings.TrimSpace(scanner.Text())
	if line == "" {
		return def, nil
	}
	return line, nil
}

func isInteractive(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func missingCreds() error {
	return &config.Error{
		Code: 3,
		Msg:  "no API key or base URL configured; run `polyapi auth login` or set POLY_API_KEY and POLY_API_BASE_URL",
	}
}

func printIdentity(w io.Writer, cfg config.Resolved) {
	inst := cfg.Instance
	if inst == "" {
		inst = "(unknown)"
	}
	url := cfg.BaseURL
	if url == "" {
		url = "(unset)"
	}
	urlSrc := cfg.URLSource.String()
	if urlSrc == "unset" {
		urlSrc = "(unset)"
	}
	keySrc := cfg.KeySource.String()
	if keySrc == "unset" {
		keySrc = "(unset)"
	}
	fmt.Fprintf(w, "%s:    %s\n", infoText("instance"), inst)
	fmt.Fprintf(w, "%s:    %s\n", infoText("base_url"), url)
	fmt.Fprintf(w, "%s:     %s\n", infoText("api_key"), cfg.RedactedKey())
	fmt.Fprintf(w, "%s:  %s\n", infoText("url_source"), urlSrc)
	fmt.Fprintf(w, "%s:  %s\n", infoText("key_source"), keySrc)
}
