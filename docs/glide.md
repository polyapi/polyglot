# Glide v2

Glide lives in the Go `polyapi` host (`src/glide`). Language SDKs do not ship a second deploy pipeline.

## Split

| Go host | Language adapter |
| --- | --- |
| Walk the repo, JSONC parse, `{{TOKEN}}` substitution, hashes, validate, plan, push, pull, branch/prod gates | Transpile / eval `polyConfig` (`extract`), JSDoc/docstring rewrites (`prepare`), `generate`, `parse_function` |
| Tiny language table: file extensions, `//` vs `#`, a `polyConfig` text marker | TypeScript compiler, Python parse, codegen |

`polyapi deploy` **always re-scans**. There is no `--lazy` mode and no cached deployable list.

Code files are candidates only when they **bind** `polyConfig` (`export const polyConfig =`, `polyConfig: PolyVariable =`, …). A mention of the name in a string or mock payload is ignored. The walk skips `node_modules`, `.git`, venvs, build output, and test/fixture directories (`tests`, `test`, `fixtures`, `__tests__`, `testdata`). Finding **zero** deployables is an error (`prepare` / `validate` / `plan` / `push`; `pull` also errors when there is nothing local and nothing remote). `polyapi doctor` reports that as a skip, not a failure.

## Commands

```text
polyapi deploy prepare
polyapi deploy validate [--only env|metadata|discovery|cross-resource|config] [--strict] [--online]
polyapi deploy env-check          # alias of validate --only env
polyapi deploy plan
polyapi deploy push [--receipts] [--delete-orphans] [--force] [--resume]
polyapi deploy pull [--dry-run] [--delete-orphans]
```

`deploy sync` is a soft-deprecated alias of `deploy push`.

## Receipts (optional)

Off by default. Enable with `deploy.receipts = true` or `--receipts`.

- **Off:** two-way compare (local hash vs remote). Deleted locals are orphans (`--delete-orphans` to remove).
- **On:** comment header (`//` / `#`) plus `.poly/receipts.json`. Three-way compare; a file we previously pushed and then deleted is removed on push without `--delete-orphans`.

## Absorb vs leave (poly-flow)

| Take from flow | Leave |
| --- | --- |
| Syncer order, plan, validate (env-check ⊂), prod gate, resume, orphans, redaction, `Client` seam, JSON payload shapes, env-token substitution, `--path` / `--exclude-path` / `--types` / contexts | Path/glob as **primary** discovery; mtime-primary sync; parallel `poly-flow` brand |

| Take from Glide (TS/Python today) | Leave |
| --- | --- |
| `polyConfig` as the authoring format, prepare (docs/AI), optional in-file receipts | Adapter-owned tree walk; lazy/git prepare cache |

JSONC artifacts remain a migration path. New projects author code + typed `polyConfig`; Go finds those files, the adapter extracts them.

## Local scaffold (`init`)

`polyapi <resource> init` writes a local file with placeholders. Without `--snippet` it does **not** call the API — edit the file, then `polyapi deploy plan` / `push`. Immediate remote create is still `create` / `add`.

```bash
polyapi vari init --name apiKey
polyapi table init --name orders
polyapi schema init --name Order
polyapi webhook init --name hook
polyapi trigger init --name weekly --type webhook
polyapi trigger init --name on-error --type error-handler
polyapi job init --name nightly
polyapi snippet init --name header
polyapi subscription init --name ordersStream --type CUSTOM
polyapi subscription init --name operaEvents --type OHIP --context opera
polyapi app init --name dashboard
polyapi function init --name helloWorld --type server
polyapi function init --name hello_world --type server --lang python
polyapi function init --name helloWorld --type server --snippet templates.helloWorld
polyapi vari init --name apiKey --snippet templates.variable --context billing
```

Most resources write JSONC under `src/artifacts/<type>/<name>.jsonc` (or `src/<context>/artifacts/…` with `--context`).

**Server and client functions** write TypeScript (`.ts`) or Python (`.py`) source instead: a typed `polyConfig` plus a function whose identifier is exactly `--name` (the same string as `polyConfig.name`). That match is required for deploy and execute. `--name` must be a valid identifier for the language (letters, digits, underscores; TypeScript also allows `$`; not a reserved word; no hyphens). Language is `--lang`, else the `--path` extension, else the project (default TypeScript). Default path is `src/<context>/server|client/<name>.ts`. `--type api` and `--type ai` still write JSONC.

Jobs, triggers, and applications have no `--context`. `--path` overrides (file or directory). `--force` overwrites. The command prints the path it wrote.

`--name` is required. `--context` is optional. `function init` also requires `--type server|client|api|ai`. `trigger init` requires `--type webhook|error-handler`. `subscription init` requires `--type CUSTOM|OHIP`.

`--snippet` fetches a PolyAPI snippet (id, `context.name`, or a name that is unique on the instance) and uses its **code** as the new file. `--name` and `--context` from this command overwrite those fields in the snippet (and the function identifier for server/client). The init `--context` is the new resource, not the snippet lookup. After a successful fetch, the snippet language must match the file being generated: `json` / `jsonc` for JSONC artifacts; `typescript` / `javascript` or `python` for server/client functions (`--lang`, path extension, or project). `trigger init --snippet` also requires `--type` to match the snippet source (webhook vs error-handler). `subscription init --snippet` must match `--type` when the snippet JSON has a type field. A mismatch or a fetch error exits without writing a file. `--snippet` requires credentials.

## Variables

`polyapi vari` is the imperative tree ([docs/vari.md](vari.md)). On deploy, SECRET values in source are replaced with `{}` and never pushed. OBSCURED values are kept. Existing obscured remotes need `--force` to update.

## Tables

`polyapi table` is the imperative tree ([docs/table.md](table.md)). Glide deploys **schema only** (`name`, `context`, `description`, `visibility`, `columns`). Seed rows in a JSON artifact are not pushed — use `table rows insert` (or the generated SDK). Auto columns `id` / `created_at` / `updated_at` are stripped from the local column list. Plan/push GET the table by id so columns (missing from `GET /tables` list DTOs) are compared.

## Webhooks and triggers

`polyapi webhook` / `polyapi trigger` are the imperative trees ([docs/webhook.md](webhook.md), [docs/trigger.md](trigger.md)). Security functions and trigger source/destination may be `context.name`; the host rewrites them to UUIDs (webhooks before triggers in deploy order). Trigger updates only send `name` / `waitForResponse` / `enabled`; a source or destination drift fails the item so you delete and recreate. Plan/push GET webhooks by id because list DTOs omit `securityFunctions`.

## Jobs

`polyapi job` is the imperative tree ([docs/job.md](job.md)). Jobs have no context (matched on name). Function list entries may be `functionContext`+`functionName` or `id`; the host rewrites them to server-function UUIDs (jobs deploy last). A crontab string becomes `{type: "periodical", value: …}`. Deploy payload is `name`, `schedule`, `functions`, `executionType`, `enabled` only.

## Schemas

`polyapi schema` is the imperative tree ([docs/schema.md](schema.md)). Deploy payload is `name`, `context`, `definition`, `visibility`. Plan/push GET the schema by id because list DTOs omit `definition`. Compare keeps local definition keys only, so platform-injected `$schema` / `additionalProperties` do not force an update. `x-poly-ref` paths are checked by `deploy validate --only refs` (public-namespace refs are skipped). OpenAPI train still upserts schemas via `PUT /schemas`.

## Snippets

`polyapi snippet` is the imperative tree ([docs/snippet.md](snippet.md)). Deploy payload is `name`, `context`, `description`, `code`, `language`, `visibility`. Plan/push GET the snippet by id because list DTOs omit `code`. Snippets deploy after jobs.

## GraphQL subscriptions

`polyapi subscription` is the imperative tree ([docs/subscription.md](subscription.md)). `CUSTOM` or `OHIP` JSONC under `**/subscriptions/**`. `functionId` / `paramsSfxId` / `paramsVariableId` may be `context.name` and are rewritten to UUIDs. Plan/push GET by id because list DTOs omit query, URL, and function. PATCH sends only changed authorable fields so an unchanged query does not restart the live websocket. `ohipOffset` is runtime state and is not deployed. Empty or `{{ENV_TOKEN}}` `paramsObject` does not overwrite vaulted remote params. Pull redacts `clientSecret`. Subscriptions deploy after snippets and before applications.

## Applications

`polyapi app` is the imperative tree ([docs/app.md](app.md)). Applications have no context (matched on name). Deploy payload is `name`, `description`, `visibility`, `config`. Plan/push GET the application by id because list DTOs omit `config`. Applications deploy last so referenced functions already exist. Collection function path/id rewriting inside `config` is not done in v1.
