# Contributing

Coding agents: start with [AGENTS.md](AGENTS.md). Product spec is [RFC.md](RFC.md); the work catalogue is [TICKETS.md](TICKETS.md).

## Prerequisites

- Go 1.27 or later (`go version`; `go.mod` currently asks for 1.27.1)
- Optional: `node` and/or `python3` on `PATH` so TypeScript / Python adapter fixture tests run instead of skipping

## Workflow

1. Pick a ticket from [TICKETS.md](TICKETS.md) (or an issue that points at one). The Epic overview **Status** column is the tracker; do not tick the per-ticket acceptance-criteria checkboxes.
2. Keep a **single Go module**. Put production code under `src/` and tests under `tests/`; do not introduce extra modules unless compile time or a publish boundary forces it.
3. Match existing Cobra / Fang / theme / error conventions. Secrets must never be printed.

```bash
gofmt -w src tests
go test ./...
```

CI (`.github/workflows/ci.yml`) runs `gofmt -l src tests` (must be empty), `sh -n` on `scripts/*.sh`, and `go test ./...` on Ubuntu, macOS, and Windows. Cross-compiled release binaries are built from `.github/workflows/dist.yml` (manual dispatch or `v*` tags) via `scripts/build.sh`. Tags named `v*` publish a GitHub Release with SHA-256 checksums. Local versioned binaries land in `builds/` (`scripts/build.sh`; gitignored). See [docs/install.md](docs/install.md).

## Layout

See the package table in [README.md](README.md).

- Language-specific transpile / detailed parse / codegen belongs in SDK adapters ([docs/delegate-protocol.md](docs/delegate-protocol.md)), not in this binary. Glide file-finding stays in Go ([docs/glide.md](docs/glide.md)).
- Tests are external packages under `tests/` (for example `package cli_test`). Production packages are not `internal/` so those tests can import them.
- Default tests must not call a live PolyAPI instance. Use `httptest`, `api.MemoryClient`, and the adapters under `tests/fixtures/delegate/`.

## Style

- `gofmt` for `src/` and `tests/` only (that is what CI checks).
- Fang help examples: comment lines start with `# `; command lines include the program name `polyapi`. See `src/cli/examples.go`.
- Brand colors live in `src/cli/theme.go` as `lipgloss.Color("#RRGGBB")` tokens. Use the existing helpers (`okText`, `failText`, `warnText`, `infoText`); do not invent off-palette colors.
- Required flags must be marked with Cobra (`MarkFlagRequired` / `MarkFlagsOneRequired`) so both `--help` and the interactive TUI stay in sync.
- `-v` is verbose (repeatable). `--version` is the version flag. Do not reuse `-v` for version.

## Pull requests

- One ticket-sized change per PR when possible.
- Mention the ticket id (`POLY-CLI-N`) in the PR title or body.
- Do not add live PolyAPI calls to the default test suite.
- Do not implement a stub command or the next catalogue ticket unless that is the point of the PR.
