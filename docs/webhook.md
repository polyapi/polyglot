# Webhooks

`polyapi webhook` is the imperative tree for webhook handles. Create, test, and print a URL without Canopy.

Glide already treats webhooks as first-class deployables (`type: webhook`). Security function ids in source may be `context.name`; the host rewrites them to UUIDs on plan/push.

Platform docs: [Webhooks](https://docs.polyapi.io/webhooks/).

## Commands

```bash
polyapi webhook init --name hook
polyapi webhook list --context billing
polyapi webhook get hook --context billing
polyapi webhook get billing.hook
polyapi webhook create --name hook --context billing --event-payload '{"n":3}'
polyapi webhook update hook --context billing --description "Orders inbound"
polyapi webhook delete hook --context billing
polyapi webhook url hook --context billing
polyapi webhook test hook --context billing --data '{"n":5}'
```

Get/update/delete/test/url take an ID, a `context.name`, or a name with `--context` (case-insensitive, then exact).

`webhook list` prints NAME, VISIBILITY, and ID. Use `webhook get` for URL, method, security functions, and payload schema.

## Create / update fields

| Flag | Notes |
| --- | --- |
| `--name` / `--context` | Required on create. On update, `--name` renames; `--new-context` moves. |
| `--visibility` | `PUBLIC`, `TENANT`, or `ENVIRONMENT` (create default `ENVIRONMENT`). |
| `--method` | `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`, `OPTIONS`, or a comma-separated list (create default `POST`). |
| `--slug` / `--subpath` | Optional pretty URL and path/query template. |
| `--require-api-key` | Require a Poly API key on incoming requests. |
| `--event-payload` / `--event-payload-file` | Sample event (JSON if valid, otherwise a string). |
| `--event-payload-schema` | JSON Schema object for the event. |
| `--response-payload` / `--response-payload-file` | Static response body. |
| `--response-headers` | JSON object. |
| `--response-status` | 200–599. |
| `--security-function` | Repeatable. `id`, `context.name`, or JSON `{id,message}`. Resolved to a server-function UUID. |
| `--security-functions` | JSON array alternative to `--security-function`. |
| `--xml-parser` | JSON object of XML parser options. |

## Test and URL

`webhook test` `POST`s JSON to `/webhooks/{id}` (the same execute route as the docs' curl example). Default body is `{}`. A 2xx plain-text body is printed as a string.

`webhook url` prints the `url` from `GET /webhooks/{id}` (slug-based when the webhook has a slug).

## Security functions

Each entry is a **server function** that receives body, headers, and parsed params and returns `true` to accept the request. Pass `context.name` or a UUID:

```bash
polyapi webhook update hook --context billing \
  --security-function '{"id":"billing.hasValidCode","message":"Invalid or missing code"}'
```

## Glide

`polyapi webhook init` writes a JSONC scaffold (default `src/artifacts/webhooks/<name>.jsonc`, or `src/<context>/artifacts/webhooks/<name>.jsonc` with `--context`) with placeholders for every create-DTO field. Without `--snippet` it does not call the API. `--snippet` copies a JSON/JSONC snippet as the file (name and context overwritten). Only `--name` is required.

Webhooks are discovered as code + typed `polyConfig`, or JSON/JSONC under `**/webhooks/**`. Deploy payload is the create DTO fields only (`name`, `context`, `description`, `visibility`, `method`, `securityFunctions`, …). Plan/push `GET` the webhook by id because list DTOs omit security functions.

See [docs/glide.md](glide.md) and [docs/trigger.md](trigger.md).
