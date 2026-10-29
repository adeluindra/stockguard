# BE-2A — Password Security

This patch adds `apps/api/internal/auth/password.go` and unit tests. It **does not** implement login, sessions, account provisioning, or password resets.

## Design

- Argon2id, memory=19 MiB, iterations=2, parallelism=1 (OWASP baseline).
- 16-byte cryptographically random salt for every password; 32-byte derived key.
- PHC-style encoding: `$argon2id$v=19$m=19456,t=2,p=1$<salt-base64>$<hash-base64>`.
- Constant-time comparison of derived keys.
- Only the fixed bounded parameters are accepted on verification, preventing unbounded memory/CPU cost on malformed stored hashes.
- `ErrInvalidHash` is internal and must not be exposed to clients through login responses.
- `maxPasswordBytes=1024` is a computational resource bound, **not** the user-facing password strength policy. Account creation/reset must choose and enforce a product policy later.
- Changing Argon2id parameters later requires backward-compatible verification and an account rehash migration strategy; this initial version intentionally accepts one format.

## Apply from repository root on Windows (NOT from apps/api)

```powershell
cd C:\Users\adelu\OneDrive\Desktop\stockguard
if (-not (Test-Path .\docker-compose.yml)) { throw 'Run from StockGuard root.' }
Expand-Archive -LiteralPath "$HOME\Downloads\stockguard-be2a-auth.zip" -DestinationPath . -Force
cd apps\api
go get golang.org/x/crypto/argon2@v0.58.0
go mod tidy
go fmt ./...
go vet ./...
go test ./... -v
```

Test against the locally installed Go toolchain (user previously reported Go 1.27.2). Do not run the Go test commands from the repository root.

Source: https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html
