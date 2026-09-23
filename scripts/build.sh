#!/bin/sh
# =============================================================================
# polyapi build.sh — cross-compile versioned binaries into builds/
# =============================================================================
#
# Contributor / CI helper. This is not the end-user installer (that is
# install.sh). You need a Go toolchain on PATH. CGO is forced off so the
# same script can cross-compile from any host.
#
# What it does
#   1. Resolves version / commit / date (env, then git, then 0.1.0).
#   2. `go build`s polyapi for each target OS/arch into builds/, with the
#      version in the filename so 0.1.0 and 0.2.0 can sit side by side:
#        builds/polyapi-<ver>-<goos>-<goarch>[.exe]
#   3. Writes SHA-256 sums for *this* version only:
#        builds/checksums-<ver>.txt   (kept so older versions stay listed)
#        builds/checksums.txt         (copy of the file above, for the
#                                      GitHub Release asset name install.sh
#                                      looks up)
#
# What it never does
#   - Does not publish a GitHub Release (dist.yml does that after calling us).
#   - Does not install the binary onto PATH.
#   - Does not enable CGO, so Darwin/Windows binaries can be produced on Linux.
#
# Usage
#   scripts/build.sh           # linux/darwin/windows × amd64/arm64
#   scripts/build.sh --native  # this machine only (faster local check)
#   scripts/build.sh --help
#
# Env (all optional)
#   POLYAPI_VERSION  semver or git tag (a leading v is stripped)
#   POLYAPI_COMMIT   git SHA stamped into the binary
#   POLYAPI_DATE     RFC3339 UTC timestamp stamped into the binary
#   POLYAPI_OUT      output directory (default: <repo>/builds)
#   POLYAPI_NATIVE=1 same as --native
# =============================================================================

# Fail fast: unset variables are errors, and any failed build aborts before
# we write a partial checksums file for a version that did not finish.
set -eu

# Repo root is the parent of scripts/, regardless of the caller's cwd.
# CDPATH= avoids `cd` printing a path when CDPATH is set in the environment.
ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
OUT=${POLYAPI_OUT:-"$ROOT/builds"}

# --native / POLYAPI_NATIVE=1: only the host GOOS/GOARCH. Used by tests and
# by people who just want a stamped local binary without waiting on five
# other cross-compiles.
NATIVE=0
if [ "${1:-}" = "--native" ] || [ "${1:-}" = "-n" ]; then
	NATIVE=1
fi
if [ "${POLYAPI_NATIVE:-}" = "1" ]; then
	NATIVE=1
fi

# --help prints the header above (the comment block) and exits. The line
# range must stay in sync with the closing "====" of that header.
if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
	sed -n '2,37p' "$0" | sed 's/^# \{0,1\}//'
	exit 0
fi

if ! command -v go >/dev/null 2>&1; then
	echo "scripts/build.sh: go is required on PATH" >&2
	exit 1
fi

# --- stamp: version / commit / date ------------------------------------------
#
# These become -X github.com/polyapi/polyglot/src/version.Version (etc.) so
# `polyapi version` reports what you actually built. dist.yml passes all
# three from the git tag and GITHUB_SHA; a local run fills them from git.

ver=${POLYAPI_VERSION:-}
if [ -z "$ver" ]; then
	if command -v git >/dev/null 2>&1 && git -C "$ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
		# Prefer an exact tag (v0.2.0) so a tagged checkout stamps cleanly.
		# Otherwise `git describe` (v0.2.0-5-gabc1234) so untagged commits
		# are still distinguishable. No tags at all → fall through to 0.1.0.
		if ver=$(git -C "$ROOT" describe --tags --exact-match 2>/dev/null); then
			:
		elif ver=$(git -C "$ROOT" describe --tags 2>/dev/null); then
			:
		else
			ver=
		fi
	fi
fi
# Asset names and ldflags use 0.2.0, not v0.2.0.
ver=${ver#v}
if [ -z "$ver" ]; then
	ver=0.1.0
fi

commit=${POLYAPI_COMMIT:-}
if [ -z "$commit" ] && command -v git >/dev/null 2>&1; then
	commit=$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || true)
fi

date=${POLYAPI_DATE:-}
if [ -z "$date" ]; then
	date=$(date -u +%Y-%m-%dT%H:%M:%SZ)
fi

# SHA-256 of files, portable across Linux (sha256sum) and macOS (shasum).
# Output format is what install.sh / install.ps1 parse: "<hex>  <filename>".
checksum() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$@"
	else
		shasum -a 256 "$@"
	fi
}

mkdir -p "$OUT"

# Target list is GOOS/GOARCH pairs. Cross-compile works because CGO_ENABLED=0
# and this module does not link C. Keep this list in sync with what
# install.sh is willing to detect (linux/darwin/windows × amd64/arm64).
if [ "$NATIVE" -eq 1 ]; then
	targets="$(go env GOOS)/$(go env GOARCH)"
else
	targets="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64"
fi

# -s -w strip symbol/debug tables (smaller release binaries).
# The -X flags overwrite the defaults in src/version (Version=0.1.0, empty
# Commit/Date). A local `go build` without this script still fills commit
# from VCS via debug.ReadBuildInfo.
pkg=github.com/polyapi/polyglot/src/version
ldflags="-s -w -X ${pkg}.Version=${ver} -X ${pkg}.Commit=${commit} -X ${pkg}.Date=${date}"

pkgdir="$ROOT/src/cmd/polyapi"
if [ ! -f "$pkgdir/main.go" ]; then
	echo "scripts/build.sh: missing $pkgdir/main.go" >&2
	echo "scripts/build.sh: if this is a git checkout, .gitignore must not ignore a path named polyapi (use /polyapi for the root binary)" >&2
	exit 1
fi

hostos=$(go env GOHOSTOS)

echo "building polyapi ${ver} -> ${OUT}"
for pair in $targets; do
	goos=${pair%/*}
	goarch=${pair#*/}
	ext=
	if [ "$goos" = windows ]; then
		ext=.exe
	fi
	# Same pattern as version.AssetName: polyapi-<ver>-<os>-<arch>[.exe]
	name="polyapi-${ver}-${goos}-${goarch}${ext}"
	echo "  ${name}"
	# -C "$ROOT" so the script is safe to run from any cwd.
	CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -C "$ROOT" --trimpath -ldflags "$ldflags" -o "${OUT}/${name}" ./src/cmd/polyapi

	# Remove debug symbols from the binary. strip(1) only understands the
	# host object format, so skip when cross-compiling to another OS
	# (GNU strip on the Ubuntu runner cannot process Mach-O or PE).
	if [ "$goos" = "$hostos" ]; then
		strip "${OUT}/${name}"
	fi

	# TODO: Investigate performance and tradeoffs with using binary compression.
	# # Compress the executable (test performance impact before using in production).
	# upx -9 ${OUT}/${name} --force-macos

	# Check the final binary size.
	ls -lh "${OUT}/${name}"
done

# Hash only this version's files, from inside $OUT so the checksums file
# stores bare filenames (what install.sh greps for), not absolute paths.
# Older polyapi-<otherver>-* files in builds/ keep their own
# checksums-<otherver>.txt; we do not rewrite those.
(
	cd "$OUT"
	files=
	for pair in $targets; do
		goos=${pair%/*}
		goarch=${pair#*/}
		ext=
		if [ "$goos" = windows ]; then
			ext=.exe
		fi
		files="$files polyapi-${ver}-${goos}-${goarch}${ext}"
	done
	# Word-split $files on purpose: it is a list of filenames we just built,
	# none of which contain spaces.
	# shellcheck disable=SC2086
	checksum $files >"checksums-${ver}.txt"
	# checksums.txt is the name GitHub Releases + install.sh look for.
	cp "checksums-${ver}.txt" checksums.txt
)

echo "checksums: ${OUT}/checksums-${ver}.txt"
