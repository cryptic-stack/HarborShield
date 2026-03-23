param(
  [string]$ProjectRoot = "",
  [Parameter(Mandatory = $true)]
  [string]$Version,
  [Parameter(Mandatory = $true)]
  [string]$ApiImage,
  [Parameter(Mandatory = $true)]
  [string]$WorkerImage,
  [Parameter(Mandatory = $true)]
  [string]$BlobnodeImage,
  [Parameter(Mandatory = $true)]
  [string]$FrontendImage
)

$ErrorActionPreference = "Stop"
$scriptRoot = if ($PSScriptRoot) { $PSScriptRoot } else { Split-Path -Parent $MyInvocation.MyCommand.Path }
if ([string]::IsNullOrWhiteSpace($ProjectRoot)) {
  $ProjectRoot = Split-Path -Parent $scriptRoot
}
Set-Location $ProjectRoot

$releaseName = "HarborShield-$Version"
$outputRoot = Join-Path $ProjectRoot "artifacts/releases"
$stagingRoot = Join-Path $outputRoot $releaseName

if (Test-Path $stagingRoot) {
  Remove-Item -Recurse -Force $stagingRoot
}

New-Item -ItemType Directory -Path $stagingRoot | Out-Null

$copies = @(
  @{ Source = "docker-compose.yml"; Destination = "docker-compose.yml" }
  @{ Source = ".env.example"; Destination = ".env.example" }
  @{ Source = "README.md"; Destination = "README.md" }
  @{ Source = "LICENSE"; Destination = "LICENSE" }
  @{ Source = "docs/deployment.md"; Destination = "docs/deployment.md" }
  @{ Source = "docs/operations.md"; Destination = "docs/operations.md" }
  @{ Source = "docs/troubleshooting.md"; Destination = "docs/troubleshooting.md" }
  @{ Source = "docs/backup-restore.md"; Destination = "docs/backup-restore.md" }
  @{ Source = "docs/secret-rotation.md"; Destination = "docs/secret-rotation.md" }
  @{ Source = "deploy/caddy/Caddyfile"; Destination = "deploy/caddy/Caddyfile" }
  @{ Source = "deploy/init/prometheus.yml"; Destination = "deploy/init/prometheus.yml" }
)

foreach ($entry in $copies) {
  $sourcePath = Join-Path $ProjectRoot $entry.Source
  $destinationPath = Join-Path $stagingRoot $entry.Destination
  $destinationDir = Split-Path -Parent $destinationPath
  if (-not (Test-Path $destinationDir)) {
    New-Item -ItemType Directory -Path $destinationDir -Force | Out-Null
  }
  Copy-Item -Path $sourcePath -Destination $destinationPath -Force
}

$imagesEnvPath = Join-Path $stagingRoot "release-images.env"
@(
  "# HarborShield release image pins for $Version"
  "API_IMAGE=$ApiImage"
  "WORKER_IMAGE=$WorkerImage"
  "BLOBNODE_IMAGE=$BlobnodeImage"
  "FRONTEND_IMAGE=$FrontendImage"
) | Set-Content -Path $imagesEnvPath -NoNewline:$false

$notesPath = Join-Path $stagingRoot "RELEASE.txt"
@(
  "HarborShield $Version"
  ""
  "Quick start:"
  "1. Copy .env.example to .env and rotate all secrets."
  "2. Review release-images.env and keep the image pins intact for this release."
  "3. Start the stack with:"
  "   docker compose --env-file .env --env-file release-images.env up -d"
) | Set-Content -Path $notesPath -NoNewline:$false

if (-not (Test-Path $outputRoot)) {
  New-Item -ItemType Directory -Path $outputRoot -Force | Out-Null
}

$zipPath = Join-Path $outputRoot "$releaseName.zip"
if (Test-Path $zipPath) {
  Remove-Item -Force $zipPath
}
$zipItems = Get-ChildItem -Force -Path $stagingRoot | ForEach-Object { $_.FullName }
Compress-Archive -Path $zipItems -DestinationPath $zipPath

$tarPath = Join-Path $outputRoot "$releaseName.tar.gz"
if (Test-Path $tarPath) {
  Remove-Item -Force $tarPath
}
tar -czf $tarPath -C $outputRoot $releaseName
if ($LASTEXITCODE -ne 0) {
  throw "tar failed with exit code $LASTEXITCODE."
}

$hashLines = @()
foreach ($artifactPath in @($zipPath, $tarPath)) {
  $hash = Get-FileHash -Path $artifactPath -Algorithm SHA256
  $hashLines += "$($hash.Hash.ToLowerInvariant())  $([System.IO.Path]::GetFileName($artifactPath))"
}

$checksumsPath = Join-Path $outputRoot "$releaseName-sha256.txt"
$hashLines | Set-Content -Path $checksumsPath -NoNewline:$false

Write-Host "Created release bundle artifacts:"
Write-Host "- $zipPath"
Write-Host "- $tarPath"
Write-Host "- $checksumsPath"
