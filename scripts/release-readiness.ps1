param(
  [string]$ProjectRoot = (Split-Path -Parent $PSScriptRoot),
  [switch]$IncludeReleaseSmokes,
  [switch]$IncludeResilience,
  [switch]$IncludeS3Regression
)

$ErrorActionPreference = "Stop"
Set-Location $ProjectRoot

$results = [System.Collections.Generic.List[object]]::new()

function Invoke-ReleaseStep {
  param(
    [string]$Name,
    [scriptblock]$Action
  )

  $startedAt = Get-Date
  Write-Host ""
  Write-Host "==> $Name"

  try {
    & $Action
    $results.Add([pscustomobject]@{
      Name = $Name
      Status = "passed"
      DurationSeconds = [math]::Round(((Get-Date) - $startedAt).TotalSeconds, 1)
    })
  } catch {
    $results.Add([pscustomobject]@{
      Name = $Name
      Status = "failed"
      DurationSeconds = [math]::Round(((Get-Date) - $startedAt).TotalSeconds, 1)
      Error = $_.Exception.Message
    })
    throw
  }
}

Invoke-ReleaseStep -Name "Compose config validation" -Action {
  docker compose --env-file .env.example config
  if ($LASTEXITCODE -ne 0) {
    throw "docker compose config failed with exit code $LASTEXITCODE."
  }
}

Invoke-ReleaseStep -Name "Backend test suite" -Action {
  Push-Location (Join-Path $ProjectRoot "backend")
  try {
    go test ./...
    if ($LASTEXITCODE -ne 0) {
      throw "go test failed with exit code $LASTEXITCODE."
    }
  } finally {
    Pop-Location
  }
}

Invoke-ReleaseStep -Name "Frontend production build" -Action {
  Push-Location (Join-Path $ProjectRoot "frontend")
  try {
    npm.cmd run build
    if ($LASTEXITCODE -ne 0) {
      throw "npm build failed with exit code $LASTEXITCODE."
    }
  } finally {
    Pop-Location
  }
}

if ($IncludeReleaseSmokes) {
  Invoke-ReleaseStep -Name "Release clean-install smoke" -Action {
    powershell -ExecutionPolicy Bypass -File ".\scripts\release-clean-install-smoke.ps1"
    if ($LASTEXITCODE -ne 0) {
      throw "release-clean-install-smoke failed with exit code $LASTEXITCODE."
    }
  }

  Invoke-ReleaseStep -Name "Release upgrade smoke" -Action {
    powershell -ExecutionPolicy Bypass -File ".\scripts\release-upgrade-smoke.ps1"
    if ($LASTEXITCODE -ne 0) {
      throw "release-upgrade-smoke failed with exit code $LASTEXITCODE."
    }
  }
}

if ($IncludeResilience) {
  Invoke-ReleaseStep -Name "Worker restart resilience smoke" -Action {
    powershell -ExecutionPolicy Bypass -File ".\scripts\release-worker-restart-smoke.ps1"
    if ($LASTEXITCODE -ne 0) {
      throw "release-worker-restart-smoke failed with exit code $LASTEXITCODE."
    }
  }

  Invoke-ReleaseStep -Name "Session revocation regression smoke" -Action {
    powershell -ExecutionPolicy Bypass -File ".\scripts\release-session-regression-smoke.ps1"
    if ($LASTEXITCODE -ne 0) {
      throw "release-session-regression-smoke failed with exit code $LASTEXITCODE."
    }
  }
}

if ($IncludeS3Regression) {
  Invoke-ReleaseStep -Name "S3 SDK smoke" -Action {
    powershell -ExecutionPolicy Bypass -File ".\scripts\s3-sdk-smoke.ps1"
    if ($LASTEXITCODE -ne 0) {
      throw "s3-sdk-smoke failed with exit code $LASTEXITCODE."
    }
  }

  Invoke-ReleaseStep -Name "S3 edge-case smoke" -Action {
    powershell -ExecutionPolicy Bypass -File ".\scripts\s3-edge-smoke.ps1"
    if ($LASTEXITCODE -ne 0) {
      throw "s3-edge-smoke failed with exit code $LASTEXITCODE."
    }
  }

  Invoke-ReleaseStep -Name "S3 policy smoke" -Action {
    powershell -ExecutionPolicy Bypass -File ".\scripts\s3-policy-smoke.ps1"
    if ($LASTEXITCODE -ne 0) {
      throw "s3-policy-smoke failed with exit code $LASTEXITCODE."
    }
  }

  Invoke-ReleaseStep -Name "S3 policy conditions smoke" -Action {
    powershell -ExecutionPolicy Bypass -File ".\scripts\s3-policy-conditions-smoke.ps1"
    if ($LASTEXITCODE -ne 0) {
      throw "s3-policy-conditions-smoke failed with exit code $LASTEXITCODE."
    }
  }
}

Write-Host ""
Write-Host "Release readiness summary:"
$results | Format-Table -AutoSize | Out-Host

if (-not $IncludeReleaseSmokes -or -not $IncludeResilience -or -not $IncludeS3Regression) {
  Write-Host ""
  Write-Host "Optional gates not run in this pass:"
  if (-not $IncludeReleaseSmokes) {
    Write-Host "- release install and upgrade smokes"
  }
  if (-not $IncludeResilience) {
    Write-Host "- worker restart and session regression smokes"
  }
  if (-not $IncludeS3Regression) {
    Write-Host "- S3 SDK, edge, and policy regression smokes"
  }
}

Write-Host ""
Write-Host "Release readiness gate completed."
