param(
  [string]$ProjectRoot = (Split-Path -Parent $PSScriptRoot),
  [switch]$IncludeReleaseSmokes,
  [switch]$IncludeResilience,
  [switch]$IncludeS3Regression
)

$ErrorActionPreference = "Stop"
Set-Location $ProjectRoot

$results = [System.Collections.Generic.List[object]]::new()
$script:NpmCommand = if (Get-Command "npm.cmd" -ErrorAction SilentlyContinue) { "npm.cmd" } else { "npm" }
$script:PowerShellCommand = if (Get-Command "pwsh" -ErrorAction SilentlyContinue) { "pwsh" } else { "powershell" }

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

function Invoke-CheckedCommand {
  param(
    [string]$FilePath,
    [string[]]$Arguments
  )

  & $FilePath @Arguments
  if ($LASTEXITCODE -ne 0) {
    throw "$FilePath failed with exit code $LASTEXITCODE."
  }
}

Invoke-ReleaseStep -Name "Compose config validation" -Action {
  Invoke-CheckedCommand -FilePath "docker" -Arguments @("compose", "--env-file", ".env.example", "config")
}

Invoke-ReleaseStep -Name "Backend test suite" -Action {
  Push-Location (Join-Path $ProjectRoot "backend")
  try {
    Invoke-CheckedCommand -FilePath "go" -Arguments @("test", "./...")
  } finally {
    Pop-Location
  }
}

Invoke-ReleaseStep -Name "Frontend production build" -Action {
  Push-Location (Join-Path $ProjectRoot "frontend")
  try {
    Invoke-CheckedCommand -FilePath $script:NpmCommand -Arguments @("run", "build")
  } finally {
    Pop-Location
  }
}

if ($IncludeReleaseSmokes) {
  Invoke-ReleaseStep -Name "Release clean-install smoke" -Action {
    Invoke-CheckedCommand -FilePath $script:PowerShellCommand -Arguments @("-ExecutionPolicy", "Bypass", "-File", ".\scripts\release-clean-install-smoke.ps1")
  }

  Invoke-ReleaseStep -Name "Release upgrade smoke" -Action {
    Invoke-CheckedCommand -FilePath $script:PowerShellCommand -Arguments @("-ExecutionPolicy", "Bypass", "-File", ".\scripts\release-upgrade-smoke.ps1")
  }
}

if ($IncludeResilience) {
  Invoke-ReleaseStep -Name "Worker restart resilience smoke" -Action {
    Invoke-CheckedCommand -FilePath $script:PowerShellCommand -Arguments @("-ExecutionPolicy", "Bypass", "-File", ".\scripts\release-worker-restart-smoke.ps1")
  }

  Invoke-ReleaseStep -Name "Session revocation regression smoke" -Action {
    Invoke-CheckedCommand -FilePath $script:PowerShellCommand -Arguments @("-ExecutionPolicy", "Bypass", "-File", ".\scripts\release-session-regression-smoke.ps1")
  }
}

if ($IncludeS3Regression) {
  Invoke-ReleaseStep -Name "S3 SDK smoke" -Action {
    Invoke-CheckedCommand -FilePath $script:PowerShellCommand -Arguments @("-ExecutionPolicy", "Bypass", "-File", ".\scripts\s3-sdk-smoke.ps1")
  }

  Invoke-ReleaseStep -Name "S3 edge-case smoke" -Action {
    Invoke-CheckedCommand -FilePath $script:PowerShellCommand -Arguments @("-ExecutionPolicy", "Bypass", "-File", ".\scripts\s3-edge-smoke.ps1")
  }

  Invoke-ReleaseStep -Name "S3 policy smoke" -Action {
    Invoke-CheckedCommand -FilePath $script:PowerShellCommand -Arguments @("-ExecutionPolicy", "Bypass", "-File", ".\scripts\s3-policy-smoke.ps1")
  }

  Invoke-ReleaseStep -Name "S3 policy conditions smoke" -Action {
    Invoke-CheckedCommand -FilePath $script:PowerShellCommand -Arguments @("-ExecutionPolicy", "Bypass", "-File", ".\scripts\s3-policy-conditions-smoke.ps1")
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
