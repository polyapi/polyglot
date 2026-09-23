# polyglot

The PolyAPI polyglot CLI: a single global `polyapi` binary for every language the platform supports.

- **Repo:** [`polyapi/polyglot`](https://github.com/polyapi/polyglot)
- **Module:** `github.com/polyapi/polyglot`
- **Binary / command:** `polyapi`
- **License:** [MIT](LICENSE)

Product spec: [RFC.md](RFC.md). Work catalogue: [TICKETS.md](TICKETS.md). Contributor and agent conventions: [CONTRIBUTING.md](CONTRIBUTING.md), [AGENTS.md](AGENTS.md).

## Status

Foundation is in (POLY-CLI-1–5, doctor/version/update/`init`, Glide v2, the function command tree, OpenAPI `model`, Vari, Tabi tables, webhooks/triggers, jobs, schemas, snippets, and Canopy applications). Remaining resource CRUD and real language adapters are not implemented yet — those commands still exist in the tree so help and the interactive browser are complete.

| Area | Commands | State |
| --- | --- | --- |
| Config / auth | `auth login` / `logout` / `whoami`, `config show` / `get` / `set` | Working |
| Meta | `version`, `doctor`, `update` | Working |
| SDK | `generate`, `clear` | Working (via language adapter) |
| Interactive | bare `polyapi` on a TTY | Working (walks the real Cobra tree) |
| Init | `init` | Working (templates, GitHub/zip unpack, venv, runtime install; [docs/init.md](docs/init.md)) |
| Glide | `deploy prepare` / `validate` / `plan` / `push` (`sync`) / `pull` | Working (JSONC in-host; code modules via adapter `extract` / `prepare`) |
| Functions | `function list` / `get` / `init` / `add` / `update` / `delete` / `execute` / `logs` | Working (`init` writes TS/Python source for server/client, JSONC for api/ai; `add`/`update` parse via adapter, then HTTP upsert) |
| Variables | `vari list` / `get` / `init` / `create` / `update` / `delete` / `copy` | Working (POLY-CLI-7; [docs/vari.md](docs/vari.md)) |
| Tables | `table list` / `get` / `init` / `create` / `update` / `delete`, `table rows …` | Working (POLY-CLI-8; [docs/table.md](docs/table.md)) |
| Webhooks | `webhook list` / `get` / `init` / `create` / `update` / `delete` / `test` / `url` | Working (POLY-CLI-9; [docs/webhook.md](docs/webhook.md)) |
| Triggers | `trigger list` / `get` / `init` / `create` / `update` / `delete` | Working (POLY-CLI-9; [docs/trigger.md](docs/trigger.md)) |
| Jobs | `job list` / `get` / `init` / `create` / `update` / `delete` / `enable` / `disable` / `run`, `job executions …` | Working (POLY-CLI-10; [docs/job.md](docs/job.md)) |
| Schemas | `schema list` / `get` / `init` / `create` / `update` / `delete` | Working (POLY-CLI-18; [docs/schema.md](docs/schema.md)) |
| Snippets | `snippet list` / `get` / `init` / `add` / `update` / `delete` | Working (POLY-CLI-18; [docs/snippet.md](docs/snippet.md)) |
| Subscriptions | `subscription list` / `get` / `init` / `create` / `update` / `delete` / `recover` | Working (POLY-CLI-19; [docs/subscription.md](docs/subscription.md)) |
| Applications | `app list` / `get` / `init` / `create` / `update` / `delete` / `url` (aliases `canopy`, `application`) | Working (POLY-CLI-23; [docs/app.md](docs/app.md)) |
| Resources | `logs`, `tenant` | Tree only (CRUD is stubbed) |
| OpenAPI | `model generate\|validate\|train` | Working (POLY-CLI-6; [docs/model.md](docs/model.md)) |

Real TypeScript / Python adapters live in those SDKs (POLY-CLI-11 / 12), not this repo. Tests use fixtures under `tests/fixtures/delegate/`.

## Install

### Linux and MacOS:

```bash
curl -fsSL https://raw.githubusercontent.com/polyapi/polyglot/main/scripts/install.sh | sh
polyapi version
```

If the install directory is not already on `PATH`, the script appends it to your shell rc file (search for `>>> polyapi PATH >>>`). `curl | sh` cannot change the current shell — copy the `export PATH=…` line it prints, or open a new terminal.

### Windows:

```powershell
irm https://raw.githubusercontent.com/polyapi/polyglot/main/scripts/install.ps1 | iex
polyapi version
```

If the install directory is not already on `PATH`, the script prepends it to the user PATH and to this PowerShell session. Open a new terminal for other apps.

## Update

```bash
polyapi update --check
polyapi update
```

## Build

Requires [Go](https://go.dev/) 1.27 or later.

```bash
go test ./...
go run ./src/cmd/polyapi --help
go build -o polyapi ./src/cmd/polyapi
scripts/build.sh           # versioned binaries in builds/
scripts/build.sh --native  # this OS/arch only
```

CGO is not required. Static / cross builds:

```bash
CGO_ENABLED=0 go build -o polyapi ./src/cmd/polyapi
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o polyapi ./src/cmd/polyapi
```

Tagged `v*` builds in [`.github/workflows/dist.yml`](.github/workflows/dist.yml) cross-compile linux/darwin/windows (amd64+arm64), attach SHA-256 checksums, and publish a GitHub Release.

## Versioning

The `polyapi` binary follows [semver](https://semver.org/). The language-delegate protocol is versioned separately (`POLY_DELEGATE_VERSION` / protocol `1`) and is not tied 1:1 to the binary version.

`polyapi version` (and `--version`) print semver plus commit, build time, Go toolchain, OS/arch, and protocol. Tagged dist builds set `Version` / `Commit` / `Date` via `-ldflags`. A local `go build` in this git checkout fills commit/time from VCS info.

## Package layout

One Go module (no extra modules until compile time or a publish boundary forces a split). Production code lives under `src/`; tests live under `tests/` as external `*_test` packages (not beside production files).

| Path | Responsibility |
| --- | --- |
| `src/cmd/polyapi` | Binary entrypoint |
| `src/cli` | Cobra command tree, Fang help, brand theme, TUI |
| `src/config` | Viper merge of flags/env/files, encrypted secrets, instance map, deploy policy |
| `src/api` | REST client (`Client` interface, HTTP + in-memory fakes) |
| `src/delegate` | Language detection + adapter process protocol ([RFC §5.5](RFC.md)) |
| `src/glide` | Find deployables, validate / plan / push / pull, local JSONC scaffolds ([docs/glide.md](docs/glide.md)) |
| `src/model` | OpenAPI → Spec Input → validate/train ([docs/model.md](docs/model.md)) |
| `src/exitcode` | Process exit codes |
| `src/version` | Binary semver + build metadata |
| `scripts/` | `build.sh` (versioned `builds/` output), `install.sh`, `install.ps1` |
| `tests/` | Package tests and fixtures (`tests/cli`, `tests/config`, `tests/api`, `tests/delegate`, `tests/glide`, `tests/model`, `tests/scripts`) |

## Config and auth

Precedence (highest first): CLI flags → environment (`POLY_API_KEY`, `POLY_API_BASE_URL`) → project `./.poly/config.toml` → user `$XDG_CONFIG_HOME/poly/config.toml` or `~/.config/poly/config.toml` (XDG even on macOS) → legacy SDK files (read-only).

API keys written by `polyapi auth login` are encrypted at rest. The project file is not enough to decrypt them: decryption also needs a device secret (OS keychain, or `~/.config/poly/device.key` if the keychain is unavailable). CI should pass `POLY_API_KEY` in the environment instead of storing a key.

Instance shorthands: `na1`, `eu1`, `na2`, `dev` / `develop`, `local`. See [docs/config.md](docs/config.md) for the URL map, `[deploy]` policy, and SDK migration notes.

```bash
polyapi auth login na1 "$POLY_API_KEY"
polyapi auth whoami
polyapi config show
polyapi doctor
polyapi version
polyapi update --check
polyapi generate
```

`polyapi doctor` reports binary, protocol, config, adapter, live auth (`GET /auth`), an offline `deploy validate` summary, and whether the current git branch may push (from `[deploy]` / `deploy.targets`). Pass `--offline` to skip live HTTP. `polyapi update` replaces this binary from GitHub Releases after verifying `checksums.txt` (`--check` reports only).

On a TTY, bare `polyapi` opens an interactive command browser over the same Cobra tree. `--help`, `--non-interactive`, pipes, and `CI=1` stay on the one-shot CLI.

`polyapi generate` resolves a TypeScript or Python SDK adapter and runs the language-delegate protocol (see [docs/delegate-protocol.md](docs/delegate-protocol.md)). Pass `--lang` / `--adapter` to override discovery.

## Docs

| Doc | What it is |
| --- | --- |
| [RFC.md](RFC.md) | Product architecture and command taxonomy |
| [TICKETS.md](TICKETS.md) | Draft ticket catalogue and suggested sequencing |
| [docs/config.md](docs/config.md) | Config, auth, instances, deploy policy |
| [docs/delegate-protocol.md](docs/delegate-protocol.md) | Language adapter protocol |
| [docs/glide.md](docs/glide.md) | Glide v2 pipeline (find / validate / plan / push / pull) |
| [docs/install.md](docs/install.md) | Install script, GitHub Releases, `polyapi update` |
| [docs/app.md](docs/app.md) | Canopy applications (`polyapi app`) |
| [CONTRIBUTING.md](CONTRIBUTING.md) | How to build, test, and send a PR |
| [AGENTS.md](AGENTS.md) | Conventions for coding agents working in this repo |
