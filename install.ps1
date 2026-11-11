# cadenza installer for Windows (PowerShell).
#
#   irm https://raw.githubusercontent.com/mutex-solutions/cadenza/main/install.ps1 | iex
#
# Env overrides:
#   $env:CADENZA_VERSION = "v0.1.0"   install a specific tag (default: latest)
#   $env:CADENZA_INSTALL_DIR = "..."  where to put the binary

$ErrorActionPreference = "Stop"
$Repo = "mutex-solutions/cadenza"

# --- Detect arch ---
$arch = if ([Environment]::Is64BitOperatingSystem) {
  if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
} else { throw "unsupported architecture" }

# --- Resolve version ---
$version = $env:CADENZA_VERSION
if (-not $version) {
  Write-Host "  Finding the latest release..."
  $rel = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ "User-Agent" = "cadenza-installer" }
  $version = $rel.tag_name
}
if (-not $version) { throw "could not determine latest version" }

$archive = "cadenza_windows_$arch.zip"
$url = "https://github.com/$Repo/releases/download/$version/$archive"

# --- Install dir ---
$installDir = $env:CADENZA_INSTALL_DIR
if (-not $installDir) { $installDir = Join-Path $env:LOCALAPPDATA "Programs\cadenza" }
New-Item -ItemType Directory -Force -Path $installDir | Out-Null

# --- Download + extract ---
$tmp = New-Item -ItemType Directory -Force -Path (Join-Path $env:TEMP ("cadenza-" + [guid]::NewGuid()))
try {
  Write-Host "  Downloading cadenza $version (windows/$arch)..."
  $zip = Join-Path $tmp $archive
  Invoke-WebRequest -Uri $url -OutFile $zip -Headers @{ "User-Agent" = "cadenza-installer" }
  Expand-Archive -Path $zip -DestinationPath $tmp -Force
  Copy-Item -Path (Join-Path $tmp "cadenza.exe") -Destination (Join-Path $installDir "cadenza.exe") -Force
  Write-Host "  Installed to $installDir\cadenza.exe"

  # --- PATH hint ---
  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  if ($userPath -notlike "*$installDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$installDir", "User")
    Write-Host "  Added $installDir to your user PATH (restart your shell)."
  }
  Write-Host ""
  Write-Host '  Next: run "cadenza login <server-url>" to pair this machine.'
} finally {
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
