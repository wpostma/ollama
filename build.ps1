# abuild.ps1 -- Quick dev build: React SPA + ollama.exe + ollama-app.exe
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File .\abuild.ps1
# Skips GPU backends, signing, and installer.

$ErrorActionPreference = "Stop"

$SRC_DIR = $PSScriptRoot

# MinGW GCC required for CGO (WebView2, SQLite)
$mingw = "C:\msys64\mingw64\bin"
if (Test-Path $mingw) {
    $env:PATH = "$mingw;$env:PATH"
} else {
    Write-Warning "MinGW not found at $mingw -- CGO may fail"
}

$env:CGO_ENABLED = "1"

# Detect version from git
$gitDesc = git describe --tags --first-parent --abbrev=7 --long --dirty --always 2>$null
if ($gitDesc -match "v(.+)") {
    $VERSION = $matches[1]
} else {
    $VERSION = $gitDesc
}
if (-not $VERSION) { $VERSION = "0.0.0-dev" }
Write-Host "Version: $VERSION"

# -----------------------------------------------------------------------
# 1. React SPA
# -----------------------------------------------------------------------
Write-Host ""
Write-Host "=== Building React SPA ==="
Push-Location (Join-Path $SRC_DIR "app\ui\app")
try {
    & npm install
    if ($LASTEXITCODE -ne 0) { throw "npm install failed ($LASTEXITCODE)" }
    & npm run build
    if ($LASTEXITCODE -ne 0) { throw "npm run build failed ($LASTEXITCODE)" }
    if (!(Test-Path "dist") -or (Get-ChildItem "dist" -Recurse).Count -eq 0) {
        throw "dist/ is missing or empty after npm run build"
    }
    Write-Host "SPA dist/ OK"
} finally {
    Pop-Location
}

# -----------------------------------------------------------------------
# 2. ollama.exe (CLI)
# -----------------------------------------------------------------------
Write-Host ""
Write-Host "=== Building ollama.exe ==="
Push-Location $SRC_DIR
& go build -trimpath `
    -ldflags "-s -w -X=github.com/ollama/ollama/version.Version=$VERSION -X=github.com/ollama/ollama/server.mode=release" `
    -o ollama.exe .
if ($LASTEXITCODE -ne 0) { throw "go build ollama.exe failed ($LASTEXITCODE)" }
Write-Host "ollama.exe OK"
Pop-Location

# -----------------------------------------------------------------------
# 3. ollama-app.exe (headless-capable app, embeds SPA)
# -----------------------------------------------------------------------
Write-Host ""
Write-Host "=== Building ollama-app.exe ==="
Push-Location $SRC_DIR
& go build -trimpath `
    -ldflags "-s -w -X=github.com/ollama/ollama/app/version.Version=$VERSION" `
    -o ollama-app.exe ./app/cmd/app
if ($LASTEXITCODE -ne 0) { throw "go build ollama-app.exe failed ($LASTEXITCODE)" }
Write-Host "ollama-app.exe OK"
Pop-Location

Write-Host ""
Write-Host "=== Build complete ==="
Write-Host "  ollama.exe      -- CLI"
Write-Host "  ollama-app.exe  -- App (run with --headless --port=3001)"
