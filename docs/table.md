# Tabi tables

`polyapi table` is the imperative tree for Tabi (PolyAPI's managed PostgreSQL tables) and row operations.

Glide already treats tables as first-class deployables (`type: table`). This pack adds the command tree and documents that **Glide deploys table schema only** — not seed rows.

Platform docs: [Tabi Tables](https://docs.polyapi.io/tabi_tables/).

## Commands

```bash
polyapi table init --name orders
polyapi table list --context billing
polyapi table get orders --context billing
polyapi table create --name orders --context billing --columns-file ./orders.columns.json
polyapi table update orders --context billing --description "Order lines"
polyapi table delete orders --context billing

polyapi table rows list orders --context billing --where '{"status":"open"}' --limit 50 --order-by '{"id":"asc"}'
polyapi table rows get orders abc123 --context billing
polyapi table rows insert orders --context billing --data '[{"sku":"A-1"}]'
polyapi table rows upsert orders --context billing --data '{"sku":"A-1","status":"open"}'
polyapi table rows update orders --context billing --where '{"sku":"A-1"}' --data '{"status":"closed"}'
polyapi table rows delete orders --context billing --where '{"sku":"A-1"}'
polyapi table rows query orders --context billing --where '{"status":"open"}'
polyapi table rows count orders --context billing
```

Get/update/delete take a table ID, or a name with `--context` (case-insensitive, then exact, matching `vari` / `function`).

`polyapi table init` writes a JSONC scaffold (default `src/artifacts/tabi/<name>.jsonc`, or `src/<context>/artifacts/tabi/<name>.jsonc` with `--context`) with a sample column. Without `--snippet` it does not call the API — edit, then `polyapi deploy plan`. `--snippet` copies a JSON/JSONC snippet as the file (name and context overwritten). Only `--name` is required.

`table list` prints NAME, VISIBILITY, and ID. The collection endpoint does not include columns; use `table get` for the schema.

## Create / update fields

| Flag | Notes |
| --- | --- |
| `--name` / `--context` | Required on create. On update, `--name` renames; `--new-context` moves. |
| `--columns` / `--columns-file` | JSON array of column objects. Create requires one of these. A `{ "columns": [...] }` wrapper is accepted. |
| `--visibility` | `ENVIRONMENT` or `TENANT` (create default `ENVIRONMENT`). Tables are not `PUBLIC`. |
| `--description` | Optional. |
| `--otp` | Sent as `x-otp` on update/delete when the instance requires MFA. |

Tabi adds `id` (UUID PK), `created_at`, and `updated_at` on every table. Do not include those in `--columns`.

Column objects: `name`, `type`, optional `required`, `unique`, `primary`, `default`, `schema`. `schema` is for generated SDK type hints, not runtime enforcement.

Schema PATCH can **add or drop columns** only — not rename columns or change types. UNIQUE can be added after create, but not removed. Composite UNIQUE / indexes and foreign keys are not supported.

## Column types

Tabi accepts these names and aliases (stored PG type in parentheses):

- `string`, `text`, `varchar` (text)
- `char` (char)
- `uuid` (uuid)
- `number`, `numeric`, `decimal` (numeric)
- `float`, `double`, `float8` (double precision)
- `int`, `integer`, `int4` (integer)
- `bigint`, `int8` (bigint)
- `serial` / `bigserial`
- `boolean`, `bool`
- `date`
- `time`, `timesec` (time)
- `timestamp`, `timestamptz` (timestamptz)
- `json`, `jsonb`, `object` (jsonb)

## Rows

Row commands POST to `/tables/{id}/insert|upsert|select|update|delete|count`. There is no per-row REST GET; `rows get` is a select with `where.id`.

| Command | Body |
| --- | --- |
| `rows list` | select; prints the `results` array (`no rows found` when empty) |
| `rows query` | select; prints the full `{ results, pagination }` body |
| `rows get` | select `where: { id }` |
| `rows insert` / `upsert` | `{ data: [ ... ] }` (`--data` may be one object) |
| `rows update` | `{ where, data }` — `data` is a single object of columns to set |
| `rows delete` | `{ where }` — `--all` omits `where` (every row) |
| `rows count` | `{ where? }` → `{ count }` |

`--where` and `--order-by` are JSON objects. `--order-by` values are `asc` or `desc`. `--offset` requires `--order-by` so pages stay stable.

Upsert currently matches on **one unique column**.

## Glide

Tables are discovered as:

- **Code + typed `polyConfig`** (recommended): `polyConfig: PolyTable = { … }`. Go finds the binding; the language adapter `extract`s the payload (POLY-CLI-11/12).
- **JSON/JSONC** (flow compat): files under `**/tabi/**` or `**/tables/**`.

Deploy payload is schema only: `name`, `context`, `description`, `visibility`, `columns`. Seed `rows` / `data` / `seed` in a JSON artifact are **not** pushed. Insert rows with `polyapi table rows insert` (or the generated SDK). Auto columns (`id`, `created_at` / `createdAt`, `updated_at` / `updatedAt`) are stripped from the local column list before compare/push.

See [docs/glide.md](glide.md).
