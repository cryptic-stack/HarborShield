param(
  [string]$ProjectRoot = (Split-Path -Parent $PSScriptRoot),
  [string]$BaseUrl = "http://localhost",
  [string]$BootstrapEmail = "admin@example.com",
  [string]$BootstrapPassword = "change_me_now",
  [string]$NewAdminPassword = "SessionSmoke!234"
)

$ErrorActionPreference = "Stop"
Set-Location $ProjectRoot

function Wait-ApiHealthy {
  param([int]$TimeoutSeconds = 300)
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  do {
    try {
      $health = Invoke-RestMethod -Uri "$BaseUrl/healthz"
      if ($health.status -eq "ok") {
        return
      }
    } catch {
    }
    Start-Sleep -Seconds 3
  } while ((Get-Date) -lt $deadline)

  throw "API did not become healthy within timeout."
}

function Reset-FirstRunBaseline {
  Write-Host "Resetting stack back to first-run baseline..."
  docker compose down -v --remove-orphans | Out-Host
  docker compose --env-file .env up --build -d | Out-Host
  Wait-ApiHealthy
}

function Login-Admin {
  param([string]$Password)
  Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/login" -Method Post -ContentType "application/json" -Body (@{
    email    = $BootstrapEmail
    password = $Password
  } | ConvertTo-Json -Compress)
}

function Refresh-Session {
  param([string]$RefreshToken)
  Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/refresh" -Method Post -ContentType "application/json" -Body (@{
    refreshToken = $RefreshToken
  } | ConvertTo-Json -Compress)
}

function Expect-RefreshUnauthorized {
  param(
    [string]$RefreshToken,
    [string]$Description
  )
  try {
    $null = Refresh-Session -RefreshToken $RefreshToken
    throw "Expected unauthorized refresh for $Description."
  } catch {
    if (-not $_.Exception.Response) {
      throw
    }
    if ($_.Exception.Response.StatusCode.value__ -ne 401) {
      throw "Expected 401 for $Description, got $($_.Exception.Response.StatusCode.value__)."
    }
  }
}

try {
  Write-Host "Resetting stack to clean state..."
  docker compose down -v --remove-orphans | Out-Host
  docker compose --env-file .env up --build -d | Out-Host
  Wait-ApiHealthy

  Write-Host "Bootstrapping admin password..."
  $bootstrap = Login-Admin -Password $BootstrapPassword
  if (-not $bootstrap.mustChangePassword) {
    throw "Expected bootstrap login to require password change."
  }
  $bootstrapHeaders = @{ Authorization = "Bearer $($bootstrap.accessToken)" }
  Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/change-password" -Method Post -Headers $bootstrapHeaders -ContentType "application/json" -Body (@{
    currentPassword = $BootstrapPassword
    newPassword     = $NewAdminPassword
  } | ConvertTo-Json -Compress) | Out-Null

  Write-Host "Creating concurrent admin sessions..."
  $sessionA = Login-Admin -Password $NewAdminPassword
  $sessionB = Login-Admin -Password $NewAdminPassword

  if ($sessionA.refreshToken -eq $sessionB.refreshToken) {
    throw "Expected distinct refresh tokens for concurrent sessions."
  }

  Write-Host "Validating refresh rotation..."
  $sessionARefresh1 = Refresh-Session -RefreshToken $sessionA.refreshToken
  if ($sessionARefresh1.refreshToken -eq $sessionA.refreshToken) {
    throw "Expected refresh rotation to issue a new refresh token."
  }
  Expect-RefreshUnauthorized -RefreshToken $sessionA.refreshToken -Description "rotated refresh token reuse"

  Write-Host "Validating single-session logout..."
  Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/logout" -Method Post -ContentType "application/json" -Body (@{
    refreshToken = $sessionB.refreshToken
  } | ConvertTo-Json -Compress) | Out-Null
  Expect-RefreshUnauthorized -RefreshToken $sessionB.refreshToken -Description "logged out session refresh"

  Write-Host "Creating another live session before logout-all..."
  $sessionC = Login-Admin -Password $NewAdminPassword
  $logoutAllHeaders = @{ Authorization = "Bearer $($sessionARefresh1.accessToken)" }

  Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/logout-all" -Method Post -Headers $logoutAllHeaders | Out-Null

  Write-Host "Validating logout-all invalidation..."
  Expect-RefreshUnauthorized -RefreshToken $sessionARefresh1.refreshToken -Description "post logout-all current session refresh"
  Expect-RefreshUnauthorized -RefreshToken $sessionC.refreshToken -Description "post logout-all other session refresh"

  Write-Host "Validating fresh login after logout-all..."
  $postLogoutAll = Login-Admin -Password $NewAdminPassword
  if (-not $postLogoutAll.accessToken -or -not $postLogoutAll.refreshToken) {
    throw "Expected fresh login to succeed after logout-all."
  }

  Write-Host "Session regression smoke passed."
}
finally {
  Reset-FirstRunBaseline
}
