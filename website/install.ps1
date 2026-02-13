# ─────────────────────────────────────────────────────────────────────────────
# TuTu Engine Installer — Windows PowerShell
# Enterprise-grade installer with multi-source version detection, retry,
# verification, and intelligent fallback mechanisms.
#
# Usage: irm tutuengine.tech/install.ps1 | iex
#
# Environment variables:
#   $env:TUTU_VERSION      Override version (e.g., "v0.9.4")
#   $env:TUTU_INSTALL_DIR  Override install directory
#   $env:TUTU_HOME         Override TuTu home directory
# ─────────────────────────────────────────────────────────────────────────────
$ErrorActionPreference = "Continue"

$repo = "NikeGunn/tutu"
$binary = "tutu.exe"
$maxRetries = 3
$retryDelay = 2

# ─── Banner ─────────────────────────────────────────────────────────────────
Write-Host ""
Write-Host "  ████████╗██╗   ██╗████████╗██╗   ██╗" -ForegroundColor Magenta
Write-Host "  ╚══██╔══╝██║   ██║╚══██╔══╝██║   ██║" -ForegroundColor Magenta
Write-Host "     ██║   ██║   ██║   ██║   ██║   ██║" -ForegroundColor Magenta
Write-Host "     ██║   ╚██████╔╝   ██║   ╚██████╔╝" -ForegroundColor Magenta
Write-Host "     ╚═╝    ╚═════╝    ╚═╝    ╚═════╝" -ForegroundColor Magenta
Write-Host ""
Write-Host "  The Local-First AI Runtime" -ForegroundColor White
Write-Host ""
Write-Host "  Installing TuTu Engine for Windows..." -ForegroundColor Cyan
Write-Host ""

# ─── Platform Detection ─────────────────────────────────────────────────────
$arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }
if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64" -or $env:PROCESSOR_IDENTIFIER -match "ARM") {
    $arch = "arm64"
}
Write-Host "  Platform: windows/$arch" -ForegroundColor Gray

# ─── Install Directory ──────────────────────────────────────────────────────
$installDir = if ($env:TUTU_INSTALL_DIR) { $env:TUTU_INSTALL_DIR } else { "$env:LOCALAPPDATA\TuTu\bin" }
if (-not (Test-Path $installDir)) {
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null
}

# ─── TuTu Home ──────────────────────────────────────────────────────────────
$tutuHome = if ($env:TUTU_HOME) { $env:TUTU_HOME } else { "$env:USERPROFILE\.tutu" }
foreach ($subDir in @("bin", "models", "keys")) {
    $sub = Join-Path $tutuHome $subDir
    if (-not (Test-Path $sub)) { New-Item -ItemType Directory -Path $sub -Force | Out-Null }
}

# ─── Version Resolution with Multi-Source Fallback ───────────────────────────
$version = $env:TUTU_VERSION
if (-not $version) {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12 -bor [Net.SecurityProtocolType]::Tls13

    # Strategy 1: GitHub API (preferred — most reliable)
    for ($attempt = 1; $attempt -le $maxRetries; $attempt++) {
        try {
            $release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest" `
                -Headers @{"User-Agent" = "TuTu-Installer/3.0"; "Accept" = "application/vnd.github+json" } `
                -TimeoutSec 15 -ErrorAction Stop
            $version = $release.tag_name
            if ($version) {
                Write-Host "  Latest version: $version" -ForegroundColor Green
                break
            }
        }
        catch {
            if ($attempt -lt $maxRetries) {
                $delay = $retryDelay * $attempt
                Write-Host "  API attempt $attempt/$maxRetries failed. Retrying in ${delay}s..." -ForegroundColor Yellow
                Start-Sleep -Seconds $delay
            }
        }
    }

    # Strategy 2: GitHub redirect header (no API quota needed)
    if (-not $version) {
        try {
            Write-Host "  Trying redirect detection..." -ForegroundColor Gray
            $req = [System.Net.HttpWebRequest]::Create("https://github.com/$repo/releases/latest")
            $req.UserAgent = "TuTu-Installer/3.0"
            $req.AllowAutoRedirect = $false
            $req.Timeout = 15000
            $resp = $req.GetResponse()
            $location = $resp.Headers["Location"]
            $resp.Close()
            if ($location -match '/tag/(v[0-9]+\.[0-9]+\.[0-9]+.*)') {
                $version = $Matches[1]
                Write-Host "  Latest version (redirect): $version" -ForegroundColor Green
            }
        }
        catch {
            # Silently continue to next strategy
        }
    }

    # Strategy 3: Scrape releases page as last resort
    if (-not $version) {
        try {
            Write-Host "  Trying releases page..." -ForegroundColor Gray
            $releasePage = Invoke-WebRequest -Uri "https://github.com/$repo/releases" `
                -UseBasicParsing -TimeoutSec 15 `
                -Headers @{"User-Agent" = "TuTu-Installer/3.0" } -ErrorAction Stop
            $pageContent = $releasePage.Content
            if ($pageContent -match '/releases/tag/(v[0-9]+\.[0-9]+\.[0-9]+[^"]*)') {
                $version = $Matches[1]
                Write-Host "  Latest version (page): $version" -ForegroundColor Green
            }
        }
        catch {}
    }

    # Fatal: Cannot determine version
    if (-not $version) {
        Write-Host ""
        Write-Host "  ERROR: Could not detect latest version." -ForegroundColor Red
        Write-Host "  Possible causes:" -ForegroundColor Yellow
        Write-Host "    - No internet connection" -ForegroundColor Yellow
        Write-Host "    - GitHub API rate limited" -ForegroundColor Yellow
        Write-Host ""
        Write-Host "  Fix: Set version manually:" -ForegroundColor Cyan
        Write-Host "    `$env:TUTU_VERSION='v0.9.4'; irm tutuengine.tech/install.ps1 | iex" -ForegroundColor White
        Write-Host ""
        exit 1
    }
}
else {
    Write-Host "  Using specified version: $version" -ForegroundColor Green
}

# ─── Check Existing Installation ────────────────────────────────────────────
$existingPath = Get-Command tutu -ErrorAction SilentlyContinue
if ($existingPath) {
    try {
        $existingVersion = & $existingPath.Source --version 2>$null
        Write-Host "  Existing: $existingVersion" -ForegroundColor Gray
    }
    catch {}
}

# ─── Download with Retry ────────────────────────────────────────────────────
$url = "https://github.com/$repo/releases/download/$version/tutu-windows-$arch.exe"
$tmpFile = Join-Path $env:TEMP "tutu-download-$([guid]::NewGuid().ToString('N').Substring(0,8)).exe"

Write-Host ""
Write-Host "  Downloading TuTu $version..." -ForegroundColor Cyan
Write-Host "  $url" -ForegroundColor Gray

$downloadSuccess = $false
for ($attempt = 1; $attempt -le $maxRetries; $attempt++) {
    try {
        [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12 -bor [Net.SecurityProtocolType]::Tls13
        $webClient = New-Object System.Net.WebClient
        $webClient.Headers.Add("User-Agent", "TuTu-Installer/3.0")
        $webClient.DownloadFile($url, $tmpFile)

        if ((Test-Path $tmpFile) -and ((Get-Item $tmpFile).Length -gt 1048576)) {
            $downloadSuccess = $true
            break
        }
    }
    catch {
        try {
            Invoke-WebRequest -Uri $url -OutFile $tmpFile -UseBasicParsing -TimeoutSec 120 `
                -Headers @{"User-Agent" = "TuTu-Installer/3.0" } -ErrorAction Stop
            if ((Test-Path $tmpFile) -and ((Get-Item $tmpFile).Length -gt 1048576)) {
                $downloadSuccess = $true
                break
            }
        }
        catch {}
    }

    if (-not $downloadSuccess -and $attempt -lt $maxRetries) {
        $delay = $retryDelay * $attempt
        Write-Host "  Attempt $attempt/$maxRetries failed. Retrying in ${delay}s..." -ForegroundColor Yellow
        Start-Sleep -Seconds $delay
    }
}

if (-not $downloadSuccess) {
    # Clean up partial file
    if (Test-Path $tmpFile) { Remove-Item $tmpFile -Force -ErrorAction SilentlyContinue }
    Write-Host ""
    Write-Host "  Download failed after $maxRetries attempts." -ForegroundColor Red
    Write-Host ""
    Write-Host "  Manual download:" -ForegroundColor Cyan
    Write-Host "    $url" -ForegroundColor White
    Write-Host ""
    Write-Host "  Or build from source (requires Go 1.24+):" -ForegroundColor Cyan
    Write-Host "    git clone https://github.com/$repo.git" -ForegroundColor White
    Write-Host "    cd tutu ; go build -o tutu.exe .\cmd\tutu" -ForegroundColor White
    Write-Host ""
    exit 1
}

# ─── Multi-Layer Verification ───────────────────────────────────────────────
Write-Host "  Verifying download..." -ForegroundColor Cyan

# Layer 1: HTML error page check
$firstBytes = [System.IO.File]::ReadAllBytes($tmpFile) | Select-Object -First 100
$firstText = [System.Text.Encoding]::ASCII.GetString($firstBytes)
if ($firstText -match "<!DOCTYPE|<html|Not Found") {
    Remove-Item $tmpFile -Force -ErrorAction SilentlyContinue
    Write-Host "  Download returned HTML instead of binary (404)." -ForegroundColor Red
    Write-Host "  Check releases: https://github.com/$repo/releases" -ForegroundColor Yellow
    exit 1
}

# Layer 2: File size check (binary should be > 1MB)
$fileSize = (Get-Item $tmpFile).Length
if ($fileSize -lt 1048576) {
    Remove-Item $tmpFile -Force -ErrorAction SilentlyContinue
    Write-Host "  Downloaded file too small ($fileSize bytes). Expected > 1MB." -ForegroundColor Red
    exit 1
}
$sizeMB = [math]::Round($fileSize / 1048576, 1)
Write-Host "  Size: ${sizeMB} MB" -ForegroundColor Green

# Layer 3: PE header check (Windows executables start with "MZ")
$peHeader = [System.Text.Encoding]::ASCII.GetString($firstBytes[0..1])
if ($peHeader -ne "MZ") {
    Write-Host "  Warning: File may not be a valid Windows executable." -ForegroundColor Yellow
}

# Layer 4: Execution test (non-fatal)
try {
    $testOutput = & $tmpFile --version 2>$null
    if ($testOutput) {
        Write-Host "  Execution test: passed ($testOutput)" -ForegroundColor Green
    }
}
catch {
    Write-Host "  Execution test: skipped (may work after install)" -ForegroundColor Yellow
}

# ─── Install ─────────────────────────────────────────────────────────────────
$dest = Join-Path $installDir $binary
Move-Item -Path $tmpFile -Destination $dest -Force
Write-Host "  Installed to $dest" -ForegroundColor Green

# ─── PATH Management ────────────────────────────────────────────────────────
$currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($currentPath -notlike "*$installDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$currentPath;$installDir", "User")
    $env:Path = "$env:Path;$installDir"
    Write-Host "  Added $installDir to PATH" -ForegroundColor Green
}

# ─── Final Verification ─────────────────────────────────────────────────────
$verified = $false
for ($wait = 0; $wait -lt 5; $wait++) {
    if (Test-Path $dest) {
        try {
            $installedVersion = & $dest --version 2>$null
            if ($installedVersion) {
                $verified = $true
                break
            }
        }
        catch {}
    }
    Start-Sleep -Seconds 1
}

Write-Host ""
if ($verified) {
    Write-Host "  ═══════════════════════════════════════════" -ForegroundColor Green
    Write-Host "    TuTu Engine installed successfully!" -ForegroundColor Green
    Write-Host "    Version: $installedVersion" -ForegroundColor Green
    Write-Host "  ═══════════════════════════════════════════" -ForegroundColor Green
}
else {
    Write-Host "  TuTu Engine installed to $dest" -ForegroundColor Green
}
Write-Host ""
Write-Host "  Get started:" -ForegroundColor Cyan
Write-Host "    tutu run llama3.2        # Chat with Llama 3.2"
Write-Host "    tutu run phi3            # Chat with Phi-3"
Write-Host "    tutu run qwen2.5         # Chat with Qwen 2.5"
Write-Host "    tutu serve               # Start API server"
Write-Host "    tutu --help              # See all commands"
Write-Host ""
Write-Host "  API endpoints:" -ForegroundColor Cyan
Write-Host "    Ollama:   http://localhost:11434/api/chat"
Write-Host "    OpenAI:   http://localhost:11434/v1/chat/completions"
Write-Host "    MCP:      http://localhost:11434/mcp"
Write-Host ""
Write-Host "  Docs: https://tutuengine.tech/docs.html" -ForegroundColor Gray
Write-Host ""
Write-Host "  Restart your terminal for PATH changes to take effect." -ForegroundColor Yellow
