
$ErrorActionPreference = "Stop"

$baseUrl = "http://127.0.0.1:8080"
$username = "admin"

$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession

Write-Host "StockGuard Real Authentication Test"
Write-Host "Checking API readiness..."

$ready = Invoke-RestMethod `
    -Uri "$baseUrl/readyz" `
    -Method Get

if ($ready.data.status -ne "ok" -or
    $ready.data.database -ne "ok") {
    throw "API atau database belum siap."
}

Write-Host "PASS: API and database ready"

$securePassword = Read-Host "Admin password" -AsSecureString

$pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR(
    $securePassword
)

try {
    $plainPassword = [Runtime.InteropServices.Marshal]::PtrToStringBSTR(
        $pointer
    )
}
finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer)
}

try {
    # Step 1: Obtain login CSRF token and matching cookie.
    $loginCSRF = Invoke-RestMethod `
        -Uri "$baseUrl/api/v1/auth/login-csrf" `
        -Method Get `
        -WebSession $session

    if (-not $loginCSRF.data.csrf_token) {
        throw "Login CSRF token tidak ditemukan."
    }

    Write-Host "PASS: Login CSRF token obtained"

    # Step 2: Authenticate against PostgreSQL.
    $loginBody = @{
        username = $username
        password = $plainPassword
    } | ConvertTo-Json -Compress

    $login = Invoke-RestMethod `
        -Uri "$baseUrl/api/v1/auth/login" `
        -Method Post `
        -WebSession $session `
        -ContentType "application/json" `
        -Headers @{
            Origin = $baseUrl
            "X-CSRF-Token" = $loginCSRF.data.csrf_token
        } `
        -Body $loginBody

    $plainPassword = $null
    $loginBody = $null

    if ($login.data.role -ne "admin") {
        throw "Login berhasil tetapi role bukan admin."
    }

    Write-Host "PASS: Admin login successful"

    # Step 3: Access protected identity endpoint.
    $me = Invoke-RestMethod `
        -Uri "$baseUrl/api/v1/auth/me" `
        -Method Get `
        -WebSession $session

    if ($me.data.role -ne "admin") {
        throw "Session identity tidak sesuai."
    }

    Write-Host "PASS: Authenticated /me"

    # Step 4: Obtain session-bound CSRF token.
    $sessionCSRF = Invoke-RestMethod `
        -Uri "$baseUrl/api/v1/auth/csrf" `
        -Method Get `
        -WebSession $session

    if (-not $sessionCSRF.data.csrf_token) {
        throw "Session CSRF token tidak ditemukan."
    }

    Write-Host "PASS: Session CSRF token obtained"

    # Step 5: Revoke session through logout.
    Invoke-RestMethod `
        -Uri "$baseUrl/api/v1/auth/logout" `
        -Method Post `
        -WebSession $session `
        -Headers @{
            Origin = $baseUrl
            "X-CSRF-Token" = $sessionCSRF.data.csrf_token
        } | Out-Null

    Write-Host "PASS: Logout completed"

    # Step 6: The revoked session must not authenticate.
    try {
        Invoke-RestMethod `
            -Uri "$baseUrl/api/v1/auth/me" `
            -Method Get `
            -WebSession $session | Out-Null

        throw "FAIL: /me masih bisa diakses setelah logout."
    }
    catch {
        $response = $_.Exception.Response

        if ($null -eq $response -or
            [int]$response.StatusCode -ne 401) {
            throw
        }
    }

    Write-Host "PASS: Revoked session rejected (401)"
    Write-Host "ALL REAL AUTHENTICATION TESTS PASSED"
}
finally {
    $plainPassword = $null
    $loginBody = $null
    $securePassword = $null
}
