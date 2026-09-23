# `polyapi init`

Scaffold a new Poly project or inject Poly into an existing Node/Python app. Distinct from `polyapi <resource> init`, which writes a single local artifact.

```bash
polyapi init --lang typescript --template none --yes
polyapi init --lang python --template polyapi/poly-glide-template-py --yes
polyapi init --template https://github.com/acme/poly-starter --lang typescript
polyapi init --existing --template none --yes
```

## What it does

1. **Destination.** Files go in the current directory, or the nearest project root when a `package.json` or Python requirements/pyproject file is found above cwd.
2. **Language.** `--lang typescript|python` (Java is later). Interactive runs prompt; non-interactive defaults to TypeScript when nothing is detected.
3. **Runtime.** TypeScript needs Node.js 20+; Python needs Python 3.10+. If the interpreter is missing, init asks to install it (Homebrew, winget, apt/dnf/pacman). Declining prints install links and exits — re-run `polyapi init` after you install the runtime.
4. **Template (first-time only).** When `.poly/config.toml` is not present, init can unpack:
   - **none** — empty project (`--template none`)
   - **Official Glide** — `polyapi/poly-glide-template-js` or `polyapi/poly-glide-template-py`
   - **Tenant catalogue** — `GET /tenants/{id}/environments/{id}/config-variables/ProjectTemplates` (same source as `npx poly setup`)
   - **Public GitHub repo** — `owner/repo`, `https://github.com/owner/repo`, optional `@ref` / `#ref` / `/tree/ref`
   - **Zip URL or path**
5. **Python virtualenv.** Creates `.venv` (or `--venv`) when missing and installs with that interpreter.
6. **SDK deps.** TypeScript: add `polyapi` and run npm/yarn/pnpm/bun install. Python: `pip install -r requirements.txt` or `polyapi-python`.
7. **Project config.** Writes `.poly/config.toml` with `language`, `[deploy]` (`unallowed_branch = error`, `main`→prod unless `--env-map` says otherwise), and `[environments.*]` stubs.
8. **Git.** `git init` for new projects; `.poly/` (plus `node_modules/` or `.venv/`) in `.gitignore`; a `pre-commit` hook that runs `polyapi deploy prepare` unless husky/pre-commit is already configured.
9. **CI.** GitHub Actions workflow (default `--provider github`) or `.gitlab-ci.yml`. Secret placeholders: `POLY_API_KEY_PROD`, `POLY_API_BASE_URL_PROD`.

Already-initialized projects (`.poly/config.toml` present) skip the template prompt and still ensure runtime, deps, gitignore, and config.

## Flags

| Flag | Role |
| --- | --- |
| `--lang` | `typescript` or `python` (root persistent flag) |
| `--template` | `none`, official name, GitHub `owner/repo`, or a zip URL/path |
| `--name` | Package name substituted into `package.json` / `pyproject.toml` |
| `--existing` | Adopt cwd; do not `git init` |
| `--env-map` | Repeatable `branch=environment` (default `main=prod`) |
| `--provider` | `github` (default), `gitlab`, or `other` |
| `--force` | Overwrite files that already exist in the template |
| `--no-deps` | Skip package install and Python virtualenv |
| `--no-runtime` | Skip Node/Python detection and install |
| `--venv` | Virtualenv directory (default `.venv`) |
| `--yes` / `-y` | Accept defaults (official template on first run; install missing runtime when a package manager is available) |
| `--non-interactive` | Do not prompt. First-time runs need `--template` (or `--yes` for the official template). |

Existing files are never overwritten without `--force` or an interactive confirm. `--yes` without `--force` leaves conflicts in place.

## Missing runtimes

**Node.js:** https://nodejs.org/en/download  
**Python:** https://www.python.org/downloads/ — [venv](https://docs.python.org/3/library/venv.html), [primer](https://realpython.com/python-virtual-environments-a-primer/)

## Next steps after init

```bash
polyapi auth login
polyapi generate
```

Create the git remote, add `POLY_API_KEY_PROD` and `POLY_API_BASE_URL_PROD` as host secrets, and grant the workflow write permission so deploy can push receipts if you enable them.
