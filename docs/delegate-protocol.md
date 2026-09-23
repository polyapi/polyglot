# Language delegate protocol

This is the **normative spec** for how the global Go `polyapi` binary collaborates with project-installed language SDKs. It matches [RFC.md](../RFC.md) §5.5. Adapters implement this protocol; contract fixtures under `tests/fixtures/delegate/protocol/v1/` freeze request/response shapes (POLY-CLI-17 expands that suite).

Protocol integer: **1** (`POLY_DELEGATE_VERSION` / `POLY_DELEGATE_PROTOCOL`).

## Split of responsibility

| Concern | Owner | Examples |
| --- | --- | --- |
| CLI UX, flags, help, exit UX | Go `polyapi` | Cobra tree, colors, `--env`, branch gates |
| Config / auth / encrypted secrets | Go | `auth login`, `auth whoami`, `config show\|get\|set`, `[deploy]` policy |
| HTTP to PolyAPI | Go `api` | CRUD, train, replicate |
| **Find deployables** | **Go `glide`** | Walk the repo; JSONC parse; language-light table (extensions, comment syntax, `polyConfig` marker) |
| Glide orchestration | Go `glide` | validate / plan / push / pull, hashes, optional receipts |
| OpenAPI model generate/validate/train | Go `model` | `polyapi model generate\|validate\|train` |
| **Codegen** | **Language adapter** | `generate` → SDK sources / types |
| **Extract canonical payload** from a **code** module | **Language adapter** | transpile/eval typed `polyConfig`. JSONC is extracted in Go. |
| **Prepare writers** (docs/JSDoc/docstrings) | **Language adapter** | rewrite source files |
| **Parse `function add` source** | **Language adapter** | args/types/deps → DTO for Go to POST |
| Optional in-file receipts | Go | Comment header (`//` / `#`) when `deploy.receipts` / `--receipts` |
| Imperative agnostic CRUD | Go | `vari create` when not going through Glide files |

Rule of thumb: if it requires **transpiling, detailed parse, or generating the project language**, it is delegated. If it is **HTTP, policy, finding files, or multi-resource orchestration**, it stays in Go. The host may know file extensions, comment characters, and a `polyConfig` text marker — not an AST.

### Network rule

The adapter **MUST NOT** call Poly HTTP. Go fetches `GET /specs` and description-generation; the adapter only reads and writes project files. Credentials are **not** injected into the adapter process. Language CLIs (`npx poly generate`, `python -m polyapi generate`) may still call Poly until they exec `polyapi` — that is dual-path UX, not this protocol.

## Adapter discovery

Delegated commands (`generate`, `clear`, `deploy prepare` inspect/prepare, `function add`) start at the current working directory and walk **parent directories** until they find a language project. `polyapi deploy` **finds** files in Go; it only spawns the adapter to `inspect` / `extract` / `prepare` **code** candidates.

A directory is a language project if it has a manifest:

| Lang | Project markers |
| --- | --- |
| TypeScript | `package.json`, `package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`, `bun.lock` |
| Python | `pyproject.toml`, `requirements*.txt`, `setup.py`, `setup.cfg`, `Pipfile`, `poetry.lock`, `uv.lock` |
| Java | `pom.xml`, `build.gradle` (later) |

`.poly/config.toml` is also a project boundary (so config is loaded from that root). The walk **stops at the first match**. Finding `package.json` without the PolyAPI library does **not** continue searching; it means this project does not have the SDK installed (exit `10`).

**SDK installed** (required unless `--adapter` is set):

| Lang | Installed when |
| --- | --- |
| TypeScript | `node_modules/polyapi/` exists |
| Python | `polyapi` package under the project (`polyapi/` or `src/polyapi/`) or in a project venv (`.venv` / `venv`) |

**Launch command order** (first match wins):

1. `--adapter <command…>` / `--lang <typescript|python|java>` override
2. `./.poly/config.toml` → `language = "typescript"` and optional `[adapter] command = "…"`
3. Language inferred from the located project's manifests
4. Interactive prompt (or error in `--non-interactive`)

**Default launch commands (v1)**

| Lang | Command |
| --- | --- |
| TypeScript | `node node_modules/polyapi/build/adapter.js` when that file exists; otherwise `node_modules/.bin/poly adapter` |
| Python | project venv `python -m polyapi adapter` when the venv has `polyapi`; otherwise `python3 -m polyapi adapter` with cwd = project root |
| Java | not implemented; pass `--adapter` when one exists |

There is no `npx` / global-install fallback when the project library is missing. Adapters MUST print protocol version on `capabilities`. Go rejects mismatches with exit `11`. A missing adapter binary or SDK module is exit `10`, with an install hint. The adapter subprocess cwd is the located project root.

## Transport

- **Invocation:** Go spawns the adapter as a **subprocess** (no long-lived daemon in v1).
- **stdin:** one UTF-8 JSON **request** document. Adapters MAY also accept `@path` via `--request-file` for large payloads; the v1 host writes stdin.
- **stdout:** one UTF-8 JSON **response** document (pretty or compact). Progress MAY use JSON-lines **before** the final response only if `request.stream === true`; v1 default is single request/response.
- **stderr:** human-readable logs only (never secrets).
- **cwd:** project root.
- **env:** inherits the process environment (including `NO_COLOR`); sets `POLY_DELEGATE=1`, `POLY_DELEGATE_PROTOCOL=1`. Does **not** inject `POLY_API_KEY` / `POLY_API_BASE_URL`.
- **timeout:** configurable (default 300s for `generate`, 120s for extract/discover and other ops). Override with `POLY_DELEGATE_TIMEOUT_SECS`.

## Protocol version

```json
{ "protocol": 1 }
```

Bump `protocol` on breaking changes. Adapters advertise `protocol` and `min_protocol` in `capabilities`. Compatible when the host protocol is in `[min_protocol, protocol]` (host min is 1).

## Request / response envelope

**Request**

```json
{
  "protocol": 1,
  "id": "uuid",
  "op": "discover",
  "project_root": "/abs/path",
  "lang": "typescript",
  "config": {
    "poly_path": ".poly",
    "disable_ai": true
  },
  "params": { }
}
```

**Response**

```json
{
  "protocol": 1,
  "id": "uuid",
  "ok": true,
  "op": "discover",
  "result": { },
  "warnings": [ { "code": "…", "message": "…", "path": "…" } ],
  "error": null
}
```

On failure: `ok: false`, `error: { "code": "…", "message": "…", "details": … }`.

## Operations (`op`)

| `op` | Direction | Purpose | `params` (summary) | `result` (summary) |
| --- | --- | --- | --- | --- |
| `capabilities` | Go→adapter | Probe version & supported ops | `{}` | `{ protocol, min_protocol, ops[], lang, sdk_version }` |
| `generate` | Go→adapter | Render local SDK from host-fetched specs | `{ specs: Specification[], no_types? }` | `{ files_written[], stats }` |
| `discover` | optional | Unused by `polyapi deploy` (Go walks the tree). Language CLIs MAY keep it. | `{ paths?, exclude_paths?, types? }` | `{ deployables: DeployableDesc[] }` |
| `inspect` | Go→adapter | Parse signatures + current docs for prepare | `{ files[] }` | `{ items: [{ file, type, code, description, arguments, returns, disable_ai }] }` |
| `extract` | Go→adapter | Canonical REST DTO for one **code** module | `{ file, type }` | `{ payload, content_hash, meta }` |
| `prepare` | Go→adapter | Write JSDoc/docstrings from host-supplied `docs` | `{ files: [{ file, docs? }], disable_docs }` | `{ changed_files[], skipped[] }` |
| `write_receipt` | optional | Unused when Go writes comment headers itself. | `{ file, receipt: Receipt }` | `{ ok: true }` |
| `parse_function` | Go→adapter | `function add` support | `{ file, name, kind: server\|client }` | `{ dto fields for API }` |
| `clear` | Go→adapter | Clear generated SDK (Python today) | `{}` | `{ ok: true }` |

TypeScript and Python adapters must implement `capabilities`, `generate`, `inspect`, `extract`, `prepare`, and `parse_function`. Python must implement `clear`; TypeScript should. `discover` and `write_receipt` are optional.

**`generate`:** Go checks `libraryGenerate`, `GET /specs` with `--contexts` / `--names` / `--ids` / `--no-types`, and passes the opaque spec array. The adapter does not fetch specs.

**`deploy prepare`:** Go finds code files → `inspect` → `POST /functions/{server\|client}/description-generation` (or webhooks) when AI is on and descriptions are missing → `prepare` writes comments. `--disable-ai` / `DISABLE_AI` / per-file `disable_ai` skip HTTP. `--disable-docs` skips writes.

**`extract` / `parse_function` payload** is the REST create body (camelCase), including `code`. Omit `type` / `kind` and Glide-cache fields.

**`DeployableDesc` (discover)**

```json
{
  "type": "server-function" | "client-function" | "variable" | "webhook" | "job" | "table" | "schema" | "trigger" | "snippet" | "…",
  "name": "apiKey",
  "context": "billing",
  "file": "src/billing/vari/apiKey.ts",
  "export": "polyConfig",
  "content_hash": "optional-if-extracted",
  "receipt": { "instance": "…", "id": "…", "hash": "…", "deployed_at": "…" } | null
}
```

Go owns correlating descriptors with remote state.

## Which user commands call the adapter

| User command | Go | Adapter ops |
| --- | --- | --- |
| `polyapi generate` | `GET /specs`, permission gate | `generate` (specs already fetched) |
| `polyapi deploy prepare` | walk tree; AI description HTTP | `inspect` then `prepare` on **code** candidates |
| `polyapi deploy validate` | walk + JSONC extract; structural + cross-ref | `extract` per code file |
| `polyapi deploy plan` / `polyapi deploy push` | walk, orchestrate syncers, HTTP | `extract` per code file |
| `polyapi deploy pull` | HTTP fetch; write JSON under the repo root | none in v1 (function codegen pull skipped) |
| `polyapi function add` / `update` | HTTP upsert | `parse_function` then optional `generate` |
| `polyapi function list\|get\|delete\|execute\|logs` | HTTP | none. Client functions have no execute route. |
| `polyapi vari create` (imperative) | HTTP only | none |
| diagnostics (`version`, `doctor`) | checks | `capabilities` |
| `polyapi clear` | resolve lang | `clear` |

Language CLIs (`npx poly`, `python -m polyapi`) keep **language-specific** commands (`generate`, `function add`, local prepare writers). They should exec global `polyapi` for `deploy` and other agnostic commands when it is on `PATH`.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Success (`ok: true`) |
| 2 | Bad request / usage (including undetected language) |
| 3 | Auth/config error inside adapter **or** missing credentials for `generate` |
| 4 | Network error (`generate`) |
| 10 | Adapter binary/module missing (host-side before spawn, or interpreter/module not found) |
| 11 | Protocol mismatch (or SDK too old to support the op) |
| 12 | Partial failure (some files); see `result` + `warnings` |
| 1 | Unclassified adapter failure |

## Security & determinism

- Never log API keys; redact values marked secret in extract output when printing.
- `extract` / discover hashing MUST be deterministic given the same file + env token materialization rules.
- Disallow non-deterministic scripting in configs that feed `content_hash` (see RFC §5.6 artifact format).
- Adapter runs with user privileges; no privilege escalation.

## Versioning & compatibility

- Protocol integer `1` ships with the initial release.
- SDKs declare compatible `poly` / protocol range in package metadata.
- `polyapi doctor` reports Go version, protocol, adapter path, `capabilities`.
- Host constant: `delegate.Version` (`POLY_DELEGATE_VERSION` / `POLY_DELEGATE_PROTOCOL` = 1).
