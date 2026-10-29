# Usage from the StockGuard repo root: .\scripts\start-api.ps1
$ErrorActionPreference = 'Stop'
$root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path

Push-Location $root
try {
    if (-not (Test-Path '.env')) {
        throw 'Missing .env at repository root.'
    }

    $config = @{}
    foreach ($line in Get-Content '.env') {
        if ($line -match '^\s*(POSTGRES_(?:USER|PASSWORD|DB|PORT))=(.*)$') {
            $config[$matches[1]] = $matches[2].Trim().Trim('"').Trim("'")
        }
    }

    foreach ($key in @('POSTGRES_USER', 'POSTGRES_PASSWORD', 'POSTGRES_DB', 'POSTGRES_PORT')) {
        if ([string]::IsNullOrWhiteSpace($config[$key])) {
            throw "Missing or empty $key in .env."
        }
    }

    if ($config['POSTGRES_PASSWORD'] -eq 'CHANGE_ME_TO_A_UNIQUE_LOCAL_PASSWORD') {
        throw 'Replace the placeholder POSTGRES_PASSWORD in .env.'
    }

    if ($config['POSTGRES_PORT'] -notmatch '^\d{1,5}$' -or
        [int]$config['POSTGRES_PORT'] -lt 1 -or
        [int]$config['POSTGRES_PORT'] -gt 65535) {
        throw 'POSTGRES_PORT must be a valid TCP port.'
    }

    Write-Host 'Starting and checking PostgreSQL...'
    docker compose up -d --wait db
    if ($LASTEXITCODE -ne 0) {
        throw 'PostgreSQL failed to become healthy. Check: docker compose ps and docker compose logs db'
    }

    # Encode credentials so special characters cannot corrupt the URI.
    $user = [uri]::EscapeDataString($config['POSTGRES_USER'])
    $password = [uri]::EscapeDataString($config['POSTGRES_PASSWORD'])
    $database = [uri]::EscapeDataString($config['POSTGRES_DB'])
    $port = $config['POSTGRES_PORT']

    $env:DATABASE_URL = 'postgres://{0}:{1}@127.0.0.1:{2}/{3}?sslmode=disable' -f `
        $user, $password, $port, $database
    $env:HTTP_ADDR = '127.0.0.1:8080'

    Push-Location (Join-Path $root 'apps/api')
    try {
        Write-Host 'Starting StockGuard Go API on http://127.0.0.1:8080 ...'
        go run ./cmd/server
        if ($LASTEXITCODE -ne 0) {
            throw 'Go API exited with an error.'
        }
    }
    finally {
        Pop-Location
    }
}
finally {
    Remove-Item Env:DATABASE_URL -ErrorAction SilentlyContinue
    Remove-Item Env:HTTP_ADDR -ErrorAction SilentlyContinue
    Pop-Location
}
