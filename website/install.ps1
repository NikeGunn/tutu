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
try {
  $release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest" -ErrorAction Stop
  $version = $release.tag_name
  Write-Host "  Latest version: $version" -ForegroundColor Green
}
catch {
  $version = "v0.1.0"
  Write-Host "  Using default version: $version" -ForegroundColor Yellow
}

$url = "https://github.com/$repo/releases/download/$version/tutu-windows-$arch.exe"

# Download
$tmpFile = Join-Path $env:TEMP "tutu-download.exe"
Write-Host "  Downloading $url..."
Invoke-WebRequest -Uri $url -OutFile $tmpFile -UseBasicParsing

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
$installedVersion = & $dest --version 2>$null
Write-Host ""
Write-Host "  ✅ TuTu installed successfully! ($installedVersion)" -ForegroundColor Green
Write-Host ""
Write-Host "  Get started:" -ForegroundColor Cyan
Write-Host "    tutu run llama3.2        # Chat with Llama 3.2"
Write-Host "    tutu run phi3            # Chat with Phi-3"
Write-Host "    tutu serve               # Start API server"
Write-Host "    tutu --help              # See all commands"
Write-Host ""
Write-Host "  Documentation: https://tutuengine.tech/docs" -ForegroundColor Gray
Write-Host ""
Write-Host "  ⚠  Restart your terminal for PATH changes to take effect." -ForegroundColor Yellow
