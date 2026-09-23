# Canopy applications

`polyapi app` (aliases `canopy`, `application`, `applications`) is the imperative tree for [Canopy](https://docs.polyapi.io/canopy/index.html) UI applications. Create, update, and delete an application without the PolyUI portal. Lookup is by ID, name, or `config.subpath` (case-insensitive, then exact). Applications have **no context**.

Platform docs: [Canopy](https://docs.polyapi.io/canopy/index.html), [Create a Canopy UI Application](https://docs.polyapi.io/canopy/create_application.html), [Architecture](https://docs.polyapi.io/canopy/architecture.html).

## Platform APIs vs portal-only (POLY-CLI-23)

The applications REST API is sufficient for CLI CRUD. Nothing in the application *record* lifecycle is portal-only.

| Method | Path | CLI |
| --- | --- | --- |
| `GET` | `/applications` (v2 paginated `results`) | `app list` |
| `GET` | `/applications/configured` | `app list --configured` |
| `POST` | `/applications` | `app create` |
| `GET` | `/applications/{id}` | `app get` |
| `PATCH` | `/applications/{id}` | `app update` |
| `DELETE` | `/applications/{id}` | `app delete` |
| `GET` | `/applications/{id}/config` | `app get --config` (from the GET-by-id body) |
| `PUT` | `/applications/{id}/config` | covered by `app update --config-file` (PATCH with `config`) |
| `DELETE` | `/applications/{id}/config` | not exposed (delete the application, or PATCH an empty collections list) |
| `GET` | `/applications/{id}/config/public` | not exposed (unauthenticated Canopy shell) |

Create requires `manageApplications`. List/get require `useApplications` or `manageApplications`.

**Still portal / Canopy-runtime, not this CLI:** rendering the generated UI, login/signup chrome, OAuth, the in-browser JSON editor, and mapping collection widgets to live function responses. The CLI stores the same JSON config the portal would. Collection `list`/`get`/`create`/`update`/`delete` entries point at Poly functions (path or id); those functions are managed with `polyapi function`, not `polyapi app`.

## Commands

```bash
polyapi app init --name dashboard
polyapi app list
polyapi app list --configured
polyapi app get dashboard
polyapi app get dashboard --config
polyapi app create --name dashboard
polyapi app create --name dashboard --config-file ./app.json --subpath partner-portal
polyapi app update dashboard --config-file ./app.json
polyapi app url dashboard
polyapi app delete dashboard
```

`app list` prints NAME, SUBPATH, VISIBILITY, and ID. List DTOs omit `config`; use `app get` (or `app get --config`) for the Canopy definition.

## Create / update fields

| Flag | Notes |
| --- | --- |
| `--name` | Required on create. On update, renames. |
| `--description` | Optional. |
| `--visibility` | `PUBLIC`, `TENANT`, or `ENVIRONMENT` (create default `ENVIRONMENT`). |
| `--subpath` | Canopy URL segment (`/canopy/<subpath>`). Written onto `config.subpath`. Create without `--subpath` slugs `--name` (`Quantum Quirk` → `quantum-quirk`). |
| `--config` / `--config-file` | Canopy config object. A `{ "config": { … } }` wrapper is accepted. Create without either sends `{name, subpath, collections: []}`. |
| `--owner` | Owner user ID. Create defaults to the authenticated user on the platform. |

`--config` and `--config-file` are mutually exclusive. Update requires at least one field.

Create sends:

```json
{
  "name": "dashboard",
  "description": "",
  "visibility": "ENVIRONMENT",
  "config": {
    "name": "dashboard",
    "subpath": "dashboard",
    "collections": []
  }
}
```

A configured app needs at least one collection with `list` and `get` mapped to functions. See [Implementing CRUD Operations in Canopy Applications](https://docs.polyapi.io/canopy/implement_crud.html).

## URL

`polyapi app url <name>` prints `{instance}/canopy/{subpath}`. Example on na1: `https://na1.polyapi.io/canopy/quantum-quirk-dashboard`. The login page is that path plus `/login`. Fails if `config.subpath` is empty.

## Glide

`polyapi app init` writes a JSONC scaffold (default `src/artifacts/applications/<name>.jsonc`). Without `--snippet` it does not call the API. `--snippet` copies a JSON/JSONC snippet as the file (name overwritten, including `config.name` when present). Only `--name` is required (no `--context`).

JSON/JSONC under `**/applications/**` (or `**/apps/**` / `**/canopy/**`) or code + `polyConfig`. Deploy payload is `name`, `description`, `visibility`, `config`. Plan/push GET the application by id because list DTOs omit `config`. Applications deploy last (after snippets) so referenced functions already exist. Function path/id rewriting inside `config.collections` is not done in v1 — point collections at real function ids or OOB paths.

```json
{
  "name": "dashboard",
  "description": "Partner portal",
  "visibility": "ENVIRONMENT",
  "config": {
    "name": "dashboard",
    "subpath": "partner-portal",
    "collections": []
  }
}
```
