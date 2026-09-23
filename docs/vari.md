# Vari (variables)

`polyapi vari` is the cross-language replacement for Java `mvn polyapi:create-server-variable` and for creating/updating/deleting variables in Canopy.

Glide already treats variables as first-class deployables (`type: variable`). This pack adds the imperative command tree and documents secret handling.

## Commands

```bash
polyapi vari init --name apiKey
polyapi vari list --context billing
polyapi vari get apiKey --context billing
polyapi vari get apiKey --context billing --value          # non-secret only
polyapi vari create --name apiKey --context billing --value '"sk-live"' --secret
polyapi vari create --name config --context billing --value-file ./config.json
polyapi vari update apiKey --context billing --value '"sk-new"'
polyapi vari delete apiKey --context billing
polyapi vari copy apiKey --context billing --to-context staging
polyapi vari copy secretKey --context billing --to-context staging --value '"sk-staging"'
```

Get/update/delete/copy take an ID, or a name with `--context` (case-insensitive, then exact, matching `function`).

`vari list` prints NAME, VISIBILITY, and ID. The collection endpoint does not include secrecy; use `vari get` for that.

## Create / update fields

| Flag | Notes |
| --- | --- |
| `--name` / `--context` | Required on create. On update, `--name` renames; `--new-context` moves. |
| `--value` / `--value-file` | JSON if the text is valid JSON, otherwise a string. Create requires one of these. |
| `--secret` | Store as `secrecy=SECRET`. |
| `--secrecy` | `NONE`, `SECRET`, or `OBSCURED`. `PARTIAL` is rejected. |
| `--visibility` | `PUBLIC`, `TENANT`, or `ENVIRONMENT` (create default `ENVIRONMENT`). |
| `--description` | Optional. |
| `--expires-at` | Optional ISO-8601 expiry. |
| `--otp` | Sent as `x-otp` on update/delete when the instance requires MFA. |

## Secrets

- SECRET / OBSCURED / PARTIAL values are **never printed**. `vari get` redacts even if the API returned plaintext.
- `vari get --value` is refused for SECRET variables (use `.inject()` in a server or API function).
- `vari copy` of a SECRET variable requires `--value`. The platform does not return secret values, so a copy cannot reuse the source secret.
- Glide **never deploys SECRET values from source**. A JSONC/code variable with `secrecy: SECRET` is pushed with `value: {}`. Set the real secret in Canopy, `vari update`, or an env token on a non-secret field. OBSCURED values are kept (readable at runtime; poly-flow parity).
- JSONC artifacts may use `{{ENV_TOKEN}}`; unresolved tokens fail validate/plan/push unless `--allow-unresolved`.

## Glide

Variables are discovered as:

- **Code + typed `polyConfig`** (recommended): `polyConfig: PolyVariable = { … }` / `export const polyConfig =`. Go finds the binding; the language adapter `extract`s the payload (POLY-CLI-11/12).
- **JSON/JSONC** (flow compat): files under `**/vari/**` or `**/variables/**`, with `{{TOKEN}}` substitution.

`polyapi vari init` writes a JSONC scaffold (default `src/artifacts/vari/<name>.jsonc`, or `src/<context>/artifacts/vari/<name>.jsonc` with `--context`) with placeholders for name, context, description, visibility, secrecy, value, and expiresAt. Without `--snippet` it does not call the API. `--snippet` copies a JSON/JSONC snippet as the file (name and context overwritten). Only `--name` is required.

Deploy order puts variables first. Existing obscured remotes require `--force` to update.

See [docs/glide.md](glide.md).

## Copy vs promote

`vari copy` duplicates a variable **in the current environment** (new name and/or context). Promoting across Poly environments/instances is POLY-CLI-14 (`env` / `replicate` + Glide push with another key).
