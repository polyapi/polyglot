# =============================================================================
# polyapi install.ps1 — download one checksummed GitHub Release binary
# =============================================================================
#
# Read this file before piping it to PowerShell. It is the Windows counterpart
# of scripts/install.sh. There is no other hidden step: this script is the
# whole install. Behaviour matches the POSIX script on purpose so an audit of
# either one applies to both.
#
# What it does
#   1. Figures out this machine's CPU (windows × amd64/arm64).
#   2. Picks a release tag (-Version, POLYAPI_VERSION, or GitHub "latest").
#   3. Downloads exactly two files over HTTPS into a temp directory:
#        - the matching binary  (polyapi-<ver>-windows-<arch>.exe)
#        - checksums.txt        (SHA-256 list published with that release)
#   4. Refuses to install unless the binary's SHA-256 matches checksums.txt.
#   5. Copies that binary to the install directory as polyapi.exe, replacing
#      an existing install if one is already on PATH.
#   6. Checks that the install directory is on PATH. If it is not, prepends it
#      to the *user* PATH (HKCU Environment Path). Machine PATH is not touched.
#
# What it never does (audit this list)
#   - Does not Invoke-Expression / iex the downloaded bytes. The download is a
#     Go .exe; it is only copied into place.
#   - Does not run polyapi.exe after installing. You do that yourself.
#   - Does not send API keys, config, or environment anywhere. The only
#     outbound requests are GET to GitHub (or the URLs you override below).
#   - Does not edit the machine (HKLM) PATH, Run keys, or services. PATH
#      changes are user-scope only and skipped if the dir is already listed.
#   - Does not install a package manager, compiler, or anything except polyapi.
#   - Does not skip the checksum. A missing or mismatched sum is a hard error.
#
# Network (default; override with env if you are mirroring)
#   GET  https://api.github.com/repos/polyapi/polyglot/releases/latest
#        (only when you did not pass a version — used solely to read tag_name)
#   GET  https://github.com/polyapi/polyglot/releases/download/<tag>/<asset>
#   GET  https://github.com/polyapi/polyglot/releases/download/<tag>/checksums.txt
#
# Usage
#   irm https://raw.githubusercontent.com/polyapi/polyglot/main/scripts/install.ps1 | iex
#   ./scripts/install.ps1
#   ./scripts/install.ps1 -Version v0.2.0
#   $env:POLYAPI_INSTALL_DIR = "$env:LOCALAPPDATA\Programs\polyapi"
#   ./scripts/install.ps1
#
# Env / parameters (all optional)
#   -Version / POLYAPI_VERSION   tag or x.y.z
#   POLYAPI_INSTALL_DIR          directory to write polyapi.exe into
#   POLYAPI_REPO                 GitHub owner/name (default: polyapi/polyglot)
#   POLYAPI_RELEASES_URL         latest-release JSON URL (tests / mirrors)
#   POLYAPI_DOWNLOAD_BASE        asset base, no trailing slash
#                                (default: https://github.com/<repo>/releases/download)
#   POLYAPI_SKIP_PATH=1          do not edit the user PATH (tests / you manage PATH)
#
# Requires: PowerShell with Invoke-WebRequest / Invoke-RestMethod / Get-FileHash
# (Windows PowerShell 5.1 or PowerShell 7+).
# =============================================================================

param(
    # Pin a release. Env POLYAPI_VERSION is the default so a piped `iex`
    # invocation can still select a tag without editing the script.
    [string]$Version = $env:POLYAPI_VERSION
)

# Any failed cmdlet aborts the script. We never continue after a failed
# download or checksum and then copy "whatever we got".
$ErrorActionPreference = "Stop"

# --- configuration -----------------------------------------------------------

# GitHub repo that publishes the binaries. Override POLYAPI_REPO if you are
# installing from a fork that cuts the same asset names.
$repo = if ($env:POLYAPI_REPO) { $env:POLYAPI_REPO } else { "polyapi/polyglot" }

# Base URL for release assets. The two downloads are:
#   $downloadBase/$tag/$asset
#   $downloadBase/$tag/checksums.txt
$downloadBase = if ($env:POLYAPI_DOWNLOAD_BASE) { $env:POLYAPI_DOWNLOAD_BASE } else { "https://github.com/$repo/releases/download" }

# GitHub "latest release" JSON. Used only to read tag_name when the caller
# did not pin a version. Invoke-RestMethod parses JSON; we only take tag_name.
$releasesUrl = if ($env:POLYAPI_RELEASES_URL) { $env:POLYAPI_RELEASES_URL } else { "https://api.github.com/repos/$repo/releases/latest" }

# --- detect this machine -----------------------------------------------------

# Map PROCESSOR_ARCHITECTURE to the GOARCH names used in published assets
# (see src/version.AssetName). Unknown values abort rather than guessing.
# OS is always windows in this script.
$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default { throw "unsupported architecture: $($env:PROCESSOR_ARCHITECTURE) (need amd64 or arm64)" }
}

# --- pick a release tag ------------------------------------------------------

# No version given: fetch GitHub's latest-release JSON and read tag_name.
# User-Agent is required by api.github.com; Accept selects the JSON preview.
if (-not $Version) {
    $headers = @{ "User-Agent" = "polyapi-install"; "Accept" = "application/vnd.github+json" }
    $rel = Invoke-RestMethod -Uri $releasesUrl -Headers $headers
    $Version = $rel.tag_name
    if (-not $Version) { throw "GitHub latest release JSON had no tag_name" }
}

# Tags on GitHub are v-prefixed (v0.2.0). Asset filenames drop the v
# (polyapi-0.2.0-windows-amd64.exe). Accept either form from the user.
$tag = $Version
if (-not $tag.StartsWith("v")) { $tag = "v$tag" }
$ver = $tag.TrimStart("v")
$asset = "polyapi-$ver-windows-$arch.exe"

# --- pick the install directory ---------------------------------------------
#
# Priority:
#   1. POLYAPI_INSTALL_DIR — caller is explicit; we never look at PATH.
#   2. Directory of an existing polyapi.exe / polyapi on PATH — replace in place.
#   3. %LOCALAPPDATA%\Programs\polyapi  (per-user, no admin). If that directory
#      is not on PATH we prepend it to the user PATH (see Ensure-OnPath).
#
if ($env:POLYAPI_INSTALL_DIR) {
    $destDir = $env:POLYAPI_INSTALL_DIR
} elseif ($existing = Get-Command polyapi.exe -ErrorAction SilentlyContinue) {
    $destDir = Split-Path -Parent $existing.Source
} elseif ($existing = Get-Command polyapi -ErrorAction SilentlyContinue) {
    $destDir = Split-Path -Parent $existing.Source
} else {
    $destDir = Join-Path $env:LOCALAPPDATA "Programs\polyapi"
}

New-Item -ItemType Directory -Force -Path $destDir | Out-Null
$dest = Join-Path $destDir "polyapi.exe"

# Temp dir holds the two downloads. The finally block deletes it on success
# or failure so a failed checksum never leaves a binary sitting around.
# New-TemporaryFile creates a *file*; we drop it and reuse the path as a dir.
$tmp = New-TemporaryFile | ForEach-Object { Remove-Item $_; New-Item -ItemType Directory -Path $_ }
try {
    Write-Host "downloading $asset ($tag)"
    $headers = @{ "User-Agent" = "polyapi-install" }

    # Invoke-WebRequest follows GitHub's 302 from the release asset URL to
    # objects.githubusercontent.com. The checksum still has to match.
    Invoke-WebRequest -Uri "$downloadBase/$tag/$asset" -OutFile (Join-Path $tmp $asset) -Headers $headers
    Invoke-WebRequest -Uri "$downloadBase/$tag/checksums.txt" -OutFile (Join-Path $tmp "checksums.txt") -Headers $headers

    # checksums.txt is `sha256sum` format: "<hex>  <filename>" (two spaces, or
    # " *" for binary mode). Match on the filename field only; take the hex.
    $expected = $null
    Get-Content (Join-Path $tmp "checksums.txt") | ForEach-Object {
        $parts = $_ -split '\s+'
        if ($parts.Length -ge 2) {
            $name = $parts[-1].TrimStart("*")
            if ($name -eq $asset) { $expected = $parts[0].ToLowerInvariant() }
        }
    }
    if (-not $expected) { throw "checksums.txt has no entry for $asset" }

    # Compare lowercase hex. Mismatch is fatal — we do not install "anyway".
    $actual = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $asset)).Hash.ToLowerInvariant()
    if ($expected -ne $actual) { throw "checksum mismatch for $asset" }

    # Copy-Item -Force replaces an existing polyapi.exe in destDir.
    Copy-Item -Force (Join-Path $tmp $asset) $dest
    Write-Host "installed $dest ($tag)"
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

# After the binary is in place: if destDir is not on PATH, prepend it to the
# user-scope Path (HKCU). Idempotent — a second install will not add a
# duplicate. POLYAPI_SKIP_PATH=1 skips this (tests). Machine PATH is never
# modified (that would need elevation and is outside this script's scope).
#
# Audit: run  [Environment]::GetEnvironmentVariable("Path", "User")
# and look for destDir. To undo, remove that entry from the user Path.
function Test-DirOnPath([string]$dir) {
    $n = $dir.TrimEnd("\")
    foreach ($p in $env:PATH.Split(";")) {
        if ($p -and ($p.TrimEnd("\") -eq $n)) { return $true }
    }
    return $false
}

function Ensure-OnPath([string]$dir) {
    if ($env:POLYAPI_SKIP_PATH -eq "1") { return }
    if (Test-DirOnPath $dir) {
        Write-Host "polyapi is on PATH ($dir)"
        return
    }

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if (-not $userPath) { $userPath = "" }
    $alreadyUser = $false
    foreach ($p in $userPath.Split(";")) {
        if ($p -and ($p.TrimEnd("\") -eq $dir.TrimEnd("\"))) { $alreadyUser = $true }
    }
    if (-not $alreadyUser) {
        Write-Host "PATH: adding $dir to the user PATH"
        $newPath = if ($userPath) { "$dir;$userPath" } else { $dir }
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    } else {
        Write-Host "PATH: $dir is already on the user PATH; this session was missing it"
    }
    # This PowerShell process (including `irm | iex`) sees the binary now.
    # Other already-open terminals will not until they are restarted.
    $env:PATH = "$dir;$env:PATH"
    Write-Host "PATH: this session updated; open a new terminal for other apps"
}

Ensure-OnPath $destDir
