param(
  [string]$ProjectRoot = (Split-Path -Parent $PSScriptRoot),
  [string]$BaseUrl = "http://localhost",
  [string]$BootstrapEmail = "admin@example.com",
  [string]$BootstrapPassword = "change_me_now",
  [string]$UpgradedAdminPassword = "UpgradeSmoke!234",
  [string]$BucketName = "release-upgrade-bucket"
)

$ErrorActionPreference = "Stop"
Set-Location $ProjectRoot
. (Join-Path $PSScriptRoot "common.ps1")
$curlCommand = Get-CurlCommand
$tempDir = Get-TempDir

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

try {
  Write-Host "Resetting stack to clean pre-upgrade state..."
  docker compose down -v --remove-orphans | Out-Host
  docker compose --env-file .env up --build -d | Out-Host
  Wait-ApiHealthy

  Write-Host "Bootstrapping admin session..."
  $login = Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/login" -Method Post -ContentType "application/json" -Body (@{
    email    = $BootstrapEmail
    password = $BootstrapPassword
  } | ConvertTo-Json -Compress)

  if (-not $login.mustChangePassword) {
    throw "Expected bootstrap login to require password change."
  }

  $headers = @{ Authorization = "Bearer $($login.accessToken)" }

  Write-Host "Rotating bootstrap password for seeded upgrade state..."
  Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/change-password" -Method Post -Headers $headers -ContentType "application/json" -Body (@{
    currentPassword = $BootstrapPassword
    newPassword     = $UpgradedAdminPassword
  } | ConvertTo-Json -Compress) | Out-Null

  $login2 = Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/login" -Method Post -ContentType "application/json" -Body (@{
    email    = $BootstrapEmail
    password = $UpgradedAdminPassword
  } | ConvertTo-Json -Compress)
  $headers2 = @{ Authorization = "Bearer $($login2.accessToken)" }

  Write-Host "Completing first-run setup for an initialized deployment..."
  $setup = Invoke-RestMethod -Uri "$BaseUrl/api/v1/setup/complete" -Method Post -Headers $headers2 -ContentType "application/json" -Body (@{
    mode            = "single-node"
    distributedMode = "local"
    remoteEndpoints = @()
  } | ConvertTo-Json -Compress)
  if (-not $setup.completed) {
    throw "Expected setup to be marked complete before upgrade."
  }

  Write-Host "Seeding pre-upgrade data..."
  $bucket = Invoke-RestMethod -Uri "$BaseUrl/api/v1/buckets" -Method Post -Headers $headers2 -ContentType "application/json" -Body (@{
    name = $BucketName
  } | ConvertTo-Json -Compress)

  $credential = Invoke-RestMethod -Uri "$BaseUrl/api/v1/credentials" -Method Post -Headers $headers2 -ContentType "application/json" -Body (@{
    userId      = ""
    role        = "admin"
    description = "release upgrade smoke credential"
  } | ConvertTo-Json -Compress)

  $payloadPath = Join-Path $tempDir "harborshield-upgrade-smoke.txt"
  "hello from release upgrade smoke" | Set-Content -Path $payloadPath -NoNewline

  & $curlCommand -fsS -X PUT "$BaseUrl/s3/$BucketName/test.txt" `
    -H "X-S3P-Access-Key: $($credential.accessKey)" `
    -H "X-S3P-Secret: $($credential.secretKey)" `
    -H "Content-Type: text/plain" `
    --data-binary "@$payloadPath" | Out-Null

  $beforeSetup = Invoke-RestMethod -Uri "$BaseUrl/api/v1/setup/status" -Headers $headers2
  $beforeBuckets = Invoke-RestMethod -Uri "$BaseUrl/api/v1/buckets" -Headers $headers2
  $beforeAudit = Invoke-RestMethod -Uri "$BaseUrl/api/v1/audit?limit=20" -Headers $headers2
  $beforeDownloaded = & $curlCommand -fsS "$BaseUrl/s3/$BucketName/test.txt" `
    -H "X-S3P-Access-Key: $($credential.accessKey)" `
    -H "X-S3P-Secret: $($credential.secretKey)"

  if ($beforeDownloaded -ne "hello from release upgrade smoke") {
    throw "Pre-upgrade S3 verification failed."
  }

  Write-Host "Simulating upgrade by rebuilding services without removing volumes..."
  docker compose down --remove-orphans | Out-Host
  docker compose --env-file .env up --build -d | Out-Host
  Wait-ApiHealthy

  Write-Host "Validating post-upgrade admin login and preserved data..."
  $postLogin = Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/login" -Method Post -ContentType "application/json" -Body (@{
    email    = $BootstrapEmail
    password = $UpgradedAdminPassword
  } | ConvertTo-Json -Compress)

  if ($postLogin.mustChangePassword) {
    throw "Did not expect password rotation to be required after upgrade."
  }

  $postHeaders = @{ Authorization = "Bearer $($postLogin.accessToken)" }
  $afterSetup = Invoke-RestMethod -Uri "$BaseUrl/api/v1/setup/status" -Headers $postHeaders
  $afterBuckets = Invoke-RestMethod -Uri "$BaseUrl/api/v1/buckets" -Headers $postHeaders
  $afterAudit = Invoke-RestMethod -Uri "$BaseUrl/api/v1/audit?limit=20" -Headers $postHeaders

  $afterDownloaded = & $curlCommand -fsS "$BaseUrl/s3/$BucketName/test.txt" `
    -H "X-S3P-Access-Key: $($credential.accessKey)" `
    -H "X-S3P-Secret: $($credential.secretKey)"

  if ($afterDownloaded -ne "hello from release upgrade smoke") {
    throw "Post-upgrade object download did not match expected content."
  }
  if (-not $afterSetup.completed -or $afterSetup.required) {
    throw "Setup state was not preserved across upgrade."
  }
  if (-not ($afterBuckets.items | Where-Object { $_.name -eq $BucketName })) {
    throw "Expected seeded bucket to remain after upgrade."
  }
  if (@($afterAudit.items).Count -lt @($beforeAudit.items).Count) {
    throw "Expected audit history to be preserved across upgrade."
  }

  Write-Host "Validating continued writes after upgrade..."
  & $curlCommand -fsS -X PUT "$BaseUrl/s3/$BucketName/post-upgrade.txt" `
    -H "X-S3P-Access-Key: $($credential.accessKey)" `
    -H "X-S3P-Secret: $($credential.secretKey)" `
    -H "Content-Type: text/plain" `
    --data-binary "@$payloadPath" | Out-Null

  $postUpgradeDownloaded = & $curlCommand -fsS "$BaseUrl/s3/$BucketName/post-upgrade.txt" `
    -H "X-S3P-Access-Key: $($credential.accessKey)" `
    -H "X-S3P-Secret: $($credential.secretKey)"

  if ($postUpgradeDownloaded -ne "hello from release upgrade smoke") {
    throw "Post-upgrade write/read verification failed."
  }

  Remove-Item -Force $payloadPath -ErrorAction SilentlyContinue
  Write-Host "Release upgrade smoke passed."
}
finally {
  Reset-FirstRunBaseline
}
