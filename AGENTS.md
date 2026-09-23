# AGENTS.md

Instructions for coding agents working in `polyapi/polyglot`. Humans: see [CONTRIBUTING.md](CONTRIBUTING.md). Product spec: [RFC.md](RFC.md). Catalogue: [TICKETS.md](TICKETS.md).

## What this is

Go CLI **`polyapi`** (module `github.com/polyapi/polyglot`). One global binary for every PolyAPI resource, Glide v2, and language-agnostic workflows. Language parsing and codegen stay in project-installed SDK adapters.

## Commands

```bash
gofmt -w src tests
go test ./...
go build -o polyapi ./src/cmd/polyapi
go run ./src/cmd/polyapi --help
```

CI formats only `src` and `tests`. Default tests must not hit a live PolyAPI instance.

## Layout

Single Go module at the repo root. Production under `src/`; tests under `tests/` as external `*_test` packages. No `internal/` (tests need to import production APIs). Split extra modules only if incremental builds or a publish boundary force it.

| Path | Responsibility |
| --- | --- |
| `src/cmd/polyapi` | `main` |
| `src/cli` | Cobra tree, Fang help, theme, TUI, command implementations |
| `src/config` | Flags/env/files, encrypted secrets, instances, git branch, deploy policy |
| `src/api` | REST `Client`, `HTTPClient`, `MemoryClient` |
| `src/delegate` | Language detect + adapter subprocess protocol |
| `src/glide` | Find deployables, validate / plan / push / pull, local JSONC scaffolds |
| `src/projinit` | Project `polyapi init`: templates, unpack, runtime, venv, deps |
| `src/model` | OpenAPI → Spec Input → validate/train |
| `src/exitcode` | Exit codes (RFC §5.5.8) |
| `src/version` | Semver + build metadata |
| `tests/` | Tests + `tests/fixtures/delegate/` + `tests/fixtures/model/` |
| `docs/` | `config.md`, `delegate-protocol.md`, `glide.md`, `init.md`, `install.md`, `model.md`, `vari.md`, `table.md`, `webhook.md`, `trigger.md`, `job.md`, `schema.md`, `snippet.md`, `subscription.md`, `app.md` |
| `scripts/` | `build.sh` (versioned `builds/` output), `install.sh`, `install.ps1` |

## Ownership split

If it requires **transpiling, detailed parse, or generating the project language**, it is delegated. If it is **HTTP, policy, finding files, or multi-resource orchestration**, it stays in Go. The host may know file extensions, comment characters, and a `polyConfig` text marker — not an AST. `polyapi deploy` always re-scans (no `--lazy`). Receipts are optional (`deploy.receipts` / `--receipts`).

- Adapter protocol: [docs/delegate-protocol.md](docs/delegate-protocol.md) (protocol integer **1**).
- Transport: subprocess, stdin JSON / stdout JSON, cwd = project root.
- Adapter HTTP: **none**. Go fetches `GET /specs` and description-generation; adapters only read/write project files. Credentials are not injected into the adapter process.
- Real TS/Python adapters are POLY-CLI-11/12 in those SDK repos, not here. Java is last (POLY-CLI-13). Tests use fixture adapters.

## What is implemented

**Working:** `auth login` / `logout` / `whoami`, `config show` / `get` / `set`, `generate` / `clear`, `doctor` / `version` / `update`, `init` (project scaffolder), `deploy prepare` / `validate` / `env-check` / `plan` / `push`/`sync` / `pull`, `function list` / `get` / `init` / `add` / `update` / `delete` / `execute` / `logs`, `model generate` / `validate` / `train`, `vari list` / `get` / `init` / `create` / `update` / `delete` / `copy`, `table list` / `get` / `init` / `create` / `update` / `delete` and `table rows …`, `webhook list` / `get` / `init` / `create` / `update` / `delete` / `test` / `url`, `trigger list` / `get` / `init` / `create` / `update` / `delete`, `job list` / `get` / `init` / `create` / `update` / `delete` / `enable` / `disable` / `run` and `job executions …`, `schema list` / `get` / `init` / `create` / `update` / `delete`, `snippet list` / `get` / `init` / `add` / `update` / `delete`, `subscription list` / `get` / `init` / `create` / `update` / `delete` / `recover`, `app list` / `get` / `init` / `create` / `update` / `delete` / `url` (aliases `canopy`, `application`), Fang `--help`, interactive TUI on a TTY, context banner.

**Stub (`RunE: stub(...)`, exit 1):** resource CRUD (`logs` `tenant`).

`polyapi init` scaffolds a project (templates from the tenant ProjectTemplates config variable, official Glide repos, a public GitHub repo, or a zip; Python venv; Node/Python install prompts). See [docs/init.md](docs/init.md). `polyapi <resource> init` is separate: `--name` required; `--context` optional (jobs/triggers/applications have no `--context`); `function init` requires `--type`; `trigger init` requires `--type webhook|error-handler` (JSONC source is a webhook handle or an error-handler path; `--snippet` must match `--type`); `subscription init` requires `--type CUSTOM|OHIP` (`--snippet` must match `--type` when the snippet JSON has a type field). Server/client functions write TypeScript or Python source (`polyConfig` + a function whose identifier is exactly `--name`, which must be a valid identifier for the language). Other resources write JSONC. `--path` and `--force` optional. `--snippet` fetches a PolyAPI snippet and uses its code as the file (name/context overlay; language must match or the command exits without writing). Without `--snippet` this does not call the API. Prints the written path.

There is **no** `setup` command (`auth login` replaces it). Glide verbs are **not** top-level; `deploy sync` and `deploy env-check` are the only aliases.

Keep stub commands in the tree (help + TUI). Do not implement a stub unless that is the requested ticket.

## Tickets

[TICKETS.md](TICKETS.md) is the work catalogue. The Epic overview **Status** column (`completed` / `in-progress` / `not started`) is the tracker. **Do not tick the per-ticket acceptance-criteria checkboxes.**

Sequencing: Phase 1 foundation **2 → 3 → 4 → 5 → 24** is done (including project `init`). **POLY-CLI-25 Glide v2**, **POLY-CLI-20** function tree, **POLY-CLI-6** OpenAPI `model`, **POLY-CLI-7** Vari, **POLY-CLI-8** Tabi tables, **POLY-CLI-9** webhooks + triggers, **POLY-CLI-10** jobs + executions, **POLY-CLI-18** snippets + schemas, **POLY-CLI-19** GraphQL subscriptions, **POLY-CLI-23** Canopy applications, and **POLY-CLI-15** distribution (releases + install script; Homebrew deferred) are **completed**. **POLY-CLI-11/12** adapters are in progress (protocol 1 freeze: no adapter HTTP; host `GET /specs` and description-generation). Then copy/promote (14). Java last.

When asked to proceed, implement the named ticket. Do not start remaining resource packs, Homebrew, Java, or flow deprecation unless asked. When a ticket’s remaining work lands, update the Epic overview Status column (still do not tick the checkboxes).

## CLI conventions

- Cobra + pflag. Fang styles help (`fang.WithoutVersion()`: **`-v` is verbose**, `--version` is version).
- Examples via `examples(ex{comment, command}...)` in `src/cli/examples.go`. Comments must start with `# `; command lines must include `polyapi`. Fang uppercases section titles (`EXAMPLES`).
- Required flags: `MarkFlagRequired` / `MarkFlagsOneRequired`. `annotateRequiredHelp` appends `(required)` or `(required: --a or --b)` onto `Flag.Usage` so Fang `--help` matches the TUI. Fang still renders a single FLAGS list; do not replace Fang’s help renderer unless asked.
- Backticks in Cobra flag usage become placeholders (`--env prod`).
- Failures: return `fail(err)` / `*ExitError` with `src/exitcode` codes (0, 1, 2, 3, 4, 10, 11, 12).
- Secrets never printed. Redact to `********` + last four. `Debug` / `GoString` on HTTP clients must not include the API key.
- Config precedence: flags → `POLY_*` env → project `./.poly/config.toml` → XDG user config (`$XDG_CONFIG_HOME/poly` or `~/.config/poly`) → read-only legacy SDK files. Env is never written back to disk.
- Instance shorthands: `na1`, `eu1`, `na2`, `dev`/`develop`, `local`. Unknown names must be full `http(s)` URLs.
- `-y` / `--yes` and `--non-interactive` are first-class. Pipes and CI must not prompt.
- Live HTTP waits show a one-line spinner on a TTY (stderr; delayed ~150ms; cleared when the request finishes). Skip it for `--quiet`, `CI`, pipes, and tests. Same path for the TUI and one-shot commands.

## Theme

`src/cli/theme.go` is the Poly CSS palette as `lipgloss.Color("#RRGGBB")`. Do **not** revert the Fang scheme and do **not** invent off-palette colors.

Fang: Title/Program/Argument River600; Flag/QuotedString Sunray600; Command Macaw500; Base/Comment/Help/Dash Stone500; Codeblock Stone850; ErrorHeader White on Macaw600.

Status: success Jungle600, error Macaw600, warning Sunray500, info Stone500. Placeholders should stay readable on black terminals (Stone500, not BrightBlack).

## TUI

Bare `polyapi` on a TTY (stdin+stdout, not `--non-interactive`, not `CI`) opens the Bubble Tea v2 browser in `src/cli/tui.go` + `interactive.go`. It **walks the real Cobra tree** — do not duplicate commands by hand.

`--help`, `--non-interactive`, pipes, and CI never enter the TUI. Context banner goes to **stderr**, skipped for help/version/doctor/update, the `auth` and `config` trees, and the TUI itself.

Bubble Tea v2: `Init` returns `tea.Cmd` (`tea.RequestWindowSize` is a Msg, not a Cmd). `PasteMsg` has `.Content`. Text input is `charm.land/bubbles/v2/textinput`.

## Tests

- External packages under `tests/` (`package cli_test`, …). Export production helpers if tests need them.
- No live PolyAPI in default CI. HTTP: `httptest` or `api.MemoryClient`. Adapters: `tests/fixtures/delegate/`.
- Isolate config: `t.Chdir` + `XDG_CONFIG_HOME` + unset `POLY_API_KEY` / `POLY_API_BASE_URL`.
- Color tests: `CLICOLOR_FORCE=1` and assert `rgbSeq` of the theme tokens. Most CLI tests set `NO_COLOR=1`.
- Help snapshots: set `__FANG_TEST_WIDTH` (Fang caps at 120).
- `POLY_UPDATE_API` overrides the GitHub releases URL for `update` tests; `POLY_UPDATE_DOWNLOAD` and `POLY_UPDATE_DEST` override the asset base URL and the binary path to replace.
- TS fixture tests skip without `node`; Python fixture tests skip without `python3`/`python`.

## Dist / version

`src/version`: `Version` default `0.1.0`; `Commit` / `Date` empty until ldflags or `debug.ReadBuildInfo`. `scripts/build.sh` and `.github/workflows/dist.yml` set all three via `-ldflags`. Asset names: `polyapi-<version>-<goos>-<goarch>[.exe]` (`version.AssetName`). User-Agent is `polyapi/` + `version.Version`.

Tagged `v*` builds publish GitHub Releases plus `checksums.txt` (SHA-256) and artifact attestations. `scripts/install.sh` (Windows: `install.ps1`) downloads one asset, verifies the checksum, and replaces an existing install. `polyapi update` does the same for the running binary (`--check` is report-only). Test overrides: `POLY_UPDATE_API`, `POLY_UPDATE_DOWNLOAD`, `POLY_UPDATE_DEST`. Homebrew is a deferred follow-up, not part of POLY-CLI-15 as landed.
