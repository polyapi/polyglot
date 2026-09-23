# GraphQL subscriptions

`polyapi subscription` (aliases `subscriptions`, `graphql-subscription`, `gql-subscription`) manages GraphQL subscriptions. Poly keeps a long-lived websocket open to an upstream provider and invokes a server function (or an attached queue) whenever the provider pushes an event.

Two types:

- `CUSTOM` — general GraphQL streams
- `OHIP` — Oracle Hospitality Integration Platform streams (replay, offsets, keep-alives)

Delivery is **at-least-once**, not exactly-once. Handler args are `[event, params]`. Lookup is by ID, `context.name`, or name plus `--context` (and a name that is unique on the instance). Names must start with a letter or underscore and contain only letters, digits, and underscores (no hyphens).

REST collection: `GET/POST /subscriptions/graphql`, `GET/PATCH/DELETE /subscriptions/graphql/{id}`, `POST /subscriptions/graphql/{id}/recover`. Permission: `manageGraphQLSubscriptions` (list is API-key only). Mutating calls accept `--otp` when the instance requires MFA.

Platform docs: [Managing GraphQL Subscriptions](https://docs.polyapi.io/graphql_subscriptions/managing-subscriptions.html), [OHIP GraphQL Subscriptions](https://docs.polyapi.io/graphql_subscriptions/ohip-subscriptions.html).

## Commands

```bash
polyapi subscription init --name ordersStream --type CUSTOM
polyapi subscription init --name operaEvents --type OHIP --context opera
polyapi subscription list --context shopify
polyapi subscription get shopify.ordersStream
polyapi subscription create --name ordersStream --context shopify --type CUSTOM \
  --websocket-url wss://example.com/graphql \
  --query 'subscription { orderUpdated { id } }' \
  --function shopify.handleOrder
polyapi subscription update shopify.ordersStream --enabled=false
polyapi subscription recover opera.events --ohip-offset-selection PERSISTED
polyapi subscription delete shopify.ordersStream
```

`subscription list` prints NAME, ENABLED, and ID. The list DTO omits type, URL, query, and function — use `get` for the full resource. `get` redacts `paramsObject.ohip.clientSecret`.

## Create / update fields

| Flag | Notes |
| --- | --- |
| `--name` / `--context` | Required on create. On update, `--name` renames; `--new-context` moves. |
| `--type` | `CUSTOM` or `OHIP` (required on create). |
| `--websocket-url` | `ws://` or `wss://`. Transport is always WS (`HTTP` is not supported). |
| `--query` / `--query-file` | GraphQL subscription document. Create requires one. |
| `--function` | Destination server function: `id` or `context.name`. |
| `--params-variable` / `--params-sfx` / `--params-object` | Connection params. At most one. OHIP create requires `paramsObject.ohip` with `hostName`, `appKey`, `enterpriseId`, `clientId`, `clientSecret`. |
| `--function-params` | JSON object passed to the destination function. |
| `--visibility` | `TENANT` or `ENVIRONMENT` (create default `ENVIRONMENT`). **The platform currently ignores visibility on PATCH.** |
| `--enabled` | Create default `true`. |
| `--ohip-offset-selection` | Create: `PROVIDER_HIGHEST` or `SPECIFIC` (needs `--ohip-offset`). Update/recover: `PROVIDER_HIGHEST` or `PERSISTED`. |
| `--ohip-maintain-offset` | Persist OHIP checkpoints after a successful handler. Requires the query to select `metadata.offset`. |
| `--event-inactivity-threshold-ms` | 60000–86400000, or `0` on update to clear. Does not restart the stream. |
| `--queue-id` | Optional queue UUID. When set, events go to the queue instead of invoking the function directly (`functionId` is still required). |
| `--otp` | MFA header `x-otp`. |

Create sends (CUSTOM):

```json
{
  "name": "ordersStream",
  "context": "shopify",
  "type": "CUSTOM",
  "visibility": "ENVIRONMENT",
  "transportProtocol": "WS",
  "websocketUrl": "wss://example.com/graphql",
  "query": "subscription { orderUpdated { id } }",
  "functionId": "…",
  "enabled": true
}
```

If create-time start fails, the platform deletes the new row. Update-time start failure sets `enabled=false`.

## Restart warning

PATCH restarts the live websocket when the body **includes** `query`, `websocketUrl`, `type`, params, `functionParams`, or an OHIP offset-selection / maintain-offset change — presence, not “value differs”. A full-document PATCH of an unchanged query still reconnects the stream.

Does **not** restart: `name`, `context`, `description`, `queueId`, `eventInactivityThresholdMs`, `functionId`. `enabled=false` stops; `enabled=true` starts if down.

`subscription update` and Glide send **only the fields that changed**.

`ohipOffset` is runtime checkpoint state. It is not in the update DTO and is not deployed from JSONC.

## Recover

`polyapi subscription recover` is an operator action (not Glide). It force-stops the current owner and optionally restarts. `--restart` defaults true; pass `--restart=false` to stop only. `--ohip-offset-selection PROVIDER_HIGHEST` is allowed here because it is explicit.

## Glide

`polyapi subscription init --name NAME --type CUSTOM|OHIP` writes JSONC (default `src/artifacts/subscriptions/<name>.jsonc`, or `src/<context>/artifacts/subscriptions/<name>.jsonc` with `--context`). `--type` is required. `--snippet` must match `--type` when the snippet JSON has a `type` field. Without `--snippet` this does not call the API.

JSON/JSONC under `**/subscriptions/**` or `**/graphql-subscriptions/**`. Type aliases: `subscription`, `subscriptions`, `graphql-subscription`, `gql`.

Deploy payload (authorable): `name`, `context`, `description`, `visibility`, `type`, `transportProtocol`, `websocketUrl`, `query`, `functionId`, `functionParams`, `paramsVariableId`, `paramsSfxId`, `paramsObject`, `enabled`, `ohipMaintainOffset`, `eventInactivityThresholdMs`, `queueId`.

`functionId` and `paramsSfxId` may be `context.name` and are rewritten to server-function UUIDs. `paramsVariableId` rewrites to a variable UUID.

Plan/push GET by id because list DTOs omit query/URL/function. Compare ignores `ohipOffset` and other runtime fields. Empty or placeholder `paramsObject` (including `{{ENV_TOKEN}}` OHIP secrets) does not overwrite vaulted remote params. Pull GET-by-id and redacts `clientSecret`; it does not write `ohipOffset`.

Subscriptions deploy after snippets and before applications.
