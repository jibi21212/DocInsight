#requires -Version 5.1
<#
.SYNOPSIS
    DocInsight desktop — one-time (idempotent) setup.

.DESCRIPTION
    Provisions everything needed to build and run the Wails desktop app:
      1. Python embedding-sidecar virtualenv + requirements
      2. Go module dependencies + the Wails v2 CLI
      3. Frontend (Vite/React) dependencies

    Safe to re-run. After it finishes:
      wails dev      # hot-reload development
      wails build    # produce build\bin\docinsight.exe
#>
$ErrorActionPreference = "Stop"

$root = $PSScriptRoot
if (-not $root) { $root = Split-Path -Parent $MyInvocation.MyCommand.Path }
Write-Host "DocInsight setup" -ForegroundColor Cyan
Write-Host "Repo: $root`n"

function Find-Cmd([string[]]$names) {
    foreach ($n in $names) {
        $c = Get-Command $n -ErrorAction SilentlyContinue
        if ($c) { return $c.Source }
    }
    return $null
}

# ---------------------------------------------------------------------------
# 1. Python embedding sidecar (venv + requirements)
# ---------------------------------------------------------------------------
Write-Host "[1/3] Python embedding sidecar" -ForegroundColor Yellow
$py = Find-Cmd @('py', 'python')
if (-not $py) {
    throw "Python 3.11+ not found on PATH. Install from https://www.python.org/downloads/ (tick 'Add to PATH') and re-run."
}
$sidecar = Join-Path $root "backend\embedding-sidecar"
$venv    = Join-Path $sidecar ".venv"
$venvPy  = Join-Path $venv "Scripts\python.exe"

if (Test-Path $venvPy) {
    Write-Host "  venv exists: $venv"
} else {
    Write-Host "  creating venv: $venv"
    & $py -m venv $venv
    if ($LASTEXITCODE -ne 0) { throw "failed to create venv" }
}
Write-Host "  installing requirements (first run downloads the model deps; can take a few minutes)..."
& $venvPy -m pip install --upgrade pip --quiet
& $venvPy -m pip install -r (Join-Path $sidecar "requirements.txt")
if ($LASTEXITCODE -ne 0) { throw "pip install failed" }
Write-Host "  Python sidecar ready.`n" -ForegroundColor Green

# ---------------------------------------------------------------------------
# 2. Go dependencies + Wails CLI
# ---------------------------------------------------------------------------
Write-Host "[2/3] Go backend + Wails CLI" -ForegroundColor Yellow
$go = Find-Cmd @('go')
if (-not $go) {
    $cand = "C:\Program Files\Go\bin\go.exe"
    if (Test-Path $cand) { $go = $cand }
    else { throw "Go 1.23+ not found. Install from https://go.dev/dl/ and re-run." }
}
Push-Location $root
try {
    & $go mod download
    if ($LASTEXITCODE -ne 0) { throw "go mod download failed" }
} finally { Pop-Location }

$goBin = (& $go env GOPATH).Trim()
$wails = Join-Path $goBin "bin\wails.exe"
if (Test-Path $wails) {
    Write-Host "  Wails CLI present: $wails"
} else {
    Write-Host "  installing Wails CLI (v2.12.0)..."
    & $go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
    if ($LASTEXITCODE -ne 0) { throw "wails CLI install failed" }
}
Write-Host "  Go deps + Wails CLI ready.`n" -ForegroundColor Green

# ---------------------------------------------------------------------------
# 3. Frontend dependencies
# ---------------------------------------------------------------------------
Write-Host "[3/3] Frontend" -ForegroundColor Yellow
$npm = Find-Cmd @('npm', 'npm.cmd')
if (-not $npm) {
    throw "Node.js 20+/npm not found. Install from https://nodejs.org/ and re-run."
}
Push-Location (Join-Path $root "frontend")
try {
    & $npm install
    if ($LASTEXITCODE -ne 0) { throw "npm install failed" }
} finally { Pop-Location }
Write-Host "  Frontend deps ready.`n" -ForegroundColor Green

# ---------------------------------------------------------------------------
Write-Host "Setup complete." -ForegroundColor Cyan
Write-Host "  Develop : `"$wails`" dev"
Write-Host "  Build   : `"$wails`" build    ->  build\bin\docinsight.exe"
Write-Host "(If 'wails' isn't on your PATH, add '$goBin\bin' to it, or use the full path above.)"
