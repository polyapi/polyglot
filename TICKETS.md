# PolyAPI Polyglot CLI (`polyapi`) — Ticket Drafts

**Status:** Draft catalogue; Epic overview **Status** is the tracker  
**Last updated:** 2026-09-23 (PT)  
**Owner context:** Head of Engineering / SDK owners (TS, Python, Java)  
**Related sources:**
- Existing SDKs: `polyapi-typescript`, `polyapi-python`, `polyapi-java`
- Docs: https://docs.polyapi.io/
- Reference impl (absorb, then deprecate entrypoint): `/Users/aarongoin/Dev/poly-flow` (`polyapi/poly-flow`)
- **RFC (POLY-CLI-1):** [`RFC.md`](./RFC.md)
- **Repo:** `polyapi/polyglot` · **CLI command:** `polyapi`

## Product principles (apply to all tickets)

1. **Every PolyAPI resource** gets a first-class CLI command tree (`list|get|create|update|delete` + type-specific verbs, including `init` for a local JSONC scaffold).
2. **Glide v2** supports **every** resource type as a deployable — not only server/client functions.
3. **Discovery is metadata-first:** scan for `polyConfig` (or language equivalent) for **all** resource types—not only functions. Prefer **code modules** with typed config for editor autocomplete; path/glob JSON(C) is optional secondary (flow compat).
4. Keep Glide’s **prepare** (auto-documentation / AI fill-in with review) and **deploy receipts**.
5. Sync intelligence is Glide-style (receipts + revisions + live comparison), improved with content hashes and optional pull-back — not “developer must know live vs local.”
6. Absorb **poly-flow** semantics (multi-resource syncers, plan/dry-run, env-check, prod safety, orphan delete, deploy order) into `poly`; **do not** leave a parallel long-term tool.
7. **Golang `polyapi` CLI** (`polyapi/polyglot`) owns language-agnostic work; **project language SDKs** own codegen / AST / idioms via a delegate protocol. Language CLIs may mirror the command tree and call up to global `polyapi` for agnostic commands.
8. **`polyapi init`** scaffolds new or existing projects (language, template, git provider, tooling, CI)—not just a config file stub.

### Glide v2 hybrid (canonical design)

| Concern | Canonical approach |
| --- | --- |
| Discovery | Primary: `polyConfig` scan. Secondary: optional path globs for pure-JSON artifacts |
| Prepare | Auto-docs / AI; reviewable; AI off during CI push/sync |
| Sync intelligence | Receipts + per-instance deployment records + content hash; three-way local / receipt / remote |
| Resource coverage | Registry of syncers (flow-style) for all resource types |
| Deploy order | Flow baseline order, evolve to dependency DAG from parsed refs |
| Safety | `plan`, **`validate`**, **branch allowlist** + prod gates, redaction, orphan delete |
| On-disk resources | **Code + typed `polyConfig`** for all resource kinds (autocomplete, receipts, light scripting); JSONC/flow JSON as migration compat |

**Baseline deploy order (from flow):**  
`variables → tables → schemas → ai functions → api functions → client functions → server functions → webhooks → triggers → jobs`
(extend as new types are added; orphan cleanup reverse)

---

## Epic overview

Status values: **completed**, **in-progress**, **not started**. Acceptance-criteria checkboxes below are historical; do not tick them — this table is the tracker.

| ID | Title | Priority | Depends on | Status |
| --- | --- | --- | --- | --- |
| POLY-CLI-1 | Product RFC & command taxonomy | P0 | — | completed |
| POLY-CLI-2 | Golang repo & CI bootstrap | P0 | 1 | completed |
| POLY-CLI-3 | Unified config & auth | P0 | 2 | completed |
| POLY-CLI-4 | REST client foundation (`poly-api`) | P0 | 2, 3 | completed |
| POLY-CLI-5 | Language delegate protocol | P0 | 2, 3 | completed |
| POLY-CLI-25 | **Glide core v2** (polymorphic deployables) | P0 | 4, 5 | completed |
| POLY-CLI-26 | **poly-flow reconciliation & deprecation** | P0 | 25, 7–10, 18, 27 | completed |
| POLY-CLI-6 | OpenAPI `model` suite in Golang | P1 | 4 | completed |
| POLY-CLI-7 | Resource pack: Vari | P1 | 4, 25 (Glide type) | completed |
| POLY-CLI-8 | Resource pack: Tabi tables + rows | P1 | 4, 25 | completed |
| POLY-CLI-9 | Resource pack: Webhooks + triggers | P1 | 4, 25 | completed |
| POLY-CLI-10 | Resource pack: Jobs + executions | P1 | 4, 25 | completed |
| POLY-CLI-20 | Function command tree completion | P1 | 4, 5, 25 | completed |
| POLY-CLI-18 | Resource pack: Snippets + schemas | P1 | 4, 25 | completed |
| POLY-CLI-15 | Distribution (binaries, installers) | P1 | 2 | completed |
| POLY-CLI-24 | Meta + **`init` scaffolder** (doctor, version, update) | P1 | 3, 5, 15 (Action later) | completed |
| POLY-CLI-23 | Canopy / custom apps (stretch) | P1 | 4, 25 | completed |
| POLY-CLI-11 | Thin TypeScript SDK adapter | P1 | 5, 25 | completed |
| POLY-CLI-12 | Thin Python SDK adapter | P1 | 5, 25 | completed |
| POLY-CLI-22 | Error-handler resources | P2 | 4, 25 | completed |
| POLY-CLI-19 | Resource pack: GraphQL subscriptions | P2 | 4, 25 | completed |
| POLY-CLI-27 | **Glide ← poly-flow parity leftovers** | P2 | 25 | not started |

## Tickets

### POLY-CLI-1 — Product RFC & command taxonomy

**Type:** Design / RFC  
**Priority:** P0

**Description**  
Write and socialize an RFC that locks product principles above, the full resource command matrix, Glide v2 hybrid design, Golang-vs-language split, and non-goals for v1.

**Must include**
- Command tree for every platform resource (functions, API functions, vari, tabi, webhooks, triggers, jobs, subscriptions, schemas, snippets, envs, tenants, users/keys, policies, idp, canopy, logs, error-handlers, cloud events as needed)
- Classification: Agnostic / Hybrid / Language-specific per command
- Glide v2 discovery + receipts + prepare + three-way sync
- Decision to absorb `poly-flow` and deprecate its entrypoint after parity
- Naming: repo `polyapi/polyglot`, CLI command `polyapi`, config paths, env vars (`POLY_*`)

**Acceptance criteria**
- [ ] RFC reviewed by SDK owners (TS/Python/Java) and Luis (flow author) or delegate
- [ ] Explicit v1 vs v2 scope table
- [ ] Non-goals listed (e.g. web dashboard, multi-tenant single-run — defer as needed)
- [ ] Linked from this ticket set as the source of truth for taxonomy

---

### POLY-CLI-2 — Golang repo & CI bootstrap

**Type:** Engineering  
**Priority:** P0  
**Depends on:** POLY-CLI-1

**Description**  
Create the Golang repo **`polyapi/polyglot`** for the polyglot CLI as a **single Go module** with packages.

**Suggested packages** (under one module)
- `src/cmd/polyapi` — binary entry `polyapi`
- `src/cli` — Cobra command tree, brand theme, human UX
- `src/config` — config, encrypted secrets, errors, tracing
- `src/api` — REST client
- `src/delegate` — language discovery + IPC
- `src/glide` — prepare/plan/push/pull pipeline (may start stub)
- `src/model` — OpenAPI → Spec Input (can land with ticket 6)
- `tests/` — tests and fixtures, kept out of `src/`

**Acceptance criteria**
- [ ] Repo is `polyapi/polyglot`; binary name is `polyapi`
- [ ] Single Go module; `go test ./...` / `gofmt` clean on CI
- [ ] Package layout documented in README (no extra modules)
- [ ] Cross-compile workflow scaffolding for linux (amd64+arm64), macOS (arm64+x64), windows
- [ ] LICENSE, CONTRIBUTING, basic README
- [ ] Versioning strategy documented (semver)

---

### POLY-CLI-3 — Unified config & auth

**Type:** Engineering  
**Priority:** P0  
**Depends on:** POLY-CLI-2

**Description**  
Implement config discovery and auth for the global CLI. Per-project config. Keys encrypted at rest--decrypted using key embedded in CLI and using another secret key stored somewhere on user device (so requires both to decrypt).

**Precedence (proposed)**  
1. Explicit flags  
2. Env: `POLY_API_KEY`, `POLY_API_BASE_URL`, related  
3. Project: `./.poly/config.toml` (and legacy readers)  
4. User: XDG/`~/.config/poly/config.toml`

**Legacy compatibility**
- TS: `node_modules/.poly/.config.env`
- Python: `polyapi/.config.env` (INI)
- Java: `.mvn/settings.xml` / pom properties (document migration; read if feasible)

**Commands:** `polyapi setup` / `polyapi login` / `polyapi logout` / `polyapi whoami` / `polyapi config` (redacted)

**Committed (non-secret) deploy policy** in project config: `[environments.*]` + `deploy.targets` (branch→Poly env) + `deploy.scope` (paths/exclude/types/contexts) — flow CLI/CI parity; see POLY-CLI-25. Secrets stay env/CI/encrypted login only.

**Acceptance criteria**
- [ ] Works against dev/na1/eu1 with existing API keys
- [ ] Secrets never printed; redacted `polyapi config`
- [ ] Migration notes for TS/Python/Java projects
- [ ] Instance shorthand map (`na1`, `eu1`, `develop`, `local`) documented
- [ ] Honor TLS/mTLS settings where Python already supports them (or ticket follow-up)
- [ ] `setup` ensures `.poly/` is in `.gitignore` (append if present; create `.gitignore` if in a git repo and none exists); idempotent

---

### POLY-CLI-4 — REST client foundation (`poly-api`)

**Type:** Engineering  
**Priority:** P0  
**Depends on:** POLY-CLI-2, POLY-CLI-3

**Description**  
Typed HTTP client over Poly REST APIs (flow’s `PolyClient` Protocol as the design seam).

**Methods (minimum):** `list`, `get`, `create`, `update`, `delete` by resource collection; auth headers; retries; typed errors; pagination helpers as needed.

**Acceptance criteria**
- [ ] Trait/interface usable from syncers and imperative commands
- [ ] Unit tests with mock HTTP (no live platform required for core tests)
- [ ] Covers collections needed by early packs: functions, variables, webhooks, tables, jobs, schemas, triggers, snippets
- [ ] Permission / 401/403 errors surfaced clearly (preserve TS `ensurePermissions` intent at call sites)

---

### POLY-CLI-5 — Language delegate protocol

**Type:** Engineering  
**Priority:** P0  
**Depends on:** POLY-CLI-2, POLY-CLI-3

**Description**  
Implement the **normative language delegate protocol** defined in [`poly-cli-rfc.md` §5.5](./poly-cli-rfc.md) (POLY-CLI-1). Golang owns HTTP/orchestration/policy; adapters own codegen, `polyConfig` discover/extract, prepare writers, receipts, `function add` parse.

**Acceptance criteria**
- [ ] `docs/delegate-protocol.md` in CLI repo matches RFC §5.5 (protocol 1)
- [ ] Adapter ops: `capabilities`, `generate`, `discover`, `extract`, `prepare`, `write_receipt`, `parse_function` (TS + Python)
- [ ] Discovery order: `--lang` / `--adapter` → `.poly` → lockfiles → prompt
- [ ] `polyapi generate` works via delegate on sample TS and Python projects
- [ ] `discover` + `extract` return `DeployableDesc` / canonical payload for Glide
- [ ] Exit codes 0/2/3/4/10/11/12 honored; protocol mismatch → 11
- [ ] Adapters do not perform Poly HTTP except `generate` (documented exception)
- [ ] Contract fixtures in POLY-CLI-17 freeze request/response schemas
- [ ] Clear error when adapter missing or SDK too old

**Follow-up (do not reopen; lands with POLY-CLI-11/12)**  
Protocol 1 is amended before the first real adapter (no integer bump). Drop the generate HTTP exception. Host `GET /specs` and description-generation; adapters never call Poly. New `inspect` op. `generate` params are `{ specs, no_types }` (opaque spec list). `prepare` is write-only. `discover` and `write_receipt` stay optional. Details in **Language adapters — protocol 1 freeze** under POLY-CLI-11/12.

---

### POLY-CLI-6 — OpenAPI `model` suite in Golang

**Type:** Engineering  
**Priority:** P1  
**Depends on:** POLY-CLI-4

**Description**  
Port TS `polyapi model generate|validate|train` so all languages get OpenAPI → Spec Input → train without requiring the TS CLI.

**Acceptance criteria**
- [ ] Docs example at https://docs.polyapi.io/api_functions/openapi.html works via Golang `polyapi model …`
- [ ] Supports context, hostUrl, hostUrlAsArgument, rename, disable-ai (or equivalent)
- [ ] Validate catches invalid Spec Input
- [ ] Train upserts API functions and webhook handlers per existing semantics
- [ ] Parity notes vs current TS implementation

---

### POLY-CLI-7 — Resource pack: Vari

**Type:** Engineering  
**Priority:** P1  
**Depends on:** POLY-CLI-4, POLY-CLI-25 (Glide type registration)

**Description**  
Imperative CLI + Glide deployable for variables.

**Commands:** `polyapi vari list|get|create|update|delete|copy` (copy/promote as needed for env push)

**Glide:** `PolyVariable` as **code + typed `polyConfig`** (autocomplete); optional JSONC glob overlay for flow compat

**Accept from flow:** secret/obscured handling (never deploy secret values from source; redaction); JSONC + `{{ENV_TOKEN}}` substitution for JSON artifacts

**Acceptance criteria**
- [ ] CRUD parity with Canopy UI create/update/delete
- [ ] Secrets never printed or pushed from repo values
- [ ] Glide discover + plan/push/pull/receipt for variables
- [ ] Replaces “Java-only create-server-variable” as the cross-language path

---

### POLY-CLI-8 — Resource pack: Tabi tables + rows

**Type:** Engineering  
**Priority:** P1  
**Depends on:** POLY-CLI-4, POLY-CLI-25

**Description**  
CLI for `/tables` lifecycle and row operations; Glide deployable for table schema.

**Commands:**  
- `polyapi table list|get|create|update|delete`  
- `polyapi table rows list|get|insert|update|delete|query` (shape TBD from docs)

**Acceptance criteria**
- [ ] Matches docs at `tabi_tables/*`
- [ ] Glide type `PolyTable` (schema); clarify whether seed rows are in/out of Glide v1
- [ ] Column types/aliases from docs supported on create/update

---

### POLY-CLI-9 — Resource pack: Webhooks + triggers

**Type:** Engineering  
**Priority:** P1  
**Depends on:** POLY-CLI-4, POLY-CLI-25

**Description**  
Imperative + Glide types for webhooks and triggers (including security function ref rewriting from flow).

**Commands:**  
- `polyapi webhook list|get|create|update|delete|test|url`  
- `polyapi trigger list|get|create|update|delete`

**Acceptance criteria**
- [ ] Create/test webhook without Canopy
- [ ] Trigger create links webhook → server function
- [ ] Glide types with receipts; UUID ↔ `context.name` rewriting
- [ ] TS already syncs webhooks in Glide — preserve behavior and extend

---

### POLY-CLI-10 — Resource pack: Jobs + executions

**Type:** Engineering  
**Priority:** P1  
**Depends on:** POLY-CLI-4, POLY-CLI-25

**Description**  
Jobs CRUD + execution inspection; Glide `PolyJob`.

**Commands:** `polyapi job list|get|create|update|delete|enable|disable|run`; `polyapi job executions …`

**Acceptance criteria**
- [ ] Schedule types per platform (interval, cron/periodical, etc.)
- [ ] Function list references resolve `context.name` ↔ id
- [ ] Glide discover/plan/push/pull/receipt
- [ ] Docs examples replaceable with CLI

---

### Language adapters — protocol 1 freeze (POLY-CLI-11 / 12)

Shared contract for `polyapi-typescript` and `polyapi-python`. Protocol integer stays **1** (no bump; no real adapter has shipped). Normative write-up also belongs in [`docs/delegate-protocol.md`](./docs/delegate-protocol.md) and RFC §5.5 when the host lands.

**Split.** Go owns every Poly HTTP call, credentials, permission gates, file finding, Glide, receipts, and resource CRUD. SDKs own transpile / AST / codegen / comment writes. The adapter is a JSON subprocess, not a second CLI and not a second deploy pipeline.

**Network rule.** The adapter **must not** call Poly HTTP. Do not inject `POLY_API_KEY` into the adapter process. Language CLIs (`npx poly generate`, `python -m polyapi generate`) may keep their own `/specs` calls until they exec `polyapi` when it is on `PATH` — that is dual-path UX, not the adapter protocol.

**Launch (already in the host)**

| Lang | Command |
| --- | --- |
| TypeScript | `node node_modules/polyapi/build/adapter.js` when that file exists; else `node_modules/.bin/poly adapter` |
| Python | project venv `python -m polyapi adapter` when the venv has `polyapi`; else `python3 -m polyapi adapter` |

These are **host entrypoints**, not user commands. They must not appear in `npx poly --help` / `python -m polyapi --help`. Intercept `adapter` (and `POLY_DELEGATE=1`) before the SDK parser so help, command lists, and `CLI_COMMANDS` stay unchanged.

**Required ops**

| `op` | Host does | Adapter does | Params | Result |
| --- | --- | --- | --- | --- |
| `capabilities` | `doctor` / `version` | advertise protocol + ops | `{}` | `{ protocol, min_protocol, ops[], lang, sdk_version }` |
| `generate` | `GET /specs?contexts&names&ids&noTypes` (opaque JSON); `libraryGenerate` gate | render the spec list into the local library | `{ specs: Specification[], no_types? }` | `{ files_written[], stats? }` |
| `inspect` | `deploy prepare` spawn 1 | parse the given files only (no tree walk) | `{ files[] }` | `{ items: [{ file, type, code, description, arguments, returns, disable_ai }] }` |
| `prepare` | description-generation HTTP, then spawn 2 | write JSDoc / docstrings from `docs` | `{ disable_docs, files: [{ file, docs? }] }` | `{ changed_files[], skipped[] }` |
| `extract` | `deploy validate\|plan\|push` per code file | `polyConfig` + function → REST DTO | `{ file, type }` | `{ payload, content_hash, meta? }` |
| `parse_function` | `function add\|update`; host overlays flags then POSTs | named function → REST DTO (no `polyConfig` required) | `{ file, name, kind: server\|client }` | REST DTO object |
| `clear` | `polyapi clear` | delete generated library | `{}` | `{ ok: true }` |

Python **must** implement `clear`. TypeScript should (empty `.poly/lib` / generated output). `discover` and `write_receipt` stay **optional**; Go walks the tree and writes receipt comments itself.

**Host prepare sequence.** Find code files → `inspect` → for items with missing descriptions, unless `--disable-ai` / `DISABLE_AI` / per-file `disable_ai`: `POST /functions/server/description-generation` or `/functions/client/description-generation` (webhook: `/webhooks/description-generation`) with `{ description, arguments, code }` — same family as `polyapi model generate` — then `prepare` with the filled `docs`. `--disable-docs` means do not write. Missing credentials: skip AI, still write whatever inspect already had.

**`extract` / `parse_function` payload** is the REST create body Go will POST (no `functionOutbound`). CamelCase API keys:

- always: `name`, `context`, `description`, `code`, `language`, `visibility`
- functions: `arguments` (`key` + JSON Schema `type` + `description`), `returnType`, `returnTypeSchema?`, `typeSchemas?`, server `requirements` / `externalDependencies`, `logsEnabled`, `generateContexts`, `image`, `cachePolyLibrary`, `alwaysOn`, …
- omit Glide-cache fields (`deployments`, `fileRevision`, `dirty`, `docStartIndex`) and omit `type` / `kind` (`type` is not in Go’s read-only drop list and would be POSTed)

v1 extract types: **server-function** and **client-function** (what `function init` writes). TypeScript may also extract **webhook** code modules. Variables / tables / jobs / schemas / triggers / snippets / applications stay JSONC until someone authors them as `polyConfig`; exporting their `Poly*` types for autocomplete is in scope, extract coverage is not a v1 blocker.

**Library functions** (called by the adapter dispatcher): no `process.exit` / `sys.exit`, no repo walk, no HTTP, no deploy POST. Current `generate` / `prepare` / `function add` print and exit — split a stable library from the CLI.

**Typed configs.** Align Python `PolyServerFunction` / `PolyClientFunction` with REST/TS camelCase (`logsEnabled`, `disableAi`, `alwaysOn`, …). Accept snake_case on read if needed; **emit camelCase** from extract. Go `function init` already writes `logsEnabled`.

**Not adapter ops:** `discover`, `write_receipt`, sync/push, delete, execute, model, snippet add, setup. Function delete/execute/list/get/logs are already Go (`POLY-CLI-20`). poly-flow migration is `POLY-CLI-26`.

**Host work in `polyapi/polyglot` (lands with these tickets; POLY-CLI-5 stays completed)**

- `GET /specs` on the Go client; treat the body as opaque JSON. Allow a longer timeout than the default 30s.
- `polyapi generate`: `libraryGenerate` gate, fetch specs, pass `{ specs, no_types }` (CLI flags `--contexts` / `--names` / `--ids` / `--no-types` stay on the host). After `function add`, fetch `/specs?ids=<id>`.
- `Engine.Prepare`: inspect → description-generation → prepare-write. Stop passing `disable_ai` into adapter `prepare`.
- Stop injecting `POLY_API_KEY` / `POLY_API_BASE_URL` into the adapter env.
- Update `docs/delegate-protocol.md`, RFC §5.5 network rule, `AGENTS.md`, protocol fixtures, `GenerateParams` / `PrepareParams` / new `Inspect*` types.

**SDK CLI dual-path (not the adapter)**

- Keep `generate`, `prepare`, `function add`, Python `clear` working.
- When `polyapi` is on `PATH`, `sync` execs `polyapi deploy push` (`--dry-run` maps to `polyapi deploy plan`); else old in-process sync with a deprecation warning.
- `setup` can remain until users live on `polyapi init` / `auth login`.

---

### POLY-CLI-11 — Thin TypeScript SDK adapter

**Type:** Engineering  
**Priority:** P1  
**Depends on:** POLY-CLI-5, POLY-CLI-25  
**Repo:** `polyapi-typescript`

**Description**  
Ship `build/adapter.js` implementing the protocol 1 freeze above (`poly adapter` remains a hidden fallback for the host). Keep `npx poly` generate / prepare / function-add working during transition. Do not reimplement Glide. Do not list `adapter` in SDK help.

**Acceptance criteria**
- [ ] `node node_modules/polyapi/build/adapter.js` (and `poly adapter`) speak protocol 1 stdin/stdout JSON
- [ ] Required ops: `capabilities`, `generate`, `inspect`, `prepare`, `extract`, `parse_function`; `clear` emptying generated output
- [ ] Library path does not `process.exit`, walk the tree, or call Poly HTTP
- [ ] `generate` renders host-supplied `specs`; `extract` / `parse_function` return REST camelCase DTOs including `code`
- [ ] `npx poly generate|prepare|function add` still work; `npx poly sync` prefers global `polyapi deploy push` when present
- [ ] `PolyServerFunction` / `PolyClientFunction` match Go `function init` scaffolds
- [ ] Host `libraryGenerate` / `customDev` gates (not a second check inside the adapter subprocess)
- [ ] Protocol fixtures under `tests/fixtures/delegate/protocol/v1` stay valid against the real adapter

---

### POLY-CLI-12 — Thin Python SDK adapter

**Type:** Engineering  
**Priority:** P1  
**Depends on:** POLY-CLI-5, POLY-CLI-25  
**Repo:** `polyapi-python`

**Description**  
Same freeze as POLY-CLI-11 for hidden `python -m polyapi adapter`. `clear` is required (SDK already has it). Function execute/delete stay on the global CLI, not the adapter. poly-flow migration docs are POLY-CLI-26, not this ticket. Do not list `adapter` in SDK help.

**Acceptance criteria**
- [ ] `python -m polyapi adapter` speaks protocol 1 stdin/stdout JSON
- [ ] Required ops: `capabilities`, `generate`, `inspect`, `prepare`, `extract`, `parse_function`, `clear`
- [ ] Library path does not `sys.exit`, walk the tree, or call Poly HTTP
- [ ] `generate` renders host-supplied `specs`; `extract` / `parse_function` return REST camelCase DTOs including `code`
- [ ] `python -m polyapi generate|prepare|function add|clear` still work; `sync` prefers global `polyapi deploy push` when present
- [ ] TypedDict fields aligned with REST/TS camelCase (`logsEnabled` not only `logs_enabled`)
- [ ] Protocol fixtures under `tests/fixtures/delegate/protocol/v1` stay valid against the real adapter

---

### POLY-CLI-15 — Distribution

**Type:** Engineering  
**Priority:** P1  
**Depends on:** POLY-CLI-2

**Description**  
Ship installable binaries: GitHub Releases, checksums, install script. Homebrew is a follow-up ticket (not this one).

**Acceptance criteria**
- [ ] Build executable(s) suitable for Windows, Linux, and MacOS (supporting most common architectures)
- [ ] Build can be run via simple shell script and executables are included in git repo (each version produces new binary with version in the name so old and new version can coexist in the builds directory)
- [ ] Github CI/CD which will do the build process
- [ ] Dead simple install shell script that can be easily audited within the repo itself
- [ ] Fresh machine installs `poly` in &lt; 2 minutes without Golang toolchain
- [ ] Signed/checksummed artifacts
- [ ] Version command reports build metadata
- [ ] Running install shell script will also replace old installation if one found

---

### POLY-CLI-18 — Resource pack: Snippets + schemas

**Type:** Engineering  
**Priority:** P1  
**Depends on:** POLY-CLI-4, POLY-CLI-25

**Description**  
Bring snippets beyond TS-only `snippet add`; schemas as first-class CRUD + Glide types (also fed by OpenAPI train).

**Commands:**  
- `polyapi snippet list|get|add|update|delete`  
- `polyapi schema list|get|create|update|delete`

**Acceptance criteria**
- [ ] Snippet parity across languages via global CLI
- [ ] Schema CRUD; Glide `PolySchema` / `PolySnippet`
- [ ] OpenAPI train continues to create schemas as today where applicable

---

### POLY-CLI-19 — Resource pack: GraphQL subscriptions

**Type:** Engineering  
**Priority:** P2  
**Depends on:** POLY-CLI-4, POLY-CLI-25

**Description**  
CLI + Glide type for GraphQL subscriptions (CUSTOM/OHIP per docs).

**Acceptance criteria**
- [ ] CRUD commands documented against `graphql_subscriptions/*`
- [ ] Glide deployable registered in deploy order
- [ ] Pull/push/plan/receipts work

---

### POLY-CLI-20 — Function command tree completion

**Type:** Engineering  
**Priority:** P1  
**Depends on:** POLY-CLI-4, POLY-CLI-5, POLY-CLI-25

**Description**  
Unify function management across SDKs.

**Commands:** `polyapi function list|get|add|update|delete|execute|logs`  
(server/client/api distinctions via flags or subcommands)

**Acceptance criteria**
- [ ] Delete by id or context+name (Java parity); expose Python’s internal `spec_delete`
- [ ] Execute via HTTP where possible; language delegate when local generated module required
- [ ] `function add` remains hybrid (parse via delegate, upsert via Golang)
- [ ] Glide remains preferred for multi-function projects; `add` for one-offs

---

### POLY-CLI-22 — Logs + error-handler resources

**Type:** Engineering  
**Priority:** P2  
**Depends on:** POLY-CLI-4, POLY-CLI-25

**Description**  
`polyapi logs query|tail|get|delete`; `polyapi error-handler list|get|create|update|delete` with Glide type for handlers.

**Acceptance criteria**
- [ ] Logs query aligned with logging docs/API
- [ ] Error handlers manageable without only SDK runtime registration
- [ ] Glide support for error-handler deployables where platform allows

---

### POLY-CLI-23 — Canopy / custom apps (stretch)

**Type:** Engineering  
**Priority:** P3  
**Depends on:** POLY-CLI-4, POLY-CLI-25

**Description**  
Explore CLI for Canopy application lifecycle (`polyapi app` / `polyapi canopy`).

**Acceptance criteria**
- [ ] Spike doc: which APIs exist vs portal-only
- [ ] If APIs sufficient: CRUD + optional Glide type
- [ ] If not: explicit deferral recorded in RFC

---

### POLY-CLI-24 — Meta commands + `init` project scaffolder

**Type:** Engineering  
**Priority:** P1 (init is a major user-facing slice — do not treat as afterthought)  
**Depends on:** POLY-CLI-3, POLY-CLI-5; CI workflow templates improved once Action exists (POLY-CLI-15 / follow-up)

**Description**  
Ship `polyapi version`, `polyapi doctor`, `polyapi update`, and a **first-class `polyapi init`** that replaces the manual Glide template README checklist.

#### `polyapi init`

**Modes:** create a **new** project **or** adopt an **existing** repo/directory.

**Interactive dialog** (flags for non-interactive / CI):

| Prompt | Purpose |
| --- | --- |
| Language | TypeScript / Python (Java later) |
| New vs existing | Empty scaffold vs merge into cwd |
| Template | Official Glide template or custom git URL/ref (`polyapi/poly-glide-template-js`, `…-py`) |
| Git provider | GitHub / GitLab / other → CI flavor |
| Name / package metadata | Substitute into template |
| Environments | Branch→Poly env map (e.g. main→prod); instance URL(s); secret *names* for CI |
| Deploy scope | Optional path excludes / types defaults |
| Tooling | Install SDK deps; husky / pre-commit (`prepare`); ignore files |
| CI/CD | Write workflow (deploy on allowed branches; secret placeholders; path filters) |

**Instantiation steps**

1. Fetch template at pinned ref (or generate minimal tree if `--template none`)  
2. Write/merge `.poly/config.toml` (environments + deploy.targets + scope)  
2b. Ensure `.poly/` is in `.gitignore` (create `.gitignore` if git repo and missing)  
3. Add language SDK dependency + scripts  
4. Install hooks (prepare on commit; AI-off friendly)  
5. Emit CI workflow matching provider  
6. Optionally run package install + prompt for `polyapi setup` / `polyapi generate`  
7. Print checklist: create remote, add secrets (`POLY_API_KEY_PROD`, …), workflow permissions  

**Existing-project safety:** never overwrite without prompt; show diff/plan of files to add.

#### Other meta commands

- `polyapi version` — semver + build metadata  
- `polyapi doctor` — config, adapter, auth, validate summary, whether current branch may push / which env  
- `polyapi update` — self-update or install instructions  

**Acceptance criteria**
- [ ] `polyapi init` interactive path can create a new TS project from official template end-to-end (local)
- [ ] `polyapi init --lang python` same for Python template
- [ ] `polyapi init` in an existing Node/Python app adds Poly without destroying unrelated config (conflict prompts)
- [ ] Custom `--template https://…` works
- [ ] Dialog (or flags) captures language, provider, template, env/branch map
- [ ] Scaffolds `.poly/config.toml` with deploy.targets / environments / scope stubs
- [ ] Ensures `.poly/` is gitignored: append to existing `.gitignore`, or create one when inside a git repo
- [ ] Installs commit hook that runs `prepare` (mirrors husky / pre-commit templates)
- [ ] Writes CI workflow with secret placeholders and branch filters; documents required git-host permissions
- [ ] Non-interactive mode: all prompts available as flags + `--yes`
- [ ] Doctor fails clearly on missing key/adapter; reports branch→env deploy eligibility
- [ ] Version includes git/semver metadata from releases

**Post-v1 / future (do not block v1 `init`):**
- Systems / domain picker (multi-select) **or** free-text + AI mapping
- Pull matching **OOB catalogue** contexts into generate hints
- Seed **AI skills / agents** and optional example deployables for those systems
- Track as follow-on (e.g. POLY-CLI-28) once init + generate are stable

---

### POLY-CLI-25 — Glide core v2 (polymorphic deployables)

**Type:** Engineering (epic-level)  
**Priority:** P0  
**Depends on:** POLY-CLI-4, POLY-CLI-5  
**Blocks:** resource packs’ Glide portions, adapters’ sync migration, POLY-CLI-26, POLY-CLI-27

**Description**  
Implement the Glide v2 pipeline in Golang, combining Glide’s discovery/receipts/prepare with flow’s multi-resource sync architecture.

#### Commands
- `polyapi deploy prepare` — find deployables (Go walk, always re-scan; no `--lazy`); auto-docs/AI fill-in; reviewable file edits; AI disabled when `DISABLE_AI` / `--disable-ai`
- `polyapi deploy validate` — **preflight**: `{{ENV_TOKEN}}`s, required metadata, duplicate/unknown types, **cross-resource wiring** (e.g. webhook/trigger → server function, job → functions, schema refs); `--strict`, `--only env|metadata|refs`, optional `--online`
- `polyapi deploy plan` — dry-run of push; CI-friendly (flow `plan`); should be clean after validate
- `polyapi deploy push` — apply local → remote (evolves/aliases today’s `sync`); `--receipts` is optional
- `polyapi deploy pull` — remote → local for supported types (wire from day one; function codegen pull may phase)
- Soft-deprecated aliases **under `deploy` only**: `deploy sync` → `push`; `deploy env-check` → `validate --only env` (compat with flow)

#### Discovery
- [ ] **Primary:** recursive scan for typed `polyConfig` on **all** resource kinds (functions, vari, webhooks, jobs, …); language delegate extracts canonical payload
- [ ] SDK exports typed configs (`PolyVariable`, `PolyWebhook`, `PolyJob`, …) for editor autocomplete
- [ ] In-file deploy receipts on those modules (same UX as functions)
- [ ] Light scripting allowed under determinism rules (shared consts, env/`{{TOKEN}}`; no random/clock in hashed payload)
- [ ] **Secondary (optional):** JSON/JSONC globs for flow migration compat
- [ ] Not requiring specific folders like `src/**/server/*.py` or `artifacts/vari/**` to be discovered

#### Prepare
- [ ] Auto-documentation generation with review semantics (commit hooks / exit non-zero if files changed — match current Glide UX)
- [ ] Must not apply AI mutations during `push` in CI (prepare separately)

#### Sync intelligence (improve on both tools)
- [ ] Per-deployable **content hash** of canonical payload
- [ ] **Deploy receipts** in source (or sidecar) + project cache of deployable records
- [ ] **Per-instance** deployment records (id, deployedAt, hash, canopy URL)
- [ ] Three-way compare: local hash vs receipt hash vs remote hash → `create` | `update` | `skip` | `conflict` | `pull-back` | `remove`
- [ ] Actions for file-gone-but-was-deployed (**remove** remote) and remote-without-local (**orphan** report / `--delete-orphans`)
- [ ] Field-level diff output (flow-style) in verbose/plan mode
- [ ] Timestamp guard as secondary signal, not primary over content hash
- [ ] Resume checkpoints for interrupted push
- [ ] `--dry-run` / `plan`, `--force`, context / path / type filters

#### Resource registry
- [ ] `ArtifactSyncer`-style trait + registry (port semantics from poly-flow)
- [ ] Register types as packs land: variables, tables, schemas, functions, webhooks, triggers, jobs, snippets, subscriptions, error-handlers, …
- [ ] Deploy order: start from flow baseline; ticket follow-up for **dependency DAG** from parsed Poly refs (addresses Glide TODO)

#### Safety & deploy scope (flow CLI / CI parity)
- [ ] **Committed `[deploy]` + `[environments.*]` project config** (no secrets):
  - `deploy.targets[]`: `branches` → `environment` (+ `production = true` gate)
  - `unallowed_branch = off|warn|error` (default error)
  - `deploy.scope`: `paths`, `exclude_paths`, `types`, `contexts`, `context_path_map`, `exclude_orphan_contexts`
- [ ] Mutating commands blocked when branch matches no target (unless explicit override)
- [ ] Production targets: typed confirmation and/or CI exec key (from flow)
- [ ] CLI flags match flow: `--env`, `--path`, `--exclude-path`, `--types`, context filters
- [ ] Future GitHub Action inputs = same knobs (not just api key + base URL)
- [ ] `polyapi init` scaffolds safe defaults; `validate` / `doctor` report selected env + whether push allowed
- [ ] Secret redaction in all output
- [ ] Path confinement on pull (no `../` escape)

#### Acceptance criteria (summary)
- [ ] Sample project with functions **and** variables/webhooks/jobs can `prepare` → `validate` → `plan` → `push` (receipts optional, `--receipts` to write them)
- [ ] `validate` fails on missing env tokens **and** on broken cross-refs (e.g. webhook pointing at non-existent server function) with clear paths/`context.name`
- [ ] `validate` warns on missing useful metadata (description, etc.); `--strict` elevates warnings
- [ ] `push` on a feature branch with default config fails with a clear pointer to `deploy.targets`
- [ ] `exclude_paths` / `--exclude-path` omit whole repo sections from discover/plan/push
- [ ] Branch `main` selects configured prod environment; `develop` can map to a non-prod env without sharing the same gate
- [ ] Re-running push with no changes yields skip (quiet/cheap)
- [ ] Deleting a `polyConfig` file and pushing removes remote (or plans removal) using receipt/cache — developer need not manually track live IDs
- [ ] `pull --dry-run` shows file creates/updates for JSON artifact types
- [ ] Unit tests with fake client; no live platform required for core pipeline tests
- [ ] Design doc in repo citing Glide vs flow absorb/leave table

---

### POLY-CLI-26 — poly-flow reconciliation & deprecation

**Type:** Engineering + Docs  
**Priority:** P0  
**Depends on:** POLY-CLI-25, resource packs (7–10, 18), POLY-CLI-27; adapters 11/12 before deprecating function `polyConfig` workflows

**Description**  
Treat `/Users/aarongoin/Dev/poly-flow` as the reference implementation for multi-resource sync semantics. Inventory is done (2026-09-22 audit). Remaining engineering lives in **POLY-CLI-27**. This ticket is the written gap matrix, migration guide, and deprecation of the parallel `poly-flow` entrypoint.

**Inventory checklist (from flow README/MVP)**
- [ ] push / pull / plan / validate (env-check⊂) / config
- [ ] types: variables, tables, schemas, functions, webhooks, triggers, jobs
- [ ] env token substitution + JSONC
- [ ] content diff, force, resume, delete-orphans, adopt-orphans→pull
- [ ] prod gate, OTP hooks, redaction
- [ ] context/path/type filters
- [ ] post-function `generate` trigger
- [ ] Note flow gaps we intentionally do **not** copy: path-first discovery as primary; parallel brand

**Acceptance criteria**
- [ ] Written gap matrix: flow feature → `polyapi` command/module → status (cite POLY-CLI-27 for leftovers)
- [ ] Migration guide: `poly-flow push` → `polyapi deploy plan|push`; `artefacts/` coexistence with `polyConfig`; env vars → `.poly/config.toml`
- [ ] Deprecation notice for the `poly-flow` entrypoint once 27 + adapters cover the absorb list

---

### POLY-CLI-27 — Glide ← poly-flow parity leftovers

**Type:** Engineering  
**Priority:** P2  
**Depends on:** POLY-CLI-25  
**Blocks:** POLY-CLI-26 (deprecation/migration guide)

**Description**  
2026-09-22 audit of `src/glide` vs `/Users/aarongoin/Dev/poly-flow`. Glide already owns the push/plan/validate loop for flow’s eight types plus snippets. This ticket is **not** a second pipeline and does **not** reopen POLY-CLI-25. It ports the remaining absorb semantics, tightens a few safety nits, and finishes pull/plan UX that flow documented but did not fully ship.

JSONC multi-resource sync is in. Code `polyConfig` extract/prepare is delegated to the TypeScript and Python adapters (POLY-CLI-11/12).

#### Already absorbed (do not redo)

- `deploy plan` / `push` / `validate` (`env-check` alias) / `pull` / `prepare`
- Types: variables, tables, schemas, ai/api/client/server functions, webhooks, triggers, jobs, snippets
- `{{TOKEN}}` substitution (`.env` then process env), JSONC comment strip, `--allow-unresolved`
- Content-hash skip; `--force`; `--resume` (crash mid-run); `--path` / `--exclude-path` / `--types` / `--contexts`
- Ref rewrite on push (webhook security functions, trigger source/dest, job `functionContext`+`functionName`)
- Orphan report + `--delete-orphans` + `exclude_orphan_contexts`
- SECRET blanked to `{}`; OBSCURED kept; existing obscured needs `--force`
- Branch allowlist via `[[deploy.targets]]`; typed `PROD` / `POLYPROD_EXEC_KEY` / `--yes`
- Pull path confinement; function pull warn-and-skip
- Optional receipts / three-way conflict (Glide-only)

#### Leave (do not copy)

- Path/glob as **primary** discovery (`src/**/artefacts/…`, `src/**/server/*.py`)
- mtime as **primary** update guard on push (RFC: content hash primary)
- Parallel `poly-flow` brand; env-only config as source of truth
- `--adopt-orphans` / `adopted-orphans/` (pull replaces it)
- Shell `deployment.sh` function discovery
- “Branch is derived from `main`” prod heuristic (keep explicit `deploy.targets`)
- Dependency DAG (POLY-CLI-25 follow-up); typed `Poly*` SDK configs (11/12); function pull codegen (v1 skip)

Flow itself is incomplete in ways we should not treat as the spec: `pull` is an unregistered stub; `--resume` is discarded; `plan` does not substitute tokens; `--path` / CONTEXT do not filter functions; `--types client-functions,server-functions` is silently dropped; verbose diffs never attach payloads.

#### Work to do

**Functions**
- Server/client (and ai/api if the platform matches) **update = POST redeploy**, not PATCH. Flow `FunctionSyncer.update` calls `client.create`. Glide `applyOne` always `Client.Update` (PATCH). JSONC function artifacts should redeploy correctly even before adapters.
- After a successful function **create or update** on `deploy push`, run `polyapi generate` (same as flow `maybe_run_generate`). `--skip-generate` to skip. Missing adapter is a warning, not a hard fail (same pattern as `model train`). Dry-run / plan never generate.

**Pull**
- GET-by-id hydrate before write (list DTOs omit `definition` / `code` / `columns` / `securityFunctions`).
- Reverse UUID → `context.name` on jobs and webhooks (and trigger source/dest if present).
- If a local JSON file is **newer** than remote `lastUpdatedAt`/`updatedAt`, skip unless `--force` (mtime secondary, pull only).
- Keep American `src/<context>/artifacts/<type>/` as the default write layout; `context_path_map` still wins. Do not require British `artefacts/` on discover (walk already finds both).

**Safety**
- `--delete-orphans` on **push** requires a context filter (`--contexts` or `deploy.scope.contexts`). Refuse a whole-instance wipe (flow exit 2).
- Orphan **delete** order is reverse `DeployOrder()` (jobs → … → variables; snippets last-in first-out).
- Existing **SECRET** variables: skip update unless `--force` or an explicit opt-in (flow `update_existing_secret_variables`). OBSCURED stays `--force` as today.
- OTP on orphan delete when enabled (prompt once, send `x-otp`). Vari CRUD already has `--otp`; deploy deletes do not.
- `POLYPROD_EXEC_KEY`: reject empty/whitespace; keep non-empty env as the CI bypass; keep `--yes` and typed `PROD`. Document that flow’s HMAC compared the env var to itself.
- 1Password `op://` resolution

**Plan / resume / validate UX**
- Print field-level changed keys on `deploy plan` and on `-v` push (flow `diff: code, description`). Do not dump secret/token values.
- Keep `.poly/resume.json` when a push **finishes with failures** so `--resume` retries the rest; still delete it on a fully successful run. Today it is removed at the end of every completed push.
- `deploy validate` prints whether the current branch may push / which env would be used (`Report.Eval` is filled and unused).

#### Acceptance criteria
- [ ] Updating an existing server/client function deployable POSTs a redeploy payload; tables/variables/webhooks/jobs/schemas/snippets still PATCH
- [ ] `deploy push` that creates/updates a function runs generate unless `--skip-generate`; plan/dry-run does not; missing adapter warns
- [ ] `deploy pull` hydrates by id and writes comparable JSON (schema definition, snippet code, table columns, webhook securityFunctions present when the API returns them)
- [ ] Pull rewrites function/webhook UUIDs back to `context.name` where the local/canonical form uses refs
- [ ] Pull does not overwrite a newer local JSON file without `--force`
- [ ] `deploy push --delete-orphans` with no context filter exits usage (2) with a pointer to `--contexts`
- [ ] Orphan deletes run in reverse deploy order; OTP header is sent when OTP is enabled
- [ ] Existing SECRET variable updates are skipped without `--force` / opt-in; SECRET values are still never sent from source
- [ ] Plan output lists changed field names; `-v` does not print SECRET/OBSCURED values or unresolved-token substitutions
- [ ] A push that fails some items leaves a resume checkpoint; `--resume` skips the ones that succeeded
- [ ] Validate output includes the deploy-policy line (branch → env / blocked)
- [ ] Unit tests with fake client; no live platform. Do not tick this list when landing slices — update Epic overview Status (`in-progress` until all bullets, then `completed`)

---

## Suggested sequencing

```text
Phase 0  RFC (1)
Phase 1  Foundation: 2 → 3 → 4 → 5 → 24 (doctor/version)
Phase 2  Glide core v2 (25)  ‖  model suite (6)   # parallel once 4 exists
Phase 3  Function tree (20) + resource packs: 7, 9, 10, 18, 8, …  # register into 25 as they land
Phase 4  Adapters: 11 (TS), 12 (Python)   # NOT Java yet
Phase 5  **`init` scaffolder (24)** + env/promote (14), distro (15), tests (17), docs (16)
Phase 6  Flow reconciliation (26)   # docs/deprecation; engineering leftovers are 27
Phase 7  Stretch: 19, 21, 22, 23, 27  # 27 can land in slices before 26
Phase 8  Java / Maven adapter (13) — last
```

---

## Open questions (resolve in RFC / POLY-CLI-1)

1. Artifacts: **code + typed `polyConfig`** for all resources (recommended); JSONC as flow compat; receipts in source comments.
2. Is `polyapi sync` a permanent alias of `push`, or a soft-deprecated name?
3. Function `pull` codegen: v1 warn-and-skip (flow) vs generate stubs via delegate?
4. Single project multi-language: support or require one lang per `.poly` project?
5. Tracker of record for these tickets (Asana / GitHub / Linear)?
   - Repo for implementation: **`polyapi/polyglot`** (decided)
   - CLI command: **`polyapi`** (decided)
6. Default `deploy.targets` for `polyapi init` (`main`→prod only vs also `develop`→dev)?
7. Point at flow GitHub Action source (if any) to 1:1-map Action inputs to config.
