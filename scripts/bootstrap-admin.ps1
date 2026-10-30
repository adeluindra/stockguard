
$ErrorActionPreference = "Stop"

$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path

Push-Location $root

try {
    if (-not (Test-Path ".env")) {
        throw "File .env tidak ditemukan di root StockGuard."
    }

    $config = @{}

    foreach ($line in Get-Content ".env") {
        if ($line -match '^\s*(POSTGRES_(?:USER|PASSWORD|DB|PORT))=(.*)$') {
            $config[$matches[1]] = $matches[2].Trim().Trim('"').Trim("'")
        }
    }

    foreach ($key in @(
        "POSTGRES_USER",
        "POSTGRES_PASSWORD",
        "POSTGRES_DB",
        "POSTGRES_PORT"
    )) {
        if ([string]::IsNullOrWhiteSpace($config[$key])) {
            throw "Konfigurasi $key belum lengkap."
        }
    }

    if ($config["POSTGRES_DB"] -ne "stockguard_dev") {
        throw "Bootstrap lokal hanya diizinkan pada stockguard_dev."
    }

    if ($config["POSTGRES_PASSWORD"] -eq "CHANGE_ME_TO_A_UNIQUE_LOCAL_PASSWORD") {
        throw "Ganti password placeholder pada .env."
    }

    Write-Host "Starting StockGuard PostgreSQL..."

    docker compose up -d --wait db

    if ($LASTEXITCODE -ne 0) {
        throw "PostgreSQL gagal dijalankan."
    }

    $username = [uri]::EscapeDataString($config["POSTGRES_USER"])
    $password = [uri]::EscapeDataString($config["POSTGRES_PASSWORD"])
    $database = [uri]::EscapeDataString($config["POSTGRES_DB"])
    $port = $config["POSTGRES_PORT"]

    $env:DATABASE_URL = (
        "postgres://{0}:{1}@127.0.0.1:{2}/{3}?sslmode=disable" -f
        $username, $password, $port, $database
    )

    Push-Location (Join-Path $root "apps/api")

    try {
        Write-Host "Running initial admin bootstrap..."

        go run ./cmd/bootstrap-admin -username admin

        if ($LASTEXITCODE -ne 0) {
            throw "Bootstrap command gagal."
        }
    }
    finally {
        Pop-Location
    }
}
finally {
    Remove-Item Env:DATABASE_URL -ErrorAction SilentlyContinue

    Remove-Variable password -ErrorAction SilentlyContinue

    Pop-Location
}
