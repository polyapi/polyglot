# Schemas

`polyapi schema` is the imperative tree for JSON Schema resources. Create, update, and delete a schema without Canopy. OpenAPI train (`polyapi model train`) also upserts schemas from a Specification Input file.

Lookup is by ID, `context.name`, or name plus `--context` (case-insensitive, then exact).

Platform docs: [Creating Schemas](https://docs.polyapi.io/schemas/create_schemas.html), [Using Schemas](https://docs.polyapi.io/schemas/use_schemas.html).

## Commands

```bash
polyapi schema init --name Order
polyapi schema list --context billing
polyapi schema get Order --context billing
polyapi schema get billing.Order
polyapi schema create --name Order --context billing --definition-file ./order.schema.json
polyapi schema update Order --context billing --definition '{"type":"object"}'
polyapi schema delete Order --context billing
```

`schema list` prints NAME, VISIBILITY, and ID. The collection endpoint does not include `definition`; use `schema get` for the JSON Schema.

## Create / update fields

| Flag | Notes |
| --- | --- |
| `--name` / `--context` | Required on create. On update, `--name` renames; `--new-context` moves. |
| `--definition` / `--definition-file` | JSON Schema object. Create requires one of these. A `{ "definition": { … } }` wrapper is accepted. |
| `--visibility` | `PUBLIC`, `TENANT`, or `ENVIRONMENT` (create default `ENVIRONMENT`). |

If `$schema` is omitted, the platform defaults to draft-06. Nested schemas use `x-poly-ref` `{ "path": "context.Name" }` (optional `publicNamespace` for public schemas).

Create sends:

```json
{
  "name": "Order",
  "context": "billing",
  "definition": { "type": "object", "properties": { "sku": { "type": "string" } } },
  "visibility": "ENVIRONMENT"
}
```

## Glide

`polyapi schema init` writes a JSONC scaffold (default `src/artifacts/schemas/<name>.jsonc`, or `src/<context>/artifacts/schemas/<name>.jsonc` with `--context`) with an empty object definition. Without `--snippet` it does not call the API. `--snippet` copies a JSON/JSONC snippet as the file (name and context overwritten). Only `--name` is required.

JSON/JSONC under `**/schemas/**` (or code + `polyConfig`). Deploy payload is `name`, `context`, `definition`, `visibility`. Plan/push GET the schema by id because list DTOs omit `definition`. The platform may inject `$schema` and `additionalProperties` into the stored definition; compare keeps only the keys the local file sets.

```json
{
  "name": "Order",
  "context": "billing",
  "definition": {
    "type": "object",
    "properties": {
      "sku": { "type": "string" },
      "customer": { "x-poly-ref": { "path": "billing.Customer" } }
    }
  }
}
```

`deploy validate --only refs` fails when an `x-poly-ref` path is not in the local set (public-namespace refs are skipped).
