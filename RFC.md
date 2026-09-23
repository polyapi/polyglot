# RFC: PolyAPI Polyglot CLI (`polyapi`)

| Field | Value |
| --- | --- |
| **Status** | Draft |
| **Author** | Aaron Goin |
| **Created** | 2026-09-18 |
| **Updated** | 2026-09-22 |
| **References** | `polyapi-typescript`, `polyapi-python`, `polyapi-java`, `poly-flow`, [docs.polyapi.io](https://docs.polyapi.io/) |

## 1. Summary

Ship a **single global `polyapi` CLI** (Golang, prebuilt binaries) that:

1. Exposes a **uniform command tree for every PolyAPI resource**.
2. Implements **Glide v2**: git-native prepare / plan / push / pull for **all** resource types, with metadata-first discovery (`polyConfig`), deploy receipts, and smarter live-vs-local sync.
3. **Delegates language-specific work** (codegen, transpile, detailed parse, docs writers) to project-installed SDKs. The Go host may keep a **small** language table (file extensions, comment syntax, a `polyConfig` text marker).
4. **Absorbs `poly-flow`** semantics (multi-resource syncers, safety, plan/env-check) and deprecates the parallel `poly-flow` entrypoint after parity.

---

## 2. Motivation

### Today

| Surface | Reality |
| --- | --- |
| TypeScript | Richest CLI (`setup`, `generate`, `prepare`, `sync`, `function`, `model`, `snippet`, `tenant`) |
| Python | Partial parity; Glide prepare/sync; no `model` / snippet / tenant |
| Java | Maven goals only; create-variable + delete; no Glide |
| `poly-flow` | Strong multi-resource push/pull/plan, but Python add-on, path-centric discovery, parallel brand |
| Platform resources | Vari, Tabi, Jobs, Webhooks, Triggers, Schemas, Subscriptions, etc. mostly UI/Swagger |

### Pain

- Three (plus flow) mental models for the same platform.
- Language-agnostic work reimplemented per SDK.
- Glide only covers a subset of resources; flow covers more types but weaker discovery/receipts/docs.
- Env promote / train / resource CRUD inconsistent or missing from CLI.

### Why Golang + thin language adapters

- One installable binary (Homebrew / releases) without requiring Node/Python/Java to run agnostic commands.
- Shared HTTP, sync engine, config, safety in one place.
- Keep language expertise where it belongs: generate + parse in existing SDKs.

---

## 3. Goals and non-goals

### Goals (v1 platform)

- Global `polyapi` binary with shared config/auth and redacted diagnostics.
- Delegate protocol so `generate` / **extract** (transpile/eval `polyConfig`) / prepare writers work for **TypeScript and Python** first (Java later).
- Glide v2 core: Go finds deployables → prepare → plan → push / pull. **Receipts are optional.** There is no lazy/cached-deployable mode.
- Resource syncer registry ready to register types as packs land.
- Documented absorb path for `poly-flow`; no long-term dual tool.
- Strong **`polyapi init`** (new or existing projects): language, template, git provider, tooling, CI — dialog + flags.

### Non-goals (v1)

- Web dashboard / TUI.
- Multi-tenant / multi-base-URL in a single invocation (scriptable externally).
- Full function codegen on `pull` (may warn-and-skip initially).
- Java adapter (explicitly **last**).
- Replacing Canopy for every admin workflow on day one (CLI covers CRUD progressively).
- Plugin marketplace (architect for registry; don’t ship third-party plugin API in v1).
- **Systems picker / AI-assisted OOB catalogue + skills/agents during `polyapi init`** (post-v1; see §5.9).

---

## 4. Product principles

1. **Every resource** gets `list|get|create|update|delete` (+ type-specific verbs, including `init` for a local JSONC scaffold).
2. **Glide v2** supports every resource type as a deployable.
3. **Discovery is metadata-first** (`polyConfig` / language equivalent) and **runs in the Go host**. Path globs / JSONC are secondary. Adapters do not walk the tree.
4. Keep **prepare** (auto-docs / AI with review). **Deploy receipts are optional** (off by default).
5. Sync is **tool-smart** (content hashes + remote; three-way when receipts are on), not “developer must know live state.”
6. **Absorb flow**, deprecate parallel entrypoint after parity.
7. **Golang owns agnostic work and Glide**; language SDKs own transpile / detailed parse / codegen. Language CLIs (`npx poly`, `python -m polyapi`) keep only commands that need that work (`generate`, `function add`, …). They do not ship a second deploy pipeline.

---

## 5. Architecture

### 5.1 System context

```text
┌────────────────────────── Developer / CI ────────────────────────────┐
│  Git repo (SoT)                                                      │
│   *.ts/*.py + polyConfig · optional artifacts/** · .poly/ cache      │
│                              │                                       │
│                              ▼                                       │
│                     ┌─────────────────┐                              │
│                     │  polyapi (Golang) │  auth, config, CRUD         │
│                     │  polyglot       │  deploy, generate, init      │
│                     └────────┬────────┘                              │
│               ┌──────────────┼──────────────┐                        │
│               ▼              ▼              ▼                        │
│            poly-api     poly-glide      poly-delegate                │
│            (HTTP)       (pipeline)      (lang IPC)                   │
│                              │              │                        │
│                              │              ▼                        │
│                              │     TS / Python SDK adapters          │
│                              │     generate · parse · prepare write  │
└──────────────────────────────┼───────────────────────────────────────┘
                               │ HTTPS
                               ▼
                     PolyAPI instance (na1/eu1/dev/…)
```

### 5.2 Package layout

**Start with a single Go module.** Production code lives under `src/` (`src/cmd/polyapi` + `src/cli`, `src/config`, …). Tests live under `tests/`, next to `src/`, not beside production files. Use packages, not extra modules, until compile times or a real publish boundary force a split.

Suggested package map (names can evolve):

| Package | Responsibility |
| --- | --- |
| `src/cli` | Cobra command tree, human UX |
| `src/config` | Config, encrypted secrets, errors, tracing, instance URL map |
| `src/api` | Typed REST client (`Client` seam, à la flow) |
| `src/delegate` | Language detection + process protocol |
| `src/glide` | Find deployables, prepare orchestration, plan/push/pull, optional receipts, syncer registry |
| `src/model` | OpenAPI → Spec Input → validate/train |
| `tests/` | External tests and protocol fixtures |

Split into extra modules **only** when we feel pain (slow incremental builds, or a need to publish a library for other tools). Until then, keep the dependency graph as ordinary Go package visibility.

**Repository:** [`polyapi/polyglot`](https://github.com/polyapi/polyglot)
**CLI binary / command:** `polyapi` (not `poly` — avoids colliding with the existing language SDK `poly` / `python -m polyapi` entrypoints in docs; global tool is explicitly the polyglot CLI).

Versioning: **semver** for the binary; delegate protocol versioned separately (`POLY_DELEGATE_VERSION`).

### 5.3 Config and auth

**Precedence**

1. Explicit CLI flags  
2. Environment (`POLY_API_KEY`, `POLY_API_BASE_URL`, …) — preferred for CI  
3. Project `./.poly/config.toml`  
4. User config (`~/.config/poly/config.toml` / XDG)

**Secrets at rest**  
Project-stored API keys are **encrypted**. Decryption requires:

- a key material **embedded/derived in the CLI binary**, and  
- a **second secret on the user device** (OS keychain or machine-local file),

so theft of the project file alone is insufficient.

Exact KDF/cipher is an open design question (recommend OS keychain where available; file-backed fallback documented). Env vars remain plaintext in process env (CI norm) and are never written back to disk by `polyapi config show|get|set`.

**Legacy readers** (migrate, don’t break): TS `node_modules/.poly/.config.env`, Python `polyapi/.config.env`, Java `.mvn/settings.xml` (document; best-effort read).

**Commands:** `auth login` / `auth logout` / `auth whoami` and `config show|get|set` (redacted). There is **no** `setup` command; credentials use `auth login`. `init` and `auth login` ensure `.poly/` is listed in `.gitignore` (create the file if the project is a git repo and none exists).

**Committed deploy policy** (see §5.6): branch→`[environments.*]` targets, path include/exclude, types/contexts — same knobs as flow CLI/CI. Secrets never in that file. `polyapi init` scaffolds a safe default (e.g. `main`→prod only, `unallowed_branch = error`).

**Instances:** `na1`, `eu1`, `develop`/`dev`, `local` (+ custom URL).

### 5.4 REST client

Flow’s `PolyClient` Protocol is the design seam:

```text
list(resource) / get(resource, id) / create / update / delete
```

- Auth bearer from config  
- Retries with backoff on transient errors  
- Typed errors; map 401/403 to actionable messages (preserve TS permission-gate UX at call sites)  
- Fake in-memory client for unit tests (no live platform in default CI)

Early collections: functions (server/client/api), variables, webhooks, tables, jobs, schemas, triggers, snippets, applications.

### 5.5 Language delegate protocol

This section is the **normative spec** for how the global Golang `polyapi` binary collaborates with project-installed language SDKs. Adapters implement this protocol; contract tests freeze it.

#### 5.5.1 Split of responsibility

| Concern | Owner | Examples |
| --- | --- | --- |
| CLI UX, flags, help, exit UX | Golang `polyapi` | Cobra tree, colors, `--env`, branch gates |
| Config / auth / encrypted secrets | Golang | `auth login`, `auth whoami`, `config show\|get\|set`, `[deploy]` policy |
| HTTP to PolyAPI | Golang `api` | CRUD, train, replicate |
| **Find deployables** | **Golang `glide`** | Walk the repo; JSONC parse; language-light table (extensions, comment syntax, `polyConfig` text marker) |
| Glide orchestration | Golang `glide` | validate / plan / push / pull, hashes, optional receipts, syncer registry |
| OpenAPI model generate/validate/train | Golang `model` | ported from TS (`polyapi model generate\|validate\|train`) |
| **Codegen** | **Language adapter** | `generate` → SDK sources / types |
| **Extract canonical payload** from a **code** module | **Language adapter** | transpile/eval typed `polyConfig` (TS compiler, Python). JSONC is extracted in Go. |
| **Prepare writers** (docs/JSDoc/docstrings) | **Language adapter** | rewrite source files |
| **Parse `function add` source** | **Language adapter** | args/types/deps → DTO for Golang to POST |
| Optional in-file receipts | Golang | Comment header using the language-light comment table, when receipts are enabled |
| Imperative agnostic CRUD | Golang | `vari create` when not going through Glide files |

Rule of thumb: if it requires **transpiling, detailed parse, or generating the project language**, it is delegated. If it is **HTTP, policy, finding files, or multi-resource orchestration**, it stays in Golang. The host may know **very little** language trivia (`.ts` / `.py` extensions, `//` vs `#`, a `polyConfig` substring) — not an AST.

#### 5.5.2 Adapter discovery

**Resolution order**

1. `--adapter <command…>` / `--lang <ts|python|java>` override  
2. `./.poly/config.toml` → `language = "typescript"` and optional `adapter.command`  
3. Heuristics: `package.json` + `polyapi` dep → TypeScript; `pyproject.toml` / `requirements*` + `polyapi` → Python; Maven `io.polyapi` → Java (later)  
4. Interactive prompt (or error in `--non-interactive`)

**Default launch commands (v1)**

| Lang | Command |
| --- | --- |
| TypeScript | `npx --no-install poly adapter` (or `node node_modules/polyapi/build/adapter.js`) |
| Python | `python -m polyapi adapter` |
| Java | TBD (later) |

Adapters MUST print protocol version on `capabilities` (below). Golang rejects mismatches with exit `11`.

#### 5.5.3 Transport

- **Invocation:** Golang spawns the adapter as a **subprocess** (no long-lived daemon in v1).  
- **stdin:** one UTF-8 JSON **request** document (or `@path` via `--request-file` for large payloads).  
- **stdout:** one UTF-8 JSON **response** document (pretty or compact). Progress MAY use JSON-lines **before** the final response only if `request.stream === true`; v1 default is single request/response.  
- **stderr:** human-readable logs only (never secrets).  
- **cwd:** project root.  
- **env:** inherits the process environment (`NO_COLOR`, user `POLY_*` if already set); sets `POLY_DELEGATE=1`, `POLY_DELEGATE_PROTOCOL=1`. Does not inject resolved config credentials.  
- **timeout:** configurable (default 300s for `generate`, 120s for extract/discover).  

#### 5.5.4 Protocol version

```json
{ "protocol": 1 }
```

Bump `protocol` on breaking changes. Adapters advertise `protocol` and `min_protocol` in `capabilities`.

#### 5.5.5 Request / response envelope

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

#### 5.5.6 Operations (`op`)

| `op` | Direction | Purpose | `params` (summary) | `result` (summary) |
| --- | --- | --- | --- | --- |
| `capabilities` | Golang→adapter | Probe version & supported ops | `{}` | `{ protocol, min_protocol, ops[], lang, sdk_version }` |
| `generate` | Golang→adapter | Render local SDK from host-fetched specs | `{ specs: Specification[], no_types? }` | `{ files_written[], stats }` |
| `discover` | optional | Unused by `polyapi deploy` (Go walks the tree). Language CLIs MAY keep it. | `{ paths?, exclude_paths?, types? }` | `{ deployables: DeployableDesc[] }` |
| `inspect` | Golang→adapter | Parse signatures + current docs for prepare | `{ files[] }` | `{ items: [{ file, type, code, description, arguments, returns, disable_ai }] }` |
| `extract` | Golang→adapter | Canonical REST DTO for one **code** module | `{ file, type }` | `{ payload, content_hash, meta }` |
| `prepare` | Golang→adapter | Write JSDoc/docstrings from host-supplied `docs` | `{ files: [{ file, docs? }], disable_docs }` | `{ changed_files[], skipped[] }` |
| `write_receipt` | optional | Unused when Go writes comment headers itself. | `{ file, receipt: Receipt }` | `{ ok: true }` |
| `parse_function` | Golang→adapter | `function add` support | `{ file, name, kind: server\|client }` | `{ dto fields for API }` |
| `clear` | Golang→adapter | Clear generated SDK (Python today) | `{}` | `{ ok: true }` |

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

Golang owns correlating descriptors with remote state; the adapter MUST NOT call Poly HTTP. Golang fetches `GET /specs` and description-generation and passes results into adapter ops. Credentials from resolved config are not injected into the adapter process.

#### 5.5.7 Which user commands call the adapter

| User command | Golang | Adapter ops |
| --- | --- | --- |
| `polyapi generate` | `GET /specs`, permission gate | `generate` (specs already fetched) |
| `polyapi deploy prepare` | walk tree; AI description HTTP | `inspect` then `prepare` on **code** candidates; JSONC is a no-op |
| `polyapi deploy validate` | walk + JSONC extract; structural + cross-ref | `extract` per code file |
| `polyapi deploy plan` / `polyapi deploy push` | walk, orchestrate syncers, HTTP | `extract` per code file |
| `polyapi deploy pull` | HTTP fetch; write JSON under the repo root | none in v1 (function codegen pull skipped) |
| `polyapi function add` | HTTP create/update | `parse_function` then optional `generate` |
| `polyapi vari create` (imperative) | HTTP only | none |
| diagnostics (`version`, `doctor`) | checks | `capabilities` |

Language CLIs (`npx poly`, `python -m polyapi`) keep **language-specific** commands (`generate`, `function add`, local prepare writers). They should exec global `polyapi` for `deploy`, `auth`, `config`, and resource CRUD when it is on `PATH`. They do not reimplement Glide.

#### 5.5.8 Exit codes (subprocess)

| Code | Meaning |
| --- | --- |
| 0 | Success (`ok: true`) |
| 2 | Bad request / usage |
| 3 | Auth/config error inside adapter |
| 4 | Network error (`generate`) |
| 10 | Adapter binary/module missing (Golang-side before spawn) |
| 11 | Protocol mismatch |
| 12 | Partial failure (some files); see `result` + `warnings` |
| 1 | Unclassified adapter failure |

#### 5.5.9 Security & determinism

- Never log API keys; redact values marked secret in extract output when printing.  
- `extract` hashing MUST be deterministic given the same file + env token materialization rules. Go hashes JSONC itself.  
- Disallow non-deterministic scripting in configs that feed `content_hash` (see §5.6 artifact format).  
- Adapter runs with user privileges; no privilege escalation.

#### 5.5.10 Versioning & compatibility

- Protocol integer `1` ships with the initial release.  
- SDKs declare compatible `poly` / protocol range in package metadata.  
- Diagnostics (`polyapi version`, later `doctor`) report Golang version, protocol, adapter path, `capabilities`.  

Normative copy: [`docs/delegate-protocol.md`](docs/delegate-protocol.md).

### 5.6 Glide v2

#### Commands

Glide verbs live **only** under `polyapi deploy`. There are **no** top-level aliases (`prepare`, `validate`, `plan`, `push`, `pull`, `sync`, `env-check`).

| Command | Role |
| --- | --- |
| `polyapi deploy prepare` | Find deployables; auto-docs/AI; reviewable edits; always re-scan (no `--lazy`); exit non-zero if files changed (hook-friendly) |
| `polyapi deploy validate` | **Preflight** (supersedes flow’s `env-check`): env tokens, required metadata, cross-resource integrity |
| `polyapi deploy plan` | Dry-run push (CI); may assume or re-run validate |
| `polyapi deploy push` | Apply local → remote |
| `polyapi deploy pull` | Remote → local for supported types |

Soft-deprecated aliases **under `deploy` only** (not top-level): `deploy sync` → `deploy push` (see §7 Q2); `deploy env-check` → `deploy validate --only env`.

AI **off** during push/CI (`--disable-ai` / `DISABLE_AI`).

#### Artifact authoring format (lean into `polyConfig`)

**Primary (recommended):** every deployable — functions **and** variables, webhooks, jobs, tables, schemas, triggers, etc. — is authored as **source code** in the project language, using typed `polyConfig` (or language equivalent) for editor **autocomplete**, optional in-file **deploy receipts**, and optional light **scripting**.

```typescript
// Poly deployed @ … - billing.apiKey - https://na1… - a1b2c3d
import { PolyVariable } from 'polyapi';

export const polyConfig: PolyVariable = {
  name: 'apiKey',
  context: 'billing',
  visibility: 'ENVIRONMENT',
  secret: true,
  value: process.env.BILLING_API_KEY!, // or '{{BILLING_API_KEY}}'
};
```

```python
# Poly deployed @ …
from polyapi.typedefs import PolyVariable

polyConfig: PolyVariable = {
    "name": "apiKey",
    "context": "billing",
    "visibility": "ENVIRONMENT",
    "secret": True,
    "value": "{{BILLING_API_KEY}}",
}
```

**Why this wins**

| Concern | Code + typed `polyConfig` | Raw JSON / JSONC |
| --- | --- | --- |
| Autocomplete / typecheck | Native (TS types / TypedDict) | Weak / schema plugins |
| Optional deploy receipts | Same comment style as functions | JSONC header or sidecar |
| Discovery | Already Glide’s model | Path/glob secondary |
| Scripting | Computed defaults, shared consts, env reads | Limited |
| Flow JSON repos | — | Keep as **compat** import/secondary discovery |

**Scripting rules (keep sync deterministic):** allow importing shared constants and reading env / `{{TOKEN}}` placeholders; discourage non-deterministic values (random, clock) in payloads that are hashed. `prepare`/`validate` evaluate or extract config via the language delegate into a canonical JSON payload for diff/push.

**Secondary:** JSONC artifacts (flow layouts) remain supported for migration — receipt comment header allowed — but new projects from `polyapi init` scaffold **code + `polyConfig`** and SDK exports `PolyVariable`, `PolyWebhook`, `PolyJob`, … alongside today’s function types.

#### Discovery

Go **always re-scans** the project (no lazy mode, no cached deployable list).

1. **Walk** the repo with `[deploy.scope]` / `--path` / `--exclude-path` / `--types` / `--contexts`. Skip `node_modules`, `.git`, venvs, build dirs, `.poly`.
2. **JSON / JSONC:** Go parses. A file is an artifact when it has `name` and either `context`, a known `type` / `polyType`, or a path under `artifacts/`.
3. **Code:** a file is a candidate when its extension matches the project language (`.ts` / `.tsx` / `.js` / … or `.py`) **and** the file **binds** `polyConfig` (`export const polyConfig =`, `polyConfig: T =`, … — not a string mention). Payload comes from adapter `extract` (transpile / eval). Go does not parse TS or Python. Skip `tests` / `fixtures` / `node_modules` / build dirs.
4. Never require `src/**/server/*.py`-style paths to be found.

#### Receipts (optional)

Off by default. Enable with `deploy.receipts = true` in `.poly/config.toml` or `polyapi deploy push --receipts`.

When off, plan/push is **two-way** (local content hash vs remote). Deleted locals show up as orphans (`--delete-orphans` to remove).

When on, Go writes a comment header using the language-light comment table (`//` vs `#`) and a `.poly/receipts.json` cache. Compare becomes **three-way** (local / receipt / remote): conflict when remote drifted; deleting a file we previously pushed removes that remote without `--delete-orphans`.

#### `polyapi deploy validate` (expanded preflight)

Replaces a narrow `env-check` with a project-wide integrity check. Intended for local pre-commit and CI before `deploy plan` / `deploy push`.

**Checks (v1 target — grow with resource packs):**

| Category | Examples |
| --- | --- |
| Env / secrets | All `{{ENV_TOKEN}}` placeholders satisfied; warn on unused tokens; never print values |
| Metadata | Missing `name` / `context` / `description` / visibility where required; empty docs after prepare expectations |
| Discovery | `polyConfig` present but invalid; unknown type; duplicate `context.name` |
| Cross-resource | Webhook/trigger references a missing server function; job function list IDs/`context.name` unresolved; schema refs dangling; security function on webhook missing |
| Config | Project lang/adapter detectable; auth present (without dumping secrets) |
| Optional remote | `--online`: confirm referenced remote IDs still exist (offline by default for speed) |

**UX:** severity levels `error` / `warn` / `info`; `--strict` treats warnings as failures; exit `0` only if no errors (and no warnings if strict). Keep `polyapi deploy env-check` as a **soft-deprecated alias** of `polyapi deploy validate --only env` for flow migrants. There is no top-level `env-check`.

#### Sync intelligence (three-way)

Default is **two-way** (local hash vs remote). The table below applies when receipts are enabled.

For each deployable and target instance:

| Local hash | Receipt hash | Remote hash | Action |
| --- | --- | --- | --- |
| new | missing | missing | create |
| changed | old | = receipt | update |
| = receipt | = receipt | = receipt | skip |
| = receipt | = receipt | changed | conflict or pull-back |
| missing file | present | present | remove (plan/push) |
| — | — | present, no local | orphan (report / `--delete-orphans`) |

- Content hash is **primary**; mtime/`lastUpdatedAt` secondary.  
- Receipts (optional): in-file header and/or sidecar + `.poly/` cache; **per-instance** records (id, deployedAt, hash, canopy URL).  
- Field-level diffs in plan/verbose (flow UX).  
- Resume checkpoints (`--resume`); context / path / type filters. No prepare/discover cache.

#### Syncer registry

`ArtifactSyncer` trait + `@register`-style map (port flow semantics). Packs register types over time.

**Baseline deploy order:**

```text
variables → tables → schemas
  → ai functions → api functions → client functions → server functions
  → webhooks → triggers → jobs
```

Orphan cleanup runs **reverse**. Evolve later to a **dependency DAG** from parsed Poly refs.

#### Safety (branch-aware deploy gates)

Accidental `push` from a feature branch against a production API key is a primary risk. Flow’s “confirm when *on* prod branch” is necessary but not sufficient — we also need project policy for **which branches may deploy at all**.

**Top-level project config** should mirror the knobs teams already use with **flow CLI / CI** (path scoping, type filters, context maps, and mapping git branches → Poly environments)—not only a crude “main vs not.”

Committed policy lives in `./.poly/config.toml` (no secrets). Secrets for each named environment stay in env / CI secrets / encrypted login store.

```toml
# --- named Poly environments (URLs only here; keys via env/CI) ---
[environments.dev]
base_url = "https://na1.polyapi.io"
# api_key from POLY_API_KEY or POLY_ENV_DEV_API_KEY — never commit

[environments.prod]
base_url = "https://na1.polyapi.io"
# production gate applies when this env is selected

# --- branch → environment + who may deploy ---
[deploy]
unallowed_branch = "error"   # off | warn | error
require_git = true

[[deploy.targets]]
branches = ["main"]
environment = "prod"
production = true            # typed confirm / CI exec key

[[deploy.targets]]
branches = ["develop", "release/*"]
environment = "dev"

# --- scope (same ideas as flow --paths / --skip-path / --types / contexts) ---
[deploy.scope]
# Include only these repo paths (empty = whole repo). CLI --path adds to this.
paths = []
# Always skip these paths (flow --skip-path / CI exclude sections)
exclude_paths = ["src/experiments/**", "docs/**"]
# Artifact types enabled for push/plan (flow --types / enabled_types)
types = [
  "variables", "tables", "schemas",
  "ai-functions", "api-functions", "client-functions", "server-functions",
  "webhooks", "triggers", "jobs",
]
# Optional context-prefix filter
contexts = []
# Context prefix → local path roots (flow STORK_CONTEXT_PATH_MAP)
# context_path_map = { "integrations.crm" = "src/crm" }
# Contexts excluded from orphan delete (flow POLY_DEPLOY_EXCLUDE_ORPHAN_CONTEXTS)
exclude_orphan_contexts = []
```

**CLI / Action parity** (flags override or narrow config for one run):

| Concern | Config | CLI (flow today → `polyapi deploy`) |
| --- | --- | --- |
| Poly env / instance | `deploy.targets[].environment` + `[environments.*]` | `--env prod` / `POLY_API_BASE_URL` |
| Branch policy | `deploy.targets` + `unallowed_branch` | (implicit from git) |
| Include paths | `deploy.scope.paths` | `--path` (flow `--paths`) |
| Exclude sections of repo | `deploy.scope.exclude_paths` | `--exclude-path` (flow `--skip-path`) |
| Resource types | `deploy.scope.types` | `--types` |
| Contexts | `deploy.scope.contexts` | context args |
| Orphan excludes | `exclude_orphan_contexts` | (env today) |

A future **`polyapi` GitHub Action** should expose these same inputs (env name, paths, exclude_paths, types, contexts, dry_run) instead of only `poly_api_key` + `base_url` like the current TS deploy action.

**Behavior**

| Situation | Default |
| --- | --- |
| Branch matches no `deploy.targets` | **Block** mutating deploy (`unallowed_branch = error`) |
| Target has `production = true` | Typed `PROD` confirm and/or CI exec key |
| Branch matches target | Select that `[environments.*]` (base URL); key from CI/env for that name |
| `deploy plan` / `deploy validate` / `deploy prepare` | Allowed on any branch; validate reports whether push would be blocked and which env would be used |
| Path/type/context filters | Applied consistently in discover → validate → plan → push |

Also retain from flow:

- Secret redaction; never deploy secret variable values from source  
- Pull path confinement under repo root  

### 5.7 Imperative CRUD vs Glide

Both exist:

- **Imperative:** `polyapi vari create …`, `polyapi function delete …` — one-off / scripting.  
- **Declarative Glide:** author `polyConfig` (or marked JSON) → `deploy prepare` → `deploy plan` → `deploy push` — team/CI SoT.

Glide is preferred for multi-resource projects; imperative for spikes and admin.

### 5.8 Absorbing `poly-flow`

`polyapi` unifies the capabilities that today live in Glide and `poly-flow`. After resource-pack parity, the parallel `poly-flow` entrypoint is deprecated in favor of a single CLI.

| Absorb | Leave / improve |
| --- | --- |
| Syncer registry, deploy order, plan, **validate** (env-check⊂), prod gate, resume, orphans, redaction, `PolyClient` seam, JSON payload shapes, env-token substitution | Path/glob as **primary** discovery; parallel `poly-flow` brand; mtime-primary sync |

See Appendix A for a capability-level Glide vs flow map.

### 5.9 `polyapi init` — project scaffolder

`init` is not a thin “write `.poly/config.toml`” helper. It is how developers **start or adopt** Poly projects: new empty repo **or** inject Poly into an existing codebase.

**Modes**

| Mode | Behavior |
| --- | --- |
| New project | Create directory (optional), clone/instantiate template, git init / remote |
| Existing project | Detect language/package manager; merge Poly deps, `.poly/`, hooks, CI without clobbering (prompt on conflicts) |

**Interactive dialog** (non-interactive via flags / `--yes` for CI):

1. **Language** — TypeScript / Python first; Java later  
2. **Mode** — new vs existing (cwd)  
3. **Template** — default official (`polyapi/poly-glide-template-js` / `-py`) or custom git URL/ref  
4. **Git provider** — GitHub / GitLab / other (affects CI workflow flavor)  
5. **Project name / package fields**  
6. **Environments** — e.g. map `main`→prod, optional `develop`→dev; collect instance URL(s) (keys via later `auth login` or printed secret instructions)  
7. **Deploy policy** — scaffold `deploy.targets` / `deploy.scope`  
8. **Tooling** — package manager install, pre-commit / husky (`deploy prepare` on commit), editor hints  
9. **CI/CD** — write workflow using future `polyapi` Action (env secrets names, branch filters, path excludes placeholders)  
10. **Next steps** — print `polyapi auth login`, `polyapi generate`, secret names for the git host  

**Instantiation**

- Fetch template (git sparse/clone or zip) at pinned ref  
- Substitute placeholders (name, description, author)  
- For existing projects: additive merge (deps + scripts + `.poly/` + optional workflow path)  
- **Ensure `.poly/` is gitignored** (see below)  
- Run language package install when network allowed  
- Optionally call `polyapi auth login` if key/url provided  

**`.gitignore` / `.poly/` (init and auth login)**

Project `.poly/` may hold encrypted keys, caches, and local state — it must not be committed.

| Situation | Behavior |
| --- | --- |
| `.gitignore` exists | Append `.poly/` if not already ignored (idempotent; don’t duplicate) |
| Git repo, no `.gitignore` | Create `.gitignore` containing at least `.poly/` |
| Not a git repo | Skip (or warn); create ignore when `git init` runs as part of new-project flow |

Both `polyapi init` and `polyapi auth login` perform this check so existing projects that only run `auth login` still get protected.

**Reference templates today:** `polyapi/poly-glide-template-js`, `polyapi/poly-glide-template-py` (husky/pre-commit, `.github/workflows/deploy.yml`). `init` should automate the manual README steps those templates document (remote rewrite, secrets checklist, permissions, first generate).

**Flags (sketch):** `--lang`, `--template`, `--provider github|gitlab`, `--name`, `--existing`, `--env-map main=prod`, `--non-interactive`.

**Post-v1 (explicitly out of v1 scope):** during `polyapi init`, ask which **systems / domains** the project will integrate with (HubSpot, Stripe, OHIP, …).

| Input | Behavior |
| --- | --- |
| Pick from curated list | Multi-select known vendors/domains |
| Free-text (“hotel ops + payments”) | Optional AI assist to map to catalogue contexts / skills |

Then optionally:

- Scope or hint `polyapi generate --contexts …` toward matching **OOB catalogue** functions  
- Drop starter **AI skills / agent** packs (Cursor/IDE or repo-local) for those systems  
- Seed example `polyConfig` stubs that call relevant APIs  

v1 `init` stays focused on project/tooling/CI; systems→catalogue/skills is a later enhancement after generate + init stabilize.

---

## 6. Command surface

Classification: **A** agnostic · **H** hybrid · **L** language-specific.

Top-level commands are kept small. Workflow clusters nest under `auth`, `config`, and `deploy`. Resource roots stay top-level. There is **no** `setup` command (credentials use `auth login`). There are **no** top-level aliases for deploy verbs.

### Top-level (v1)

| Command | Class | Role |
| --- | --- | --- |
| `init` | A/H | Project scaffolder (new or existing) |
| `auth` | A | Credentials and identity (see below) |
| `config` | A | Non-secret project/user settings (see below) |
| `generate` | L→delegate | Local SDK codegen |
| `deploy` | H | Glide pipeline (see below) |
| `logs` | A | Log query; filters today, env/app aggregation later |
| `function` | H | Imperative function CRUD / execute (`--type server\|client\|api\|ai`); `init` writes a local JSONC scaffold |
| `vari` | A | Variables (`list` / `get` / `init` / `create` / `update` / `delete` / `copy`) |
| `table` | A | Tables + row operations (`list`/`get`/`init`/`create`/`update`/`delete`; `rows` insert/upsert/update/delete/query/count) |
| `webhook` | A/H | Webhooks (`list`/`get`/`init`/`create`/`update`/`delete`/`test`/`url`) |
| `trigger` | A/H | Triggers (`list`/`get`/`init`/`create`/`update`/`delete`; webhook → server function) |
| `job` | A | Jobs (`list`/`get`/`init`/`create`/`update`/`delete`/`enable`/`disable`/`run`; `executions` list/get/delete) |
| `schema` | A | Schemas (`list`/`get`/`init`/`create`/`update`/`delete`) |
| `snippet` | A | Snippets (`list`/`get`/`init`/`add`/`update`/`delete`; `add` is PUT upsert) |
| `app` / `canopy` | A | Canopy applications (`list`/`get`/`init`/`create`/`update`/`delete`/`url`). Stretch in the original v1 cut; APIs are sufficient (POLY-CLI-23). |
| `version` | A | CLI version |

### Nested groups

**`auth`**

| Subcommand | Notes |
| --- | --- |
| `login` | Instance URL + API key (replaces today’s SDK `setup`) |
| `logout` | Clear stored credentials |
| `whoami` | Show instance + redacted identity / key metadata |

**`config`**

| Subcommand | Notes |
| --- | --- |
| `show` / `get` / `set` | Redacted; never print raw secrets |

**`deploy`** (Glide v2 — only path for these verbs)

| Subcommand | Notes |
| --- | --- |
| `prepare` | Discover; auto-docs/AI |
| `validate` | Preflight; `env-check` → deprecated alias of `validate --only env` |
| `plan` | Dry-run push |
| `push` | Local → remote; `sync` → soft-deprecated alias of `push` |
| `pull` | Remote → local |

### Later / stretch (not v1 top-level)

| Command | Class | Notes |
| --- | --- | --- |
| `doctor` / `update` | A | Diagnostics / self-update (POLY-CLI-24); not part of the `auth` / `config` / `deploy` grouping |
| `model generate\|validate\|train` | A | OpenAPI suite |
| `replicate` / `env …` | A | Environments / promote |
| `subscription` | A | GraphQL subscriptions (POLY-CLI-19) |
| `error-handler` | A/H | later |
| `user` / `key` / `permissions` | A | Auth admin (later) |
| `tenant` | A | admin |

---

## 7. Open design questions

| # | Question | Proposal |
| --- | --- | --- |
| Q1 | Artifact format for non-function resources? | **Code + typed `polyConfig`** (autocomplete, light scripting); JSONC compat for flow migration; receipts optional |
| Q2 | `polyapi deploy sync` alias? | Soft-deprecated alias of `polyapi deploy push` for ≥2 majors; **not** a top-level `sync` |
| Q3 | Function `pull` codegen in v1? | Warn-and-skip; JSON metadata pull only |
| Q4 | Multi-language in one project? | One primary `lang` per `.poly` project for v1 |
| Q5 | Repo name? | **`polyapi/polyglot`** (decided) |
| Q5b | Global CLI command name? | **`polyapi`** (decided) |
| Q6 | Encryption: keychain-only vs file fallback? | Keychain preferred; file fallback for headless CI is N/A (use env) |
| Q7 | License? | MIT to match SDKs |
| Q8 | Default `deploy.targets` for greenfield `polyapi init`? | `main`→prod only + `unallowed_branch = error` |
| Q9 | Where is the flow GitHub Action source of truth for inputs (if beyond CLI flags)? | Align Action inputs 1:1 with `deploy.scope` + `--env` |
| Q10 | Post-v1: source of truth for systems↔contexts/skills mapping (Canopy catalogue metadata vs hand-curated)? | TBD. Application *records* are REST (`/applications`, `/applications/{id}/config`); POLY-CLI-23 did not defer that lifecycle. Catalogue-as-systems-picker remains post-v1 (POLY-CLI-28). |

---

## 8. Appendix A — Glide vs flow (capability map)

| Concern | Glide today | Flow today | v2 choice |
| --- | --- | --- | --- |
| Discovery | `polyConfig` scan | Path globs | **Glide primary**, globs secondary |
| Docs | `prepare` + AI | None | **Glide prepare** |
| Live tracking | Receipts + cache revisions | Content diff + mtime | **Receipts + content hash** (+ field diff UX from flow) |
| Resource width | Functions (+ TS webhooks) | 8 types | **Flow width** via registry |
| Plan/CI | dry-run sync | `plan` / dry-run | **`polyapi deploy plan`** |
| Preflight | (light) | `env-check` | **`polyapi deploy validate`** (env + metadata + cross-refs) |
| Safety | lighter | prod gate, redaction, OTP hooks | **Flow safety + branch allowlist** (block feature-branch push to prod) |
| Pull | limited | first-class | **First-class pull** |
| Brand | `poly` / Glide | `poly-flow` | **Single `polyapi` CLI** |
