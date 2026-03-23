param(
  [string]$ProjectRoot = (Split-Path -Parent $PSScriptRoot),
  [string]$BaseUrl = "http://localhost",
  [string]$BootstrapEmail = "admin@example.com",
  [string]$BootstrapPassword = "change_me_now",
  [string]$NewAdminPassword = "ReleaseSmoke!234",
  [string]$BucketName = "release-smoke-bucket"
)

$ErrorActionPreference = "Stop"

Set-Location $ProjectRoot

Write-Host "Resetting stack to clean-install state..."
docker compose down -v --remove-orphans | Out-Host
docker compose --env-file .env up --build -d | Out-Host

Write-Host "Waiting for API health..."
$deadline = (Get-Date).AddMinutes(5)
do {
  try {
    $health = Invoke-RestMethod -Uri "$BaseUrl/healthz"
    if ($health.status -eq "ok") {
      break
    }
  } catch {
  }
  Start-Sleep -Seconds 3
} while ((Get-Date) -lt $deadline)

if (-not $health -or $health.status -ne "ok") {
  throw "API did not become healthy within timeout."
}

Write-Host "Validating bootstrap login..."
$login = Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/login" -Method Post -ContentType "application/json" -Body (@{
  email = $BootstrapEmail
  password = $BootstrapPassword
} | ConvertTo-Json -Compress)

if (-not $login.mustChangePassword) {
  throw "Expected bootstrap login to require password change."
}

$headers = @{ Authorization = "Bearer $($login.accessToken)" }

Write-Host "Changing bootstrap password..."
Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/change-password" -Method Post -Headers $headers -ContentType "application/json" -Body (@{
  currentPassword = $BootstrapPassword
  newPassword = $NewAdminPassword
} | ConvertTo-Json -Compress) | Out-Null

Write-Host "Logging in with rotated admin password..."
$login2 = Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/login" -Method Post -ContentType "application/json" -Body (@{
  email = $BootstrapEmail
  password = $NewAdminPassword
} | ConvertTo-Json -Compress)

if ($login2.mustChangePassword) {
  throw "Did not expect mustChangePassword after completing password update."
}

$headers2 = @{ Authorization = "Bearer $($login2.accessToken)" }

Write-Host "Checking first-run setup status..."
$setup = Invoke-RestMethod -Uri "$BaseUrl/api/v1/setup/status" -Headers $headers2
if (-not $setup.required -or $setup.completed) {
  throw "Expected deployment setup to remain incomplete on clean install."
}

Write-Host "Creating a bucket..."
$bucket = Invoke-RestMethod -Uri "$BaseUrl/api/v1/buckets" -Method Post -Headers $headers2 -ContentType "application/json" -Body (@{
  name = $BucketName
} | ConvertTo-Json -Compress)

Write-Host "Creating an S3 credential..."
$credential = Invoke-RestMethod -Uri "$BaseUrl/api/v1/credentials" -Method Post -Headers $headers2 -ContentType "application/json" -Body (@{
  userId = ""
  role = "admin"
  description = "release smoke credential"
} | ConvertTo-Json -Compress)

$payloadPath = Join-Path $env:TEMP "harborshield-release-smoke.txt"
"hello from release clean install smoke" | Set-Content -Path $payloadPath -NoNewline

Write-Host "Uploading through the S3 plane..."
curl.exe -fsS -X PUT "$BaseUrl/s3/$BucketName/test.txt" `
  -H "X-S3P-Access-Key: $($credential.accessKey)" `
  -H "X-S3P-Secret: $($credential.secretKey)" `
  -H "Content-Type: text/plain" `
  --data-binary "@$payloadPath" | Out-Null

Write-Host "Downloading through the S3 plane..."
$downloaded = curl.exe -fsS "$BaseUrl/s3/$BucketName/test.txt" `
  -H "X-S3P-Access-Key: $($credential.accessKey)" `
  -H "X-S3P-Secret: $($credential.secretKey)"

if ($downloaded -ne "hello from release clean install smoke") {
  throw "Downloaded payload did not match expected content."
}

Write-Host "Checking audit visibility..."
$audit = Invoke-RestMethod -Uri "$BaseUrl/api/v1/audit?action=bucket.create&limit=20" -Headers $headers2
if (-not $audit.items -or $audit.items.Count -lt 1) {
  throw "Expected at least one bucket.create audit event."
}

Remove-Item -Force $payloadPath -ErrorAction SilentlyContinue

Write-Host "Release clean-install smoke passed."
