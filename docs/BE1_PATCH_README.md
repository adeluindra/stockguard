# StockGuard BE-1 — Backend foundation patch

This archive contains four complete source files only. It was prepared after reading the GitHub `main` tree. It **does not** change `.env`, `go.mod`, `go.sum`, SQL migrations, PostgreSQL data, or Docker configuration.

## Apply locally (Windows PowerShell)

From the repository root:

```powershell
# Optional: safeguard local edits in the three Go files before extraction.
# Download ZIP to your browser's Downloads folder first.
Expand-Archive -LiteralPath "$HOME\Downloads\stockguard-be1-from-github.zip" -DestinationPath . -Force
cd apps/api
# Go module and local Go version already exist; pin the driver previously selected.
go get github.com/jackc/pgx/v5/stdlib@v5.11.0
go mod tidy
go fmt ./...
go vet ./...
go test ./... -v
cd ../..
.\scripts\start-api.ps1
```

In a second PowerShell terminal:

```powershell
Invoke-RestMethod http://127.0.0.1:8080/healthz
Invoke-RestMethod http://127.0.0.1:8080/readyz
```

Expected `/healthz`: `data.status = ok`. Expected `/readyz`: `data.status = ok`, `data.database = ok` after migration v4 is applied.

Commit from the repository root only after local tests pass:

```powershell
git status --short
git add apps/api/cmd/server/main.go apps/api/internal/httpapi/router.go apps/api/internal/httpapi/router_test.go apps/api/go.mod apps/api/go.sum scripts/start-api.ps1
git commit -m "feat(api): implement backend foundation and readiness checks"
git push origin main
```

Do not stage `.env` or other secrets. Do not run `docker compose down -v`.

## Next checkpoint

BE-2 Authentication, *after* startup + local DB connectivity verification. The repository's README and docs/PROJECT_STATUS.md still describe the old DB-2 state and should be updated as a separate documentation task.
