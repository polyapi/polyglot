# OpenAPI model suite

`polyapi model generate|validate|train` ports the TypeScript `npx poly model …` flow so every language can go OpenAPI → Specification Input → trained API functions without the TS CLI.

OpenAPI is **not** parsed in this binary. The platform translates the document (`POST /specification-input/oas`); this CLI sends the file (or a remote URL), writes JSON, validates DTOs, and upserts.

See [docs.polyapi.io — Using OpenAPI Specs](https://docs.polyapi.io/api_functions/openapi.html) for the product walkthrough. Commands below replace `npx poly model …`.

## Commands

```bash
polyapi model generate ./json-placeholder-spec.yaml --context jsonPlaceholder --host-url-as-argument
polyapi model generate ./json-placeholder-spec.yaml --context jsonPlaceholder --host-url https://jsonplaceholder.typicode.com
polyapi model generate ./spec.yaml ./spec.json --context myApi --disable-ai --rename foo:bar
polyapi model validate ./fake-json-placeholder-spec.json
polyapi model train ./fake-json-placeholder-spec.json
```

`generate <path> [destination]`: `path` is a local OpenAPI 3.x file or an `http(s)` URL. With no destination, the file is named from OpenAPI `info.title` (slugified) next to the source (or cwd for a URL). If that name exists, a `-N` suffix is added. An explicit destination must not already exist. Relative destinations are resolved against the source file’s directory.

`validate` and `train` read a Specification Input JSON file (`functions` / `webhooks` / `schemas`).

## Host URL modes

Same three modes as the TS CLI. If both host flags are set, `--host-url` is sent as well; the platform treats the hardcoded host as winning.

| Mode | Flag | Trained functions |
| --- | --- | --- |
| Host as argument | `--host-url-as-argument` or `--host-url-as-argument baseUrl` | Required string argument (default name `hostUrl`). URL templates use `{{hostUrl}}`. |
| Hardcoded host | `--host-url https://api.example.com` | Host baked into every URL. Value must be a valid HTTP(S) URL. |
| OAS `servers` | omit both | Spec must define exactly one usable server. |

Empty `--host-url-as-argument` defaults the argument name to `hostUrl`.

## Train

Train is an upsert: existing API functions, webhook handles, and schemas with the same `name`+`context` are overwritten. Imperative schema CRUD is `polyapi schema` ([docs/schema.md](schema.md)).

Order matches the TS CLI: API functions, then webhooks, then schemas. Schemas are topologically sorted so `x-poly-ref` dependencies (without `publicNamespace`) train first.

Permissions (from `GET /auth`): `manageApiFunctions`, `manageWebhooks`, `manageSchemas` as needed for non-empty arrays.

If any resource upserts, the CLI then runs `polyapi generate` via the language adapter when one is present. A missing adapter is a warning, not a failed train.

## Flags vs TypeScript

| TypeScript | polyapi |
| --- | --- |
| `--hostUrl` | `--host-url` |
| `--hostUrlAsArgument` | `--host-url-as-argument` |
| `--disable-ai` | `--disable-ai` |
| `--rename foo:bar` | `--rename foo:bar` |
| `--context` | `--context` |

## Parity notes vs `npx poly model`

- Same HTTP endpoints: `POST /specification-input/oas` (`text/plain` body), `POST /functions/api/description-generation`, `POST /webhooks/description-generation`, `POST /specification-input/validation/api-function`, `POST /specification-input/validation/webhook-handle`, `PUT /functions/api`, `PUT /webhooks`, `PUT /schemas`.
- `--disable-ai` skips description-generation and keeps translator names/descriptions; `--context` still overwrites context.
- `--rename` is a whole-word (`\b`) replacement on the written JSON (so `{{foo}}` becomes `{{bar}}`, `foobaz` does not).
- After train, polyglot calls the language adapter (`polyapi generate`) instead of shelling out to `npx poly generate`. Missing adapter → warning.
- Invalid Spec Input, missing files, bad host URLs, and translator errors **exit non-zero**. The TS CLI often printed the error and exited 0.
- Train: all upserts failed → exit 1; mix of success and failure → exit 12 (`partial`).
- Schemas are upserted **sequentially** in dependency order. The TS CLI fires schema upserts concurrently in chunks of 5 (the unit test only asserts call order). Sequential PUTs are required so `x-poly-ref` targets exist.
- Written JSON is `encoding/json` pretty-print (2-space). The TS CLI hand-builds the string to dodge a `JSON.stringify` size limit; field order may differ, contents should not.
- Filename slug is lowercase letters/digits with dashes. The TS CLI has a larger transliteration map; ASCII titles such as the docs example match (`Fake json placeholder spec` → `fake-json-placeholder-spec.json`).
- Numbered collision suffix uses `title-N.json` with the full number (`-12`), not the TS regex `([0-9])+` which kept only the last digit.
