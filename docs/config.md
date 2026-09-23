# Config and auth

## Precedence

Layers are merged with [Viper](https://github.com/spf13/viper) (flags → env → files), then legacy SDK files fill any remaining gaps:

1. Explicit CLI flags (`--base-url`, `--api-key`, `--env`, …)
2. Environment: `POLY_API_KEY`, `POLY_API_BASE_URL` (preferred in CI)
3. Project file: `./.poly/config.toml` (path overridable with `--poly-path`)
4. User file: `$XDG_CONFIG_HOME/poly/config.toml` or `~/.config/poly/config.toml`
5. Legacy SDK files (read-only; never written back)

`polyapi config show` prints the **resolved** view with secrets redacted. `config get` / `config set` read and write non-secret keys. Environment values are never written back to disk.

## Instance shorthands

| Name | URL |
| --- | --- |
| `na1` | `https://na1.polyapi.io` |
| `eu1` | `https://eu1.polyapi.io` |
| `na2` | `https://na2.polyapi.io` |
| `dev`, `develop` | `https://dev.polyapi.io` |
| `local` | `http://localhost:3000` |

`polyapi auth login na1` and `polyapi auth login https://na1.polyapi.io` are equivalent. Unknown names must be full `http://` or `https://` URLs. There is no `setup` command.

## Secrets at rest

Project-stored API keys are AES-256-GCM ciphertext in `.poly/config.toml`:

```toml
instance = "na1"
base_url = "https://na1.polyapi.io"
api_version = "1"

[auth]
api_key_encrypted = "v1:…"
```

The data key is HKDF-SHA-256 of:

- application material compiled into the `polyapi` binary, and
- a per-device secret from the OS keychain (`service=polyapi`, `user=device-key`), falling back to `~/.config/poly/device.key` (mode `0600`) when the keychain is unavailable.

Theft of the project file alone cannot decrypt the key. Headless CI should set `POLY_API_KEY` instead of relying on a device secret.

`polyapi auth login` (and `polyapi init`) also ensures `.poly/` is listed in `.gitignore` (creates the file in a git repo if needed). The check is idempotent. Project scaffolding is documented in [init.md](init.md).

TLS/mTLS settings from the Python SDK (`mtls_cert_path`, `mtls_key_path`, `mtls_ca_path`) are **not** honored yet; that is a follow-up to POLY-CLI-3.

## Migrating from language SDKs

`polyapi` reads these locations if flags/env/project/user did not already supply a key and URL. It never writes them.

| SDK | File | Format |
| --- | --- | --- |
| TypeScript | `node_modules/.poly/.config.env` | dotenv (`POLY_API_KEY=`, `POLY_API_BASE_URL=`) |
| Python | `polyapi/.config.env` | INI section `[polyapi]` keys `poly_api_key`, `poly_api_base_url` |
| Java | `.mvn/settings.xml` | `<poly.hostUrl>` and `<poly.apiKey>` (best-effort) |

After a successful `polyapi auth login`, prefer the encrypted project file (or CI env vars) and stop committing SDK credential files.

## Language and adapter

Project config may pin the language and an explicit adapter launch command:

```toml
language = "typescript"

[adapter]
command = "node node_modules/polyapi/build/adapter.js"
```

Discovery order (first match wins): `--adapter` / `--lang` → this file → lockfile/manifest heuristics → prompt (or error with `--non-interactive`). See [delegate-protocol.md](delegate-protocol.md).

## Deploy policy

Committed (non-secret) branch→environment mapping lives in project config. `polyapi doctor` reports whether the current git branch may push:

```toml
[deploy]
unallowed_branch = "error"

[[deploy.targets]]
branches = ["main"]
environment = "prod"
production = true
```

`unallowed_branch = "error"` fails doctor (and `deploy push`) when the branch is not listed; `"warn"` (the default if unset) reports a warning and still marks push as not allowed. Secrets never belong in this file.

Deploy receipts are **off** unless you set `receipts = true` under `[deploy]` or pass `polyapi deploy push --receipts`.
