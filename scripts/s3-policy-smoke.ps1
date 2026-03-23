param(
  [string]$ProjectRoot = (Split-Path -Parent $PSScriptRoot),
  [string]$BaseUrl = "http://localhost",
  [string]$BootstrapEmail = "admin@example.com",
  [string]$BootstrapPassword = "change_me_now",
  [string]$NewAdminPassword = "S3PolicySmoke!234"
)

$ErrorActionPreference = "Stop"

Set-Location $ProjectRoot
. (Join-Path $PSScriptRoot "common.ps1")
$curlCommand = Get-CurlCommand
$nullDevice = Get-NullDevice

function Assert-Status {
  param(
    [string]$Actual,
    [string]$Expected,
    [string]$Message
  )
  if ($Actual -ne $Expected) {
    throw "$Message. Expected $Expected, got $Actual"
  }
}

function SignedStatus {
  param(
    [string]$Method,
    [string]$Url,
    [string]$AccessKey,
    [string]$SecretKey,
    [string]$OutputFile = $nullDevice,
    [string[]]$ExtraArgs = @()
  )

  $args = @(
    "-sS", "-o", $OutputFile, "-w", "%{http_code}",
    "--aws-sigv4", "aws:amz:us-east-1:s3",
    "--user", "${AccessKey}:${SecretKey}",
    "-X", $Method
  ) + $ExtraArgs + @($Url)

  return (& $curlCommand @args)
}

$publicFile = $null
$privateFile = $null
$policyFile = $null
$policyReadFile = $null
$listPublicFile = $null
$listPrivateFile = $null
$bucket = "s3-policy-smoke-" + ([guid]::NewGuid().ToString("N").Substring(0, 8))

try {
  Write-Host "Waiting for HarborShield health..."
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
    throw "HarborShield did not become healthy within timeout."
  }

  Write-Host "Logging in with admin account..."
  $loginBody = @{
    email = $BootstrapEmail
    password = $BootstrapPassword
  }
  try {
    $login = Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/login" -Method Post -ContentType "application/json" -Body ($loginBody | ConvertTo-Json -Compress)
  } catch {
    $loginBody.password = $NewAdminPassword
    $login = Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/login" -Method Post -ContentType "application/json" -Body ($loginBody | ConvertTo-Json -Compress)
  }

  $headers = @{ Authorization = "Bearer $($login.accessToken)" }
  if ($login.mustChangePassword) {
    Write-Host "Rotating bootstrap password for policy smoke..."
    Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/change-password" -Method Post -Headers $headers -ContentType "application/json" -Body (@{
      currentPassword = $BootstrapPassword
      newPassword = $NewAdminPassword
    } | ConvertTo-Json -Compress) | Out-Null

    $login = Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/login" -Method Post -ContentType "application/json" -Body (@{
      email = $BootstrapEmail
      password = $NewAdminPassword
    } | ConvertTo-Json -Compress)

    $headers = @{ Authorization = "Bearer $($login.accessToken)" }
  }

  Write-Host "Creating policy smoke credentials..."
  $allowed = Invoke-RestMethod -Uri "$BaseUrl/api/v1/credentials" -Method Post -Headers $headers -ContentType "application/json" -Body (@{
    role        = "admin"
    description = "policy smoke allowed"
  } | ConvertTo-Json -Compress)
  $other = Invoke-RestMethod -Uri "$BaseUrl/api/v1/credentials" -Method Post -Headers $headers -ContentType "application/json" -Body (@{
    role        = "admin"
    description = "policy smoke other"
  } | ConvertTo-Json -Compress)

  Write-Host "Creating policy smoke bucket..."
  $putBucket = SignedStatus -Method "PUT" -Url "$BaseUrl/s3/$bucket" -AccessKey $allowed.accessKey -SecretKey $allowed.secretKey
  Assert-Status -Actual $putBucket -Expected "200" -Message "bucket create failed"

  $publicFile = Join-Path $env:TEMP "hs-policy-public.txt"
  $privateFile = Join-Path $env:TEMP "hs-policy-private.txt"
  Set-Content -Path $publicFile -Value "public data" -NoNewline
  Set-Content -Path $privateFile -Value "private data" -NoNewline

  Write-Host "Uploading public and private objects..."
  $putPublic = SignedStatus -Method "PUT" -Url "$BaseUrl/s3/$bucket/public/visible.txt" -AccessKey $allowed.accessKey -SecretKey $allowed.secretKey -ExtraArgs @("-T", $publicFile)
  $putPrivate = SignedStatus -Method "PUT" -Url "$BaseUrl/s3/$bucket/private/secret.txt" -AccessKey $allowed.accessKey -SecretKey $allowed.secretKey -ExtraArgs @("-T", $privateFile)
  Assert-Status -Actual $putPublic -Expected "200" -Message "public object put failed"
  Assert-Status -Actual $putPrivate -Expected "200" -Message "private object put failed"

  $policyFile = Join-Path $env:TEMP "hs-policy-smoke.json"
  $policy = @"
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "DenyOthersPrivateRead",
      "Effect": "Deny",
      "NotPrincipal": {
        "AWS": "$($allowed.accessKey)"
      },
      "Action": "s3:GetObject",
      "Resource": "arn:aws:s3:::$bucket/private/*"
    }
  ]
}
"@
  Set-Content -Path $policyFile -Value $policy -NoNewline

  Write-Host "Applying and reading bucket policy..."
  $putPolicy = SignedStatus -Method "PUT" -Url "$BaseUrl/s3/${bucket}?policy" -AccessKey $allowed.accessKey -SecretKey $allowed.secretKey -ExtraArgs @("--upload-file", $policyFile)
  Assert-Status -Actual $putPolicy -Expected "204" -Message "put bucket policy failed"
  $policyReadFile = Join-Path $env:TEMP "hs-policy-smoke-read.json"
  $getPolicy = SignedStatus -Method "GET" -Url "$BaseUrl/s3/${bucket}?policy" -AccessKey $allowed.accessKey -SecretKey $allowed.secretKey -OutputFile $policyReadFile
  Assert-Status -Actual $getPolicy -Expected "200" -Message "get bucket policy failed"

  Write-Host "Validating signed principal-specific access..."
  $publicGetFile = Join-Path $env:TEMP "hs-policy-public-get.txt"
  $publicGet = SignedStatus -Method "GET" -Url "$BaseUrl/s3/$bucket/public/visible.txt" -AccessKey $other.accessKey -SecretKey $other.secretKey -OutputFile $publicGetFile
  Assert-Status -Actual $publicGet -Expected "200" -Message "signed public object get should succeed"
  if ((Get-Content -Path $publicGetFile -Raw) -ne "public data") {
    throw "signed public get returned unexpected body"
  }

  $allowedGetFile = Join-Path $env:TEMP "hs-policy-allowed-get.txt"
  $allowedGet = SignedStatus -Method "GET" -Url "$BaseUrl/s3/$bucket/private/secret.txt" -AccessKey $allowed.accessKey -SecretKey $allowed.secretKey -OutputFile $allowedGetFile
  Assert-Status -Actual $allowedGet -Expected "200" -Message "allowed principal should read private object"
  if ((Get-Content -Path $allowedGetFile -Raw) -ne "private data") {
    throw "allowed private read returned unexpected body"
  }

  $otherGetFile = Join-Path $env:TEMP "hs-policy-other-get.xml"
  $otherGet = SignedStatus -Method "GET" -Url "$BaseUrl/s3/$bucket/private/secret.txt" -AccessKey $other.accessKey -SecretKey $other.secretKey -OutputFile $otherGetFile
  Assert-Status -Actual $otherGet -Expected "403" -Message "other principal should be denied private object access"
  if ((Get-Content -Path $otherGetFile -Raw) -notmatch "AccessDenied") {
    throw "other principal denial should return AccessDenied"
  }

  Write-Host "Cleaning policy bucket..."
  $deletePolicy = SignedStatus -Method "DELETE" -Url "$BaseUrl/s3/${bucket}?policy" -AccessKey $allowed.accessKey -SecretKey $allowed.secretKey
  Assert-Status -Actual $deletePolicy -Expected "204" -Message "delete bucket policy failed"

  $deletePublic = SignedStatus -Method "DELETE" -Url "$BaseUrl/s3/$bucket/public/visible.txt" -AccessKey $allowed.accessKey -SecretKey $allowed.secretKey
  $deletePrivate = SignedStatus -Method "DELETE" -Url "$BaseUrl/s3/$bucket/private/secret.txt" -AccessKey $allowed.accessKey -SecretKey $allowed.secretKey
  Assert-Status -Actual $deletePublic -Expected "204" -Message "delete public object failed"
  Assert-Status -Actual $deletePrivate -Expected "204" -Message "delete private object failed"

  $versionsFile = Join-Path $env:TEMP "hs-policy-versions.xml"
  $versionsStatus = SignedStatus -Method "GET" -Url "$BaseUrl/s3/${bucket}?versions" -AccessKey $allowed.accessKey -SecretKey $allowed.secretKey -OutputFile $versionsFile
  Assert-Status -Actual $versionsStatus -Expected "200" -Message "list policy bucket versions failed"
  $versionsXml = Get-Content -Path $versionsFile -Raw
  foreach ($markerMatch in [regex]::Matches($versionsXml, '(?s)<DeleteMarker>\s*<Key>([^<]+)</Key>\s*<VersionId>([^<]+)</VersionId>')) {
    $status = SignedStatus -Method "DELETE" -Url "$BaseUrl/s3/$bucket/$($markerMatch.Groups[1].Value)?versionId=$($markerMatch.Groups[2].Value)" -AccessKey $allowed.accessKey -SecretKey $allowed.secretKey
    Assert-Status -Actual $status -Expected "204" -Message "delete marker cleanup failed"
  }
  foreach ($versionMatch in [regex]::Matches($versionsXml, '(?s)<Version>\s*<Key>([^<]+)</Key>\s*<VersionId>([^<]+)</VersionId>')) {
    $status = SignedStatus -Method "DELETE" -Url "$BaseUrl/s3/$bucket/$($versionMatch.Groups[1].Value)?versionId=$($versionMatch.Groups[2].Value)" -AccessKey $allowed.accessKey -SecretKey $allowed.secretKey
    Assert-Status -Actual $status -Expected "204" -Message "policy bucket version cleanup failed"
  }

  $deleteBucket = SignedStatus -Method "DELETE" -Url "$BaseUrl/s3/$bucket" -AccessKey $allowed.accessKey -SecretKey $allowed.secretKey
  Assert-Status -Actual $deleteBucket -Expected "204" -Message "policy bucket delete failed"

  [pscustomobject]@{
    bucket = $bucket
    getPutDeletePolicy = $true
    signedReadFlow = $true
    notPrincipalDeny = $true
    cleanedBucket = $true
  } | ConvertTo-Json -Compress
}
finally {
  $tempFiles = @(
    $publicFile, $privateFile, $policyFile, $policyReadFile,
    (Join-Path $env:TEMP "hs-policy-public-get.txt"),
    (Join-Path $env:TEMP "hs-policy-allowed-get.txt"),
    (Join-Path $env:TEMP "hs-policy-other-get.xml"),
    (Join-Path $env:TEMP "hs-policy-versions.xml")
  )
  foreach ($path in $tempFiles) {
    if ($path) {
      Remove-Item -Force $path -ErrorAction SilentlyContinue
    }
  }
  Write-Host "Resetting HarborShield to first-run baseline..."
  docker compose down -v --remove-orphans | Out-Host
  docker compose --env-file .env up --build -d | Out-Host
}
