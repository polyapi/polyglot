# Snippets

`polyapi snippet` is the cross-language replacement for TypeScript `npx poly snippet add`. Snippets are copy-paste source (TypeScript, Python, JSON, markdown, …), not executed remotely.

Lookup is by ID, `context.name`, or name plus `--context` (case-insensitive, then exact).

Platform docs: [Snippets](https://docs.polyapi.io/snippets/), [Managing Snippets](https://docs.polyapi.io/snippets/managing-snippets.html).

## Commands

```bash
polyapi snippet init --name header
polyapi snippet init --name header --snippet templates.header
polyapi snippet list --context billing
polyapi snippet get header --context billing
polyapi snippet get billing.header --code
polyapi snippet add header snippets/header.ts --context billing
polyapi snippet update header --context billing --code-file snippets/header.ts
polyapi snippet delete header --context billing
```

`snippet list` prints NAME, LANGUAGE, VISIBILITY, and ID. The collection endpoint does not include `code`; use `snippet get` (or `get --code`) for the source.

`snippet add` is a PUT upsert (`PUT /snippets`), matching the TypeScript CLI — re-running add with the same name and context replaces the snippet. `snippet create` is an alias of `add`.

## Add / update fields

| Flag | Notes |
| --- | --- |
| `<name> <path>` | Required on add. The file is read as UTF-8 source. |
| `--context` | Required on add. On update, `--new-context` moves. |
| `--language` | Lowercase. Inferred from the file extension when omitted. |
| `--description` | Optional. |
| `--visibility` | `PUBLIC`, `TENANT`, or `ENVIRONMENT` (add default `ENVIRONMENT`). |
| `--code` / `--code-file` | Update only. `--code-file` also infers `--language` when `--language` is omitted. |

Language inference:

| Extensions | Language |
| --- | --- |
| `.ts` `.tsx` `.mts` `.cts` | `typescript` |
| `.js` `.mjs` `.cjs` `.jsx` | `javascript` |
| `.py` `.pyi` | `python` |
| `.java` | `java` |
| `.go` `.json` `.yaml`/`.yml` `.md` `.html` `.css` `.sql` `.xml` `.txt` `.sh` `.rb` `.rs` `.kt` | matching name (`text` for `.txt`, `bash` for `.sh`) |

Unknown extensions require `--language`. The TypeScript CLI mapped `.ts` to `javascript`; this CLI uses `typescript` (the Canopy/API value).

Add sends:

```json
{
  "name": "header",
  "context": "billing",
  "code": "export const n = 1;\n",
  "language": "typescript",
  "visibility": "ENVIRONMENT"
}
```

## Glide

`polyapi snippet init` writes a JSONC scaffold (default `src/artifacts/snippets/<name>.jsonc`, or `src/<context>/artifacts/snippets/<name>.jsonc` with `--context`) with a TypeScript `code` placeholder. Without `--snippet` it does not call the API — `snippet add` still upserts from a source file. `--snippet` copies a JSON/JSONC snippet as the file (name and context overwritten). Only `--name` is required.

JSON/JSONC under `**/snippets/**` (or code + `polyConfig`). Deploy payload is `name`, `context`, `description`, `code`, `language`, `visibility`. Plan/push GET the snippet by id because list DTOs omit `code`.

```json
{
  "name": "header",
  "context": "billing",
  "code": "export const n = 1;",
  "language": "typescript",
  "description": "Shared header"
}
```
