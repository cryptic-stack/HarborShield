param(
  [string]$ProjectRoot = (Split-Path -Parent $PSScriptRoot),
  [string]$BaseUrl = "http://localhost",
  [string]$AdminEmail = "admin@example.com",
  [string]$AdminPassword = "change_me_now",
  [string]$FallbackAdminPassword = "",
  [string]$OutputRoot = "",
  [int]$LogTail = 300
)

$ErrorActionPreference = "Stop"
Set-Location $ProjectRoot

if (-not $OutputRoot) {
  $OutputRoot = Join-Path $ProjectRoot "artifacts\\support-bundles"
}

$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$bundleName = "harborshield-support-$timestamp"
$bundleDir = Join-Path $OutputRoot $bundleName
$bundleZip = "$bundleDir.zip"

New-Item -ItemType Directory -Path $bundleDir -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $bundleDir "compose") -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $bundleDir "health") -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $bundleDir "logs") -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $bundleDir "admin") -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $bundleDir "migrations") -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $bundleDir "database") -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $bundleDir "summary") -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $bundleDir "system") -Force | Out-Null

function Write-TextFile {
  param(
    [string]$Path,
    [string]$Content
  )
  Set-Content -Path $Path -Value $Content -Encoding UTF8
}

function Get-DotEnvValue {
  param(
    [string]$Key,
    [string]$Default = ""
  )
  $envPath = Join-Path $ProjectRoot ".env"
  if (-not (Test-Path $envPath)) {
    return $Default
  }
  $line = Get-Content -Path $envPath | Where-Object { $_ -match "^$Key=" } | Select-Object -First 1
  if (-not $line) {
    return $Default
  }
  return ($line -replace "^$Key=", "").Trim()
}

function Run-CommandCapture {
  param(
    [string]$Command,
    [string]$Path
  )
  try {
    $output = Invoke-Expression $Command 2>&1 | Out-String
    Write-TextFile -Path $Path -Content $output
  } catch {
    Write-TextFile -Path $Path -Content ($_.Exception.Message | Out-String)
  }
}

function Invoke-HttpCapture {
  param(
    [string]$Uri,
    [string]$Path,
    [hashtable]$Headers = @{},
    [int]$MaxLines = 0
  )
  try {
    $response = Invoke-WebRequest -Uri $Uri -Headers $Headers -UseBasicParsing
    $body = [string]$response.Content
    if ($MaxLines -gt 0) {
      $body = (($body -split "`r?`n") | Select-Object -First $MaxLines) -join "`r`n"
    }
    Write-TextFile -Path $Path -Content $body
  } catch {
    if ($_.Exception.Response) {
      if ($_.Exception.Response -is [System.Net.Http.HttpResponseMessage]) {
        $body = $_.Exception.Response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
      } else {
        $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
        $body = $reader.ReadToEnd()
        $reader.Close()
      }
      Write-TextFile -Path $Path -Content $body
    } else {
      Write-TextFile -Path $Path -Content ($_.Exception.Message | Out-String)
    }
  }
}

function Login-Admin {
  param(
    [string]$Email,
    [string]$Password
  )
  $payload = @{
    email = $Email
    password = $Password
  } | ConvertTo-Json -Compress
  return Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/login" -Method Post -ContentType "application/json" -Body $payload
}

Write-TextFile -Path (Join-Path $bundleDir "bundle-info.txt") -Content @"
bundleName: $bundleName
createdAt: $(Get-Date -Format o)
projectRoot: $ProjectRoot
baseUrl: $BaseUrl
machine: $env:COMPUTERNAME
"@

Run-CommandCapture -Command "docker version" -Path (Join-Path $bundleDir "system\\docker-version.txt")
Run-CommandCapture -Command "docker compose version" -Path (Join-Path $bundleDir "system\\docker-compose-version.txt")

Run-CommandCapture -Command "docker compose ps" -Path (Join-Path $bundleDir "compose\\ps.txt")
Run-CommandCapture -Command "docker compose config --services" -Path (Join-Path $bundleDir "compose\\services.txt")
Run-CommandCapture -Command "docker compose images" -Path (Join-Path $bundleDir "compose\\images.txt")
Run-CommandCapture -Command "docker volume ls" -Path (Join-Path $bundleDir "compose\\volumes.txt")
Run-CommandCapture -Command "docker system df" -Path (Join-Path $bundleDir "system\\docker-system-df.txt")

Invoke-HttpCapture -Uri "$BaseUrl/healthz" -Path (Join-Path $bundleDir "health\\healthz.json")
Invoke-HttpCapture -Uri "$BaseUrl/readyz" -Path (Join-Path $bundleDir "health\\readyz.txt")
Invoke-HttpCapture -Uri "$BaseUrl/metrics" -Path (Join-Path $bundleDir "health\\metrics-sample.txt") -MaxLines 300

$services = @("api", "worker", "frontend", "caddy", "postgres", "redis")
foreach ($service in $services) {
  Run-CommandCapture -Command "docker compose logs $service --tail=$LogTail" -Path (Join-Path $bundleDir "logs\\$service.log")
}

Get-ChildItem -Path (Join-Path $ProjectRoot "backend\\migrations") -Name | Sort-Object | Set-Content -Path (Join-Path $bundleDir "migrations\\files.txt")
Write-TextFile -Path (Join-Path $bundleDir "migrations\\notes.txt") -Content @"
HarborShield currently applies SQL files from backend/migrations at startup.
Runtime migration version tracking is not yet persisted in a dedicated schema_migrations table.
This bundle includes the migration file inventory and current database table inventory for troubleshooting.
"@

try {
  $tableOutput = docker compose exec -T postgres sh -lc 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "\\dt"' 2>&1 | Out-String
  Write-TextFile -Path (Join-Path $bundleDir "migrations\\database-tables.txt") -Content $tableOutput
} catch {
  Write-TextFile -Path (Join-Path $bundleDir "migrations\\database-tables.txt") -Content ($_.Exception.Message | Out-String)
}

$postgresUser = Get-DotEnvValue -Key "POSTGRES_USER" -Default "s3platform"
$postgresDb = Get-DotEnvValue -Key "POSTGRES_DB" -Default "s3platform"

try {
  $rowCountSql = @"
SELECT 'users' AS table_name, COUNT(*) AS row_count FROM users
UNION ALL SELECT 'credentials', COUNT(*) FROM credentials
UNION ALL SELECT 'buckets', COUNT(*) FROM buckets
UNION ALL SELECT 'objects', COUNT(*) FROM objects
UNION ALL SELECT 'audit_logs', COUNT(*) FROM audit_logs
UNION ALL SELECT 'event_targets', COUNT(*) FROM event_targets
UNION ALL SELECT 'event_deliveries', COUNT(*) FROM event_deliveries
UNION ALL SELECT 'storage_nodes', COUNT(*) FROM storage_nodes
ORDER BY table_name;
"@
  $rowCountOutput = $rowCountSql | docker compose exec -T postgres psql -U $postgresUser -d $postgresDb -P pager=off -A -F "|" 2>&1 | Out-String
  Write-TextFile -Path (Join-Path $bundleDir "database\\row-counts.txt") -Content $rowCountOutput
} catch {
  Write-TextFile -Path (Join-Path $bundleDir "database\\row-counts.txt") -Content ($_.Exception.Message | Out-String)
}

try {
  $quotaSql = @"
SELECT COUNT(*) FILTER (WHERE max_bytes IS NOT NULL OR max_objects IS NOT NULL) AS bucket_quota_rows,
       COUNT(*) FILTER (WHERE warning_threshold_percent IS NOT NULL) AS bucket_warning_rows
FROM bucket_quotas;
SELECT COUNT(*) FILTER (WHERE max_bytes IS NOT NULL) AS user_quota_rows,
       COUNT(*) FILTER (WHERE warning_threshold_percent IS NOT NULL) AS user_warning_rows
FROM user_quotas;
"@
  $quotaOutput = $quotaSql | docker compose exec -T postgres psql -U $postgresUser -d $postgresDb -P pager=off -A -F "|" 2>&1 | Out-String
  Write-TextFile -Path (Join-Path $bundleDir "database\\quota-state.txt") -Content $quotaOutput
} catch {
  Write-TextFile -Path (Join-Path $bundleDir "database\\quota-state.txt") -Content ($_.Exception.Message | Out-String)
}

$adminHeaders = @{}
$authStatus = [ordered]@{
  authenticated = $false
  usedFallback = $false
  mustChangePassword = $false
}

try {
  $login = Login-Admin -Email $AdminEmail -Password $AdminPassword
} catch {
  if ($FallbackAdminPassword) {
    $login = Login-Admin -Email $AdminEmail -Password $FallbackAdminPassword
    $authStatus.usedFallback = $true
  } else {
    $login = $null
  }
}

if ($login) {
  $authStatus.authenticated = $true
  $authStatus.mustChangePassword = [bool]$login.mustChangePassword
  $adminHeaders = @{ Authorization = "Bearer $($login.accessToken)" }
  Invoke-HttpCapture -Uri "$BaseUrl/api/v1/auth/me" -Headers $adminHeaders -Path (Join-Path $bundleDir "admin\\auth-me.json")
  Invoke-HttpCapture -Uri "$BaseUrl/api/v1/setup/status" -Headers $adminHeaders -Path (Join-Path $bundleDir "admin\\setup-status.json")
  Invoke-HttpCapture -Uri "$BaseUrl/api/v1/settings" -Headers $adminHeaders -Path (Join-Path $bundleDir "admin\\settings.json")
  Invoke-HttpCapture -Uri "$BaseUrl/api/v1/dashboard" -Headers $adminHeaders -Path (Join-Path $bundleDir "admin\\dashboard.json")
  Invoke-HttpCapture -Uri "$BaseUrl/api/v1/health" -Headers $adminHeaders -Path (Join-Path $bundleDir "admin\\health.json")
  Invoke-HttpCapture -Uri "$BaseUrl/api/v1/storage/nodes" -Headers $adminHeaders -Path (Join-Path $bundleDir "admin\\storage-nodes.json")
  Invoke-HttpCapture -Uri "$BaseUrl/api/v1/audit?limit=50" -Headers $adminHeaders -Path (Join-Path $bundleDir "admin\\audit-recent.json")
  try {
    $settings = Get-Content -Path (Join-Path $bundleDir "admin\\settings.json") -Raw | ConvertFrom-Json
    $settingsSummary = [ordered]@{
      appEnv                     = $settings.appEnv
      storageBackend             = $settings.storageBackend
      storageDistributedReplicas = $settings.storageDistributedReplicas
      storageDefaultClass        = $settings.storageDefaultClass
      oidcEnabled                = $settings.oidcEnabled
      oidcLoginReady             = $settings.oidcLoginReady
      oidcClientSecretConfigured = $settings.oidcClientSecretConfigured
      malwareEnabled             = $settings.enableClamAV
      malwareScanMode            = $settings.malwareScanMode
    }
    $settingsSummary | ConvertTo-Json -Depth 6 | Set-Content -Path (Join-Path $bundleDir "summary\\settings-summary.json")
  } catch {
    Write-TextFile -Path (Join-Path $bundleDir "summary\\settings-summary.json") -Content ($_.Exception.Message | Out-String)
  }
  try {
    $setup = Get-Content -Path (Join-Path $bundleDir "admin\\setup-status.json") -Raw | ConvertFrom-Json
    $setupSummary = [ordered]@{
      completed             = $setup.completed
      required              = $setup.required
      applyRequired         = $setup.applyRequired
      runtimeStorageBackend = $setup.runtimeStorageBackend
      desiredStorageBackend = $setup.desiredStorageBackend
      distributedScope      = $setup.distributedScope
      remoteEndpointCount   = @($setup.remoteEndpoints).Count
    }
    $setupSummary | ConvertTo-Json -Depth 6 | Set-Content -Path (Join-Path $bundleDir "summary\\setup-summary.json")
  } catch {
    Write-TextFile -Path (Join-Path $bundleDir "summary\\setup-summary.json") -Content ($_.Exception.Message | Out-String)
  }
  try {
    $auditPayload = Get-Content -Path (Join-Path $bundleDir "admin\\audit-recent.json") -Raw | ConvertFrom-Json
    $items = @($auditPayload.items)
    $auditSummary = [ordered]@{
      totalItems        = $items.Count
      latestTimestamp   = if ($items.Count -gt 0) { $items[0].timestamp } else { "" }
      byOutcome         = @{}
      byCategory        = @{}
      bySeverity        = @{}
      topActions        = @()
    }
    foreach ($group in ($items | Group-Object -Property outcome)) {
      $auditSummary.byOutcome[$group.Name] = $group.Count
    }
    foreach ($group in ($items | Group-Object -Property category)) {
      $auditSummary.byCategory[$group.Name] = $group.Count
    }
    foreach ($group in ($items | Group-Object -Property severity)) {
      $auditSummary.bySeverity[$group.Name] = $group.Count
    }
    $auditSummary.topActions = @(
      $items |
        Group-Object -Property action |
        Sort-Object -Property Count -Descending |
        Select-Object -First 10 |
        ForEach-Object { [ordered]@{ action = $_.Name; count = $_.Count } }
    )
    $auditSummary | ConvertTo-Json -Depth 8 | Set-Content -Path (Join-Path $bundleDir "summary\\audit-summary.json")
  } catch {
    Write-TextFile -Path (Join-Path $bundleDir "summary\\audit-summary.json") -Content ($_.Exception.Message | Out-String)
  }
} else {
  Write-TextFile -Path (Join-Path $bundleDir "admin\\auth-error.txt") -Content "Unable to authenticate to the admin API with the supplied credentials."
}

$authStatus | ConvertTo-Json | Set-Content -Path (Join-Path $bundleDir "admin\\auth-status.json")

if (Test-Path $bundleZip) {
  Remove-Item -Force $bundleZip
}
Compress-Archive -Path $bundleDir -DestinationPath $bundleZip

Write-Host "Support bundle created:"
Write-Host $bundleZip
