# Triggers

`polyapi trigger` links a webhook (or error handler) to a server function.

Triggers have **no context** on the platform DTO. Lookup is by ID or name. Source and destination are fixed at create time — update only patches `name`, `waitForResponse`, and `enabled`.

Platform docs: [Managing Triggers](https://docs.polyapi.io/webhooks/managing-triggers.html).

## Commands

```bash
polyapi trigger init --name weekly --type webhook
polyapi trigger init --name on-error --type error-handler
polyapi trigger list
polyapi trigger get weekly
polyapi trigger create --name weekly --webhook billing.hook --function billing.weeklyReport
polyapi trigger create --name on-error --error-handler-path billing.handle --function billing.weeklyReport
polyapi trigger update weekly --wait-for-response=false
polyapi trigger delete weekly
```

## Create

| Flag | Notes |
| --- | --- |
| `--name` | Optional. The platform generates `{webhook}_to_{function}` if omitted. |
| `--webhook` | Source webhook: `id` or `context.name`. |
| `--function` | Destination server function: `id` or `context.name`. Required. |
| `--error-handler-path` | Alternative source (instead of `--webhook`). |
| `--wait-for-response` | Default `true` for `--webhook`. Omitted for `--error-handler-path` unless you pass the flag (the platform stores false). |
| `--enabled` | Default `true`. |

Create sends the v1 nested body:

```json
{
  "name": "weekly",
  "source": { "webhookHandleId": "…" },
  "destination": { "serverFunctionId": "…" },
  "waitForResponse": true,
  "enabled": true
}
```

Error-handler create omits `waitForResponse` unless `--wait-for-response` is passed:

```json
{
  "name": "on-error",
  "source": { "errorHandler": { "path": "billing.handle" } },
  "destination": { "serverFunctionId": "…" },
  "enabled": true
}
```

The server function should accept `eventPayload`, `headersPayload`, and `paramsPayload`.

## Update

Only `--name`, `--wait-for-response`, and `--enabled`. Changing source or destination requires delete + create.

## Glide

`polyapi trigger init` writes a JSONC scaffold (default `src/artifacts/triggers/<name>.jsonc`). `--type webhook|error-handler` is required and selects the source shape. Triggers have no `--context`. Without `--snippet` it does not call the API. `--snippet` copies a JSON/JSONC snippet as the file (name overwritten). `--type` is still required and must match the snippet source (webhook handle vs error-handler path).

JSON/JSONC under `**/triggers/**` (or code + `polyConfig`). Author source/destination as `context.name`:

Webhook:

```json
{
  "name": "weekly",
  "source": { "webhookHandleId": "billing.hook" },
  "destination": { "serverFunctionId": "billing.weeklyReport" },
  "waitForResponse": true
}
```

Error handler (`--type error-handler`):

```json
{
  "name": "on-error",
  "source": { "errorHandler": { "path": "billing.handle" } },
  "destination": { "serverFunctionId": "billing.weeklyReport" }
}
```

Error-handler triggers omit `waitForResponse` (the platform stores it as false). You may also author `webhookContext`/`webhookName` and `functionContext`/`functionName`; the host rewrites those to UUIDs (webhooks deploy before triggers). Compare flattens nested create payloads against the flat Trigger DTO. A source/destination drift fails the item (`delete and recreate`) instead of PATCHing fields the platform ignores.

See [docs/glide.md](glide.md) and [docs/webhook.md](webhook.md).
