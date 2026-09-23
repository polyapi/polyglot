#!/bin/sh
# =============================================================================
# polyapi install.sh — download one checksummed GitHub Release binary
# =============================================================================
#
# Read this file before piping it to a shell. It is the supported no-Go
# installer. There is no other hidden step: this script is the whole install.
#
# What it does
#   1. Figures out this machine's OS and CPU (linux/darwin/windows × amd64/arm64).
#   2. Picks a release tag (an argument, POLYAPI_VERSION, or GitHub "latest").
#   3. Downloads exactly two files over HTTPS into a temp directory:
#        - the matching binary  (polyapi-<ver>-<os>-<arch>[.exe])
#        - checksums.txt        (SHA-256 list published with that release)
#   4. Refuses to install unless the binary's SHA-256 matches checksums.txt.
#   5. Moves that binary to the install directory as "polyapi" (or polyapi.exe),
#      replacing an existing install if one is already on PATH.
#   6. Checks that the install directory is on PATH. If it is not, appends one
#      marked `export PATH="…"` block to the user's shell rc file (see
#      ensure_on_path below) so new terminals can run `polyapi`.
#
# What it never does (audit this list)
#   - Does not run `eval`, `source`, or otherwise execute the downloaded bytes
#     as a script. The download is a Go binary; it is only chmod +x and moved.
#   - Does not run `polyapi` after installing. You do that yourself.
#   - Does not send your API keys, config, or environment to anyone. The only
#     outbound requests are GET to GitHub (or the URLs you override below).
#   - Does not rewrite your rc file. If PATH is missing the install dir, it
#      *appends* a 3-line marked block (search for ">>> polyapi PATH >>>").
#      Re-running the installer will not add a second copy of that block.
#   - Does not edit the system PATH, /etc/paths, or launchd plists.
#   - Does not install a package manager, compiler, or anything except polyapi.
#   - Does not follow a redirect onto a different host for the GitHub API JSON;
#     curl/wget -L is used because GitHub *asset* URLs 302 to
#     objects.githubusercontent.com. The checksum still has to match.
#   - Does not skip the checksum. A missing or mismatched sum is a hard error.
#
# Network (default; override with env if you are mirroring)
#   GET  https://api.github.com/repos/polyapi/polyglot/releases/latest
#        (only when you did not pass a version — used solely to read tag_name)
#   GET  https://github.com/polyapi/polyglot/releases/download/<tag>/<asset>
#   GET  https://github.com/polyapi/polyglot/releases/download/<tag>/checksums.txt
#
# Usage
#   curl -fsSL https://raw.githubusercontent.com/polyapi/polyglot/main/scripts/install.sh | sh
#   ./scripts/install.sh              # latest release
#   ./scripts/install.sh v0.2.0       # specific tag
#   POLYAPI_INSTALL_DIR=~/.local/bin ./scripts/install.sh
#
# Env (all optional)
#   POLYAPI_VERSION       tag or x.y.z; wins over the CLI argument
#   POLYAPI_INSTALL_DIR   directory to write polyapi into (created if needed)
#   POLYAPI_REPO          GitHub owner/name (default: polyapi/polyglot)
#   POLYAPI_RELEASES_URL  latest-release JSON URL (tests / mirrors)
#   POLYAPI_DOWNLOAD_BASE asset base, no trailing slash
#                         (default: https://github.com/<repo>/releases/download)
#   POLYAPI_SKIP_PATH=1   do not edit shell rc files (tests / you manage PATH)
#
# Requires: curl or wget; sha256sum (Linux) or shasum (macOS); POSIX sh.
# sudo is used only for the final move, and only if the dest directory is
# not writable by this user. PATH edits never use sudo — they only write
# files under $HOME.
# =============================================================================

# Fail fast: unset variables are errors, and any command that fails aborts
# the script. We never continue after a failed download or checksum.
set -eu

# --- configuration -----------------------------------------------------------

# GitHub repo that publishes the binaries. Override POLYAPI_REPO if you are
# installing from a fork that cuts the same asset names.
REPO=${POLYAPI_REPO:-polyapi/polyglot}

# Base URL for release assets. The two downloads are:
#   ${DOWNLOAD_BASE}/${tag}/${asset}
#   ${DOWNLOAD_BASE}/${tag}/checksums.txt
DOWNLOAD_BASE=${POLYAPI_DOWNLOAD_BASE:-"https://github.com/${REPO}/releases/download"}

# GitHub "latest release" JSON. Used only to read "tag_name" when the caller
# did not pin a version. We parse that one field with sed — we do not eval JSON.
RELEASES_URL=${POLYAPI_RELEASES_URL:-"https://api.github.com/repos/${REPO}/releases/latest"}

# Pinned version: env wins, then the first CLI argument, otherwise "ask GitHub".
VERSION=${POLYAPI_VERSION:-${1:-}}

# --- helpers -----------------------------------------------------------------

# Print a message to stderr and exit 1. Every fatal path goes through here so
# the script never installs a half-written binary.
die() {
	echo "install.sh: $*" >&2
	exit 1
}

# Require an executable on PATH (used only for sudo, and only if we need it).
need_cmd() {
	command -v "$1" >/dev/null 2>&1 || die "need $1 on PATH"
}

# GET $url and write the body to file $out.
# curl flags: -f fail on HTTP >= 400, -s silent, -S still print errors, -L
# follow redirects (GitHub asset URLs redirect once to the object store).
# wget is the fallback if curl is missing. We never pipe the body to a shell.
http_get() {
	url=$1
	out=$2
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL -H "User-Agent: polyapi-install" -o "$out" "$url"
	elif command -v wget >/dev/null 2>&1; then
		wget -q -O "$out" "$url"
	else
		die "need curl or wget"
	fi
}

# Same as http_get, but print the body to stdout (used for the tiny latest-
# release JSON so we can pick tag_name without writing a file).
http_get_stdout() {
	url=$1
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL -H "User-Agent: polyapi-install" "$url"
	elif command -v wget >/dev/null 2>&1; then
		wget -q -O - "$url"
	else
		die "need curl or wget"
	fi
}

# SHA-256 hex digest of a file. Linux ships sha256sum; macOS ships shasum.
# We only take field 1 (the hex) so the filename never leaks into the compare.
sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		die "need sha256sum or shasum"
	fi
}

# True if directory $1 is already an entry on PATH (exact match, trailing
# slashes ignored). A substring match would be wrong: /usr/bin must not
# satisfy a check for /usr.
path_has_dir() {
	needle=${1%/}
	haystack=":$PATH:"
	case "$haystack" in
	*":${needle}:"* | *":${needle}/:"*) return 0 ;;
	esac
	return 1
}

# Which rc file should receive the PATH line. We key off $SHELL (the user's
# login shell), not the shell running this script — `curl | sh` is always `sh`,
# which would otherwise send everyone to ~/.profile.
#
#   zsh  → ~/.zshrc          (macOS default; interactive + login in Terminal)
#   bash → ~/.bashrc, else ~/.bash_profile (macOS login bash), else ~/.profile
#   fish → ~/.config/fish/config.fish
#   *    → ~/.profile
path_rc_file() {
	case ${SHELL:-} in
	*/zsh)
		echo "${HOME}/.zshrc"
		;;
	*/bash)
		if [ -f "${HOME}/.bashrc" ]; then
			echo "${HOME}/.bashrc"
		elif [ -f "${HOME}/.bash_profile" ]; then
			echo "${HOME}/.bash_profile"
		else
			echo "${HOME}/.profile"
		fi
		;;
	*/fish)
		echo "${HOME}/.config/fish/config.fish"
		;;
	*)
		echo "${HOME}/.profile"
		;;
	esac
}

# After the binary is in place: if dest_dir is not on PATH, append a marked
# block to the rc file so new terminals find `polyapi`. Idempotent — a second
# install will not add another block. POLYAPI_SKIP_PATH=1 skips this (tests).
#
# The block we write is exactly:
#   # >>> polyapi PATH >>>
#   export PATH="<dest_dir>:$PATH"          # or fish_add_path on fish
#   # <<< polyapi PATH <<<
# Search for ">>> polyapi PATH >>>" in the rc file to audit or remove it.
ensure_on_path() {
	dir=$1
	if [ "${POLYAPI_SKIP_PATH:-}" = "1" ]; then
		return 0
	fi
	if path_has_dir "$dir"; then
		echo "polyapi is on PATH (${dir})"
		return 0
	fi
	[ -n "${HOME:-}" ] || die "HOME is unset; cannot update PATH (set POLYAPI_SKIP_PATH=1 to skip)"

	rc=$(path_rc_file)
	rc_dir=$(dirname "$rc")
	mkdir -p "$rc_dir" || die "cannot create $rc_dir to update PATH"

	if [ -f "$rc" ] && grep -F ">>> polyapi PATH >>>" "$rc" >/dev/null 2>&1; then
		# A previous install already appended a block. Do not add a second
		# copy; tell the user to open a new terminal (or source the file).
		echo "PATH: ${dir} is not in this shell; a polyapi PATH block is already in ${rc}"
		echo "      open a new terminal, or: . ${rc}"
		return 0
	fi

	if [ ! -f "$rc" ]; then
		# Create an empty rc rather than refusing — otherwise a fresh machine
		# with zsh and no ~/.zshrc yet would install the binary and leave it
		# undiscoverable.
		touch "$rc" || die "cannot create $rc to update PATH"
	fi

	echo "PATH: adding ${dir} to ${rc}"
	{
		echo ""
		echo "# >>> polyapi PATH >>>"
		case ${SHELL:-} in
		*/fish)
			# fish_add_path prepends uniquely; -P keeps the physical path.
			echo "fish_add_path -P \"${dir}\""
			;;
		*)
			# dest_dir is expanded now so the rc file shows the real path
			# (easier to audit than a variable). \$PATH is escaped so it
			# expands at shell start, not at install time.
			echo "export PATH=\"${dir}:\$PATH\""
			;;
		esac
		echo "# <<< polyapi PATH <<<"
	} >>"$rc" || die "cannot write PATH entry to $rc"

	# This process can see the binary immediately. The parent of `curl | sh`
	# cannot — that is a different shell — so we also print a one-liner.
	PATH="${dir}:${PATH}"
	export PATH
	echo "PATH: this shell: export PATH=\"${dir}:\$PATH\""
	echo "      new terminals: source ${rc}  (or open a new window)"
}

# --- detect this machine -----------------------------------------------------

# Map uname to the GOOS/GOARCH names used in the published asset filenames
# (see src/version.AssetName). Unknown values abort rather than guessing.
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
linux) os=linux ;;
darwin) os=darwin ;;
mingw*|msys*|cygwin*) os=windows ;;
*) die "unsupported OS: $os (need linux, darwin, or windows)" ;;
esac

arch=$(uname -m)
case "$arch" in
x86_64|amd64) arch=amd64 ;;
aarch64|arm64) arch=arm64 ;;
*) die "unsupported architecture: $arch (need amd64 or arm64)" ;;
esac

# --- pick a release tag ------------------------------------------------------

# No version given: fetch GitHub's latest-release JSON and pull tag_name.
# The sed is deliberately dumb (one field, first match) so we do not need jq
# and we never execute anything from the JSON.
if [ -z "$VERSION" ]; then
	json=$(http_get_stdout "$RELEASES_URL") || die "could not fetch latest release from GitHub"
	VERSION=$(printf '%s\n' "$json" | tr ',' '\n' | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
	[ -n "$VERSION" ] || die "GitHub latest release JSON had no tag_name"
fi

# Tags on GitHub are v-prefixed (v0.2.0). Asset filenames drop the v
# (polyapi-0.2.0-linux-amd64). Accept either form from the user.
tag=$VERSION
case "$tag" in
v*) ;;
*) tag="v$tag" ;;
esac
ver=${tag#v}

# Installed command is always "polyapi". The downloaded *asset* has the
# version, OS, and arch in its name so checksums.txt can list every platform.
ext=
bin=polyapi
if [ "$os" = windows ]; then
	ext=.exe
	bin=polyapi.exe
fi
asset="polyapi-${ver}-${os}-${arch}${ext}"

# --- pick the install directory ---------------------------------------------
#
# Priority:
#   1. POLYAPI_INSTALL_DIR — caller is explicit; we never look at PATH.
#   2. Directory of an existing polyapi on PATH — replace in place.
#   3. /usr/local/bin if it exists and is writable (no sudo).
#   4. ~/.local/bin (created later; we add it to PATH if needed).
#
if [ -n "${POLYAPI_INSTALL_DIR:-}" ]; then
	dest_dir=$POLYAPI_INSTALL_DIR
elif existing=$(command -v polyapi 2>/dev/null); then
	dest_dir=$(dirname "$existing")
elif existing=$(command -v polyapi.exe 2>/dev/null); then
	dest_dir=$(dirname "$existing")
elif [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
	dest_dir=/usr/local/bin
else
	dest_dir="${HOME}/.local/bin"
fi

mkdir -p "$dest_dir" || die "cannot create $dest_dir"
# Absolute, no trailing slash, so PATH matching and the rc-file export agree.
dest_dir=$(CDPATH= cd -- "$dest_dir" && pwd)
dest_dir=${dest_dir%/}
dest="${dest_dir}/${bin}"

# Temp dir holds the two downloads. The trap deletes it on success, failure,
# or Ctrl-C so a failed checksum never leaves a binary sitting around.
tmp=$(mktemp -d 2>/dev/null || mktemp -d -t polyapi-install)
trap 'rm -rf "$tmp"' EXIT INT HUP

# --- download and verify -----------------------------------------------------

echo "downloading ${asset} (${tag})"
http_get "${DOWNLOAD_BASE}/${tag}/${asset}" "${tmp}/${asset}" || die "download failed: ${DOWNLOAD_BASE}/${tag}/${asset}"
http_get "${DOWNLOAD_BASE}/${tag}/checksums.txt" "${tmp}/checksums.txt" || die "download failed: ${DOWNLOAD_BASE}/${tag}/checksums.txt"

# checksums.txt is `sha256sum` format: "<hex>  <filename>" (two spaces, or
# " *" for binary mode). Match on the filename field only; print the hex.
expected=$(awk -v f="$asset" '{
	name=$NF
	sub(/^\*/, "", name)
	if (name == f) { print $1; exit }
}' "${tmp}/checksums.txt")
[ -n "$expected" ] || die "checksums.txt has no entry for ${asset}"

# Compare lowercase hex. Mismatch is fatal — we do not install "anyway".
expected=$(printf '%s' "$expected" | tr '[:upper:]' '[:lower:]')
actual=$(sha256_of "${tmp}/${asset}" | tr '[:upper:]' '[:lower:]')
[ "$expected" = "$actual" ] || die "checksum mismatch for ${asset} (expected ${expected}, got ${actual})"

# --- install (replace existing) ----------------------------------------------

# The verified file is moved into place as polyapi. If dest_dir is not
# writable we escalate *only* the move+chmod, not the download.
if [ ! -w "$dest_dir" ]; then
	need_cmd sudo
	sudo mv "${tmp}/${asset}" "$dest"
	sudo chmod 755 "$dest"
else
	mv "${tmp}/${asset}" "$dest"
	chmod 755 "$dest"
fi

echo "installed ${dest} (${tag})"

# Verify dest_dir is on PATH; if not, append a marked block to the shell rc.
ensure_on_path "$dest_dir"
