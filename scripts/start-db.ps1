# From repository root: .\scripts\start-db.ps1
$ErrorActionPreference = 'Stop'

if (-not (Test-Path '.\docker-compose.yml')) {
    throw 'Run this script from the StockGuard repository root.'
}
if (-not (Test-Path '.\.env')) {
    throw 'Missing .env. Run: Copy-Item .env.example .env; then configure a local password.'
}
if (Select-String -Path '.\.env' -Pattern 'CHANGE_ME_TO_A_UNIQUE_LOCAL_PASSWORD' -Quiet) {
    throw 'Replace the placeholder POSTGRES_PASSWORD in .env before startup.'
}

docker info --format '{{.ServerVersion}}' | Out-Null
if ($LASTEXITCODE -ne 0) {
    throw 'Docker engine is unavailable. Start Docker Desktop first.'
}
docker compose config --quiet
if ($LASTEXITCODE -ne 0) { throw 'Invalid docker-compose.yml or .env.' }
docker compose up -d db
if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL startup failed. Check docker compose logs db.' }
docker compose ps
if ($LASTEXITCODE -ne 0) { throw 'Failed to display container status.' }
Write-Host 'Database startup requested. Verify STATUS=healthy before using it.'
