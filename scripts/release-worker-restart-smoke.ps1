param(
  [string]$ProjectRoot = (Split-Path -Parent $PSScriptRoot),
  [string]$BaseUrl = "http://localhost",
  [string]$BootstrapEmail = "admin@example.com",
  [string]$BootstrapPassword = "change_me_now",
  [string]$NewAdminPassword = "WorkerRestart!234",
  [string]$BucketName = "release-worker-restart-bucket"
)

$ErrorActionPreference = "Stop"
Set-Location $ProjectRoot
. (Join-Path $PSScriptRoot "common.ps1")
$curlCommand = Get-CurlCommand
$nullDevice = Get-NullDevice
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

function Wait-Until {
  param(
    [scriptblock]$Condition,
    [string]$Description,
    [int]$TimeoutSeconds = 120
  )
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  do {
    if (& $Condition) {
      return
    }
    Start-Sleep -Seconds 3
  } while ((Get-Date) -lt $deadline)
  throw "Timed out waiting for $Description."
}

function Reset-FirstRunBaseline {
  Write-Host "Resetting stack back to first-run baseline..."
  docker compose down -v --remove-orphans | Out-Host
  docker compose --env-file .env up --build -d | Out-Host
  Wait-ApiHealthy
}

try {
  Write-Host "Resetting stack to clean state..."
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
  Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/change-password" -Method Post -Headers $headers -ContentType "application/json" -Body (@{
    currentPassword = $BootstrapPassword
    newPassword     = $NewAdminPassword
  } | ConvertTo-Json -Compress) | Out-Null

  $login2 = Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/login" -Method Post -ContentType "application/json" -Body (@{
    email    = $BootstrapEmail
    password = $NewAdminPassword
  } | ConvertTo-Json -Compress)
  $headers2 = @{ Authorization = "Bearer $($login2.accessToken)" }

  Write-Host "Seeding data for worker quota recalculation..."
  $bucket = Invoke-RestMethod -Uri "$BaseUrl/api/v1/buckets" -Method Post -Headers $headers2 -ContentType "application/json" -Body (@{
    name = $BucketName
  } | ConvertTo-Json -Compress)

  $uploadPath = Join-Path $tempDir "harborshield-worker-restart-smoke.txt"
  "hello from worker restart smoke" | Set-Content -Path $uploadPath -NoNewline
  $uploadStatus = & $curlCommand -sS -o $nullDevice -w "%{http_code}" -X POST "$BaseUrl/api/v1/buckets/$($bucket.id)/objects/upload" -H "Authorization: Bearer $($login2.accessToken)" -F "key=restart/data.txt" -F "file=@$uploadPath;type=text/plain"
  if ($uploadStatus -ne "201") {
    throw "Unexpected admin upload status: $uploadStatus"
  }

  Write-Host "Stopping worker and forcing a stale running job..."
  docker compose stop worker | Out-Host
  docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "UPDATE jobs SET status = 'running', next_run_at = NOW() - INTERVAL '1 minute', updated_at = NOW() - INTERVAL '2 minutes', attempts = attempts + 1, last_error = 'simulated_inflight_before_restart' WHERE job_type = 'bucket_quota_recalc';" | Out-Null

  $beforeJob = docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -F '|' -c "SELECT status, attempts, COALESCE(last_error, '') FROM jobs WHERE job_type = 'bucket_quota_recalc';"
  $beforeParts = $beforeJob.Trim().Split("|")
  if ($beforeParts[0] -ne "running") {
    throw "Expected bucket_quota_recalc to be forced into running state."
  }
  $beforeAttempts = [int]$beforeParts[1]

  Write-Host "Restarting worker to recover stale running job..."
  docker compose up -d worker | Out-Host

  Wait-Until -Description "worker health" -Condition {
    $workerStatus = docker compose ps --format json worker | ConvertFrom-Json
    return $workerStatus.State -eq "running" -and $workerStatus.Health -eq "healthy"
  }

  Wait-Until -Description "bucket_quota_recalc recovery" -Condition {
    $jobLine = docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -F '|' -c "SELECT status, attempts, COALESCE(last_error, '') FROM jobs WHERE job_type = 'bucket_quota_recalc';"
    $parts = $jobLine.Trim().Split("|")
    $quota = docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -F '|' -c "SELECT COALESCE(current_bytes, 0), COALESCE(current_objects, 0) FROM bucket_quotas WHERE bucket_id = '$($bucket.id)'::uuid;"
    $quotaParts = $quota.Trim().Split("|")
    return $parts[0] -eq "pending" -and [int]$parts[1] -gt $beforeAttempts -and $parts[2] -eq "" -and [int64]$quotaParts[0] -gt 0 -and [int64]$quotaParts[1] -ge 1
  }

  $afterJob = docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -F '|' -c "SELECT status, attempts, COALESCE(last_error, '') FROM jobs WHERE job_type = 'bucket_quota_recalc';"
  $quotaState = docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -F '|' -c "SELECT COALESCE(current_bytes, 0), COALESCE(current_objects, 0) FROM bucket_quotas WHERE bucket_id = '$($bucket.id)'::uuid;"

  Write-Host "Worker restart resilience smoke passed."
  Write-Host "Recovered job state: $($afterJob.Trim())"
  Write-Host "Quota state: $($quotaState.Trim())"
}
finally {
  Remove-Item -Force (Join-Path $tempDir "harborshield-worker-restart-smoke.txt") -ErrorAction SilentlyContinue
  Reset-FirstRunBaseline
}
