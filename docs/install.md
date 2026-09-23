# Install

`polyapi` is a single static binary. You do not need a Go toolchain (or Node / Python / Java) to run agnostic commands.

## Install script (no Go)

```bash
curl -fsSL https://raw.githubusercontent.com/polyapi/polyglot/main/scripts/install.sh | sh
```

The script is in this repo at [`scripts/install.sh`](../scripts/install.sh) so it can be read before you pipe it. It:

1. Detects OS/arch (`linux`/`darwin`/`windows` × `amd64`/`arm64`)
2. Downloads the matching GitHub Release asset and `checksums.txt`
3. Verifies the SHA-256
4. Writes `polyapi` into the install directory, **replacing** an existing `polyapi` on `PATH` when found
5. Checks that the install directory is on `PATH`. If it is not, appends a marked `export PATH=…` block to the shell rc file (`~/.zshrc`, `~/.bashrc`, or `~/.profile` from `$SHELL`; fish uses `~/.config/fish/config.fish`). Search the file for `>>> polyapi PATH >>>` to audit or remove it. `curl | sh` cannot change the parent shell — the script prints an `export PATH=…` line for the current session. Set `POLYAPI_SKIP_PATH=1` to leave PATH alone.

Default install directory: the directory of an existing `polyapi`, else `/usr/local/bin` if writable, else `~/.local/bin`.

```bash
# specific version
curl -fsSL https://raw.githubusercontent.com/polyapi/polyglot/main/scripts/install.sh | sh -s -- v0.2.0

# pick the destination
POLYAPI_INSTALL_DIR=~/.local/bin ./scripts/install.sh
```

Windows (PowerShell), same checksum rules: [`scripts/install.ps1`](../scripts/install.ps1). If the install directory is not on `PATH`, it prepends it to the **user** PATH (not the machine PATH).

```powershell
irm https://raw.githubusercontent.com/polyapi/polyglot/main/scripts/install.ps1 | iex
```

Then:

```bash
polyapi version
polyapi update --check
```

## GitHub Releases

[`.github/workflows/dist.yml`](../.github/workflows/dist.yml) cross-compiles checksummed binaries and publishes a GitHub Release. Run it from Actions (uses `src/version.Version`, or a version input) or push a `v*` tag. A new version creates the git tag and the Release; the same version refreshes the assets.

| File | What |
| --- | --- |
| `polyapi-<version>-<os>-<arch>` (`.exe` on Windows) | The CLI |
| `checksums.txt` | SHA-256 of every asset in that release |

GitHub artifact attestations are attached (`gh attestation verify`). Apple notarization and Windows Authenticode are not part of this ticket.

## Self-update

```bash
polyapi update --check   # report only
polyapi update           # download, verify checksum, replace this binary
```

`POLY_UPDATE_API` / `POLY_UPDATE_DEST` are test overrides, not user config.

## Local / contributor builds

Requires Go 1.27+. [`scripts/build.sh`](../scripts/build.sh) cross-compiles into `builds/` with the version in the filename so old and new binaries can coexist:

```bash
scripts/build.sh           # six OS/arch pairs
scripts/build.sh --native  # this machine only
ls builds/
# polyapi-0.1.0-linux-amd64
# polyapi-0.1.0-darwin-arm64
# checksums-0.1.0.txt
```

Published copies also live on GitHub Releases.

```bash
go install github.com/polyapi/polyglot/src/cmd/polyapi@latest
go build -o polyapi ./src/cmd/polyapi
```
