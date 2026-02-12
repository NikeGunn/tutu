# TuTu Installer — Windows PowerShell
# Usage: irm tutuengine.tech/install.ps1 | iex
$ErrorActionPreference = "Stop"

$repo = "NikeGunn/tutu"
$binary = "tutu.exe"

Write-Host ""
Write-Host "  ████████╗██╗   ██╗████████╗██╗   ██╗" -ForegroundColor Magenta
Write-Host "  ╚══██╔══╝██║   ██║╚══██╔══╝██║   ██║" -ForegroundColor Magenta
Write-Host "     ██║   ██║   ██║   ██║   ██║   ██║" -ForegroundColor Magenta
Write-Host "     ██║   ╚██████╔╝   ██║   ╚██████╔╝" -ForegroundColor Magenta
Write-Host "     ╚═╝    ╚═════╝    ╚═╝    ╚═════╝" -ForegroundColor Magenta
Write-Host ""
Write-Host "  Installing TuTu for Windows..." -ForegroundColor Cyan
Write-Host ""

# Determine architecture
$arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }

# Install directory
$installDir = "$env:LOCALAPPDATA\TuTu\bin"
if (-not (Test-Path $installDir)) {
  New-Item -ItemType Directory -Path $installDir -Force | Out-Null
}

# Get latest release
$version = $null
try {
  # Use TLS 1.2+ for GitHub API
  [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12 -bor [Net.SecurityProtocolType]::Tls13
  $release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest" -ErrorAction Stop
  $version = $release.tag_name
  Write-Host "  Latest version: $version" -ForegroundColor Green
}
catch {
  $version = "v0.1.0"
  Write-Host "  Using default version: $version" -ForegroundColor Yellow
}

$url = "https://github.com/$repo/releases/download/$version/tutu-windows-$arch.exe"

# Download with proper TLS and retry
$tmpFile = Join-Path $env:TEMP "tutu-download.exe"
Write-Host "  Downloading $url..."
$downloadSuccess = $false
try {
  [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12 -bor [Net.SecurityProtocolType]::Tls13
  $webClient = New-Object System.Net.WebClient
  $webClient.DownloadFile($url, $tmpFile)
  $downloadSuccess = $true
}
catch {
  try {
    # Fallback: try Invoke-WebRequest with explicit TLS
    Invoke-WebRequest -Uri $url -OutFile $tmpFile -UseBasicParsing -TimeoutSec 60
    $downloadSuccess = $true
  }
  catch {
    $downloadSuccess = $false
  }
}

if (-not $downloadSuccess) {
  Write-Host ""
  Write-Host "  ⚠  Pre-built binary not available for Windows/$arch ($version)." -ForegroundColor Yellow
  Write-Host ""
  Write-Host "  Build from source instead (requires Go 1.24+):" -ForegroundColor Cyan
  Write-Host ""
  Write-Host "    git clone https://github.com/$repo.git"
  Write-Host "    cd tutuengine\tutu"
  Write-Host "    go build -o tutu.exe .\cmd\tutu"
  Write-Host "    Move-Item tutu.exe $installDir\$binary"
  Write-Host ""
  Write-Host "  Or check releases: https://github.com/$repo/releases" -ForegroundColor Gray
  Write-Host ""
  exit 1
}

# Validate — make sure we didn't download an HTML error page
$firstBytes = [System.IO.File]::ReadAllBytes($tmpFile) | Select-Object -First 100
$firstText = [System.Text.Encoding]::ASCII.GetString($firstBytes)
if ($firstText -match "<!DOCTYPE|<html|Not Found") {
  Remove-Item $tmpFile -Force -ErrorAction SilentlyContinue
  Write-Host ""
  Write-Host "  ⚠  Download failed — received HTML instead of binary." -ForegroundColor Yellow
  Write-Host "     The release $version may not exist for Windows/$arch." -ForegroundColor Yellow
  Write-Host ""
  Write-Host "  Build from source instead (requires Go 1.24+):" -ForegroundColor Cyan
  Write-Host ""
  Write-Host "    git clone https://github.com/$repo.git"
  Write-Host "    cd tutuengine\tutu"
  Write-Host "    go build -o tutu.exe .\cmd\tutu"
  Write-Host "    Move-Item tutu.exe $installDir\$binary"
  Write-Host ""
  exit 1
}

# Install
$dest = Join-Path $installDir $binary
Move-Item -Path $tmpFile -Destination $dest -Force

# Add to PATH if not already there
$currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($currentPath -notlike "*$installDir*") {
  [Environment]::SetEnvironmentVariable("Path", "$currentPath;$installDir", "User")
  $env:Path = "$env:Path;$installDir"
  Write-Host "  Added $installDir to PATH" -ForegroundColor Green
}

# Verify
try {
  $installedVersion = & $dest --version 2>$null
  Write-Host ""
  Write-Host "  ✅ TuTu installed successfully! ($installedVersion)" -ForegroundColor Green
}
catch {
  Write-Host ""
  Write-Host "  ✅ TuTu installed to $dest" -ForegroundColor Green
}
Write-Host ""
Write-Host "  Get started:" -ForegroundColor Cyan
Write-Host "    tutu run llama3.2        # Chat with Llama 3.2"
Write-Host "    tutu run phi3            # Chat with Phi-3"
Write-Host "    tutu serve               # Start API server"
Write-Host "    tutu --help              # See all commands"
Write-Host ""
Write-Host "  Documentation: https://tutuengine.tech/docs.html" -ForegroundColor Gray
Write-Host ""
Write-Host "  ⚠  Restart your terminal for PATH changes to take effect." -ForegroundColor Yellow
