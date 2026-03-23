param(
  [string]$ProjectRoot = (Split-Path -Parent $PSScriptRoot),
  [string]$BaseUrl = "http://localhost",
  [string]$BootstrapEmail = "admin@example.com",
  [string]$BootstrapPassword = "change_me_now",
  [string]$NewAdminPassword = "S3PolicyConditions!234"
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

$policyFile = $null
$publicFile = $null
$privateFile = $null
$publicListFile = $null
$privateListFile = $null
$publicGetFile = $null
$privateGetFile = $null
$bucket = "s3-policy-cond-" + ([guid]::NewGuid().ToString("N").Substring(0, 8))

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
    Write-Host "Rotating bootstrap password for policy-conditions smoke..."
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

  Write-Host "Creating policy-conditions credential..."
  $credential = Invoke-RestMethod -Uri "$BaseUrl/api/v1/credentials" -Method Post -Headers $headers -ContentType "application/json" -Body (@{
    role        = "admin"
    description = "policy conditions smoke"
  } | ConvertTo-Json -Compress)

  Write-Host "Creating policy-conditions bucket..."
  $putBucket = SignedStatus -Method "PUT" -Url "$BaseUrl/s3/$bucket" -AccessKey $credential.accessKey -SecretKey $credential.secretKey
  Assert-Status -Actual $putBucket -Expected "200" -Message "bucket create failed"

  $publicFile = Join-Path $env:TEMP "hs-policy-cond-public.txt"
  $privateFile = Join-Path $env:TEMP "hs-policy-cond-private.txt"
  Set-Content -Path $publicFile -Value "policy public data" -NoNewline
  Set-Content -Path $privateFile -Value "policy private data" -NoNewline

  Write-Host "Uploading test objects..."
  $putPublic = SignedStatus -Method "PUT" -Url "$BaseUrl/s3/$bucket/public/readme.txt" -AccessKey $credential.accessKey -SecretKey $credential.secretKey -ExtraArgs @("-T", $publicFile)
  $putPrivate = SignedStatus -Method "PUT" -Url "$BaseUrl/s3/$bucket/private/secret.txt" -AccessKey $credential.accessKey -SecretKey $credential.secretKey -ExtraArgs @("-T", $privateFile)
  Assert-Status -Actual $putPublic -Expected "200" -Message "public object put failed"
  Assert-Status -Actual $putPrivate -Expected "200" -Message "private object put failed"

  $policyFile = Join-Path $env:TEMP "hs-policy-conditions.json"
  $policy = @"
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowPublicPrefixList",
      "Effect": "Allow",
      "Principal": "*",
      "Action": "s3:ListBucket",
      "Resource": "arn:aws:s3:::$bucket",
      "Condition": {
        "StringEquals": {
          "s3:prefix": "public/"
        }
      }
    },
    {
      "Sid": "AllowPublicObjectRead",
      "Effect": "Allow",
      "Principal": "*",
      "Action": "s3:GetObject",
      "Resource": "arn:aws:s3:::$bucket/public/*"
    }
  ]
}
"@
  Set-Content -Path $policyFile -Value $policy -NoNewline

  Write-Host "Applying bucket policy with condition coverage..."
  $putPolicy = SignedStatus -Method "PUT" -Url "$BaseUrl/s3/${bucket}?policy" -AccessKey $credential.accessKey -SecretKey $credential.secretKey -ExtraArgs @("--upload-file", $policyFile)
  Assert-Status -Actual $putPolicy -Expected "204" -Message "put bucket policy failed"

  Write-Host "Validating anonymous prefix-constrained list..."
  $publicListFile = Join-Path $env:TEMP "hs-policy-cond-public-list.xml"
  $publicList = & $curlCommand -sS -o $publicListFile -w "%{http_code}" "$BaseUrl/s3/${bucket}?list-type=2&prefix=public/"
  Assert-Status -Actual $publicList -Expected "200" -Message "anonymous public prefix list should succeed"
  $publicListBody = Get-Content -Path $publicListFile -Raw
  if ($publicListBody -notmatch "public/readme.txt") {
    throw "public prefix list did not include expected object"
  }
  if ($publicListBody -match "private/secret.txt") {
    throw "public prefix list should not include private object"
  }

  $privateListFile = Join-Path $env:TEMP "hs-policy-cond-private-list.xml"
  $privateList = & $curlCommand -sS -o $privateListFile -w "%{http_code}" "$BaseUrl/s3/${bucket}?list-type=2&prefix=private/"
  Assert-Status -Actual $privateList -Expected "401" -Message "anonymous private prefix list should be denied"
  if ((Get-Content -Path $privateListFile -Raw) -notmatch "AccessDenied") {
    throw "private prefix denial should return AccessDenied"
  }

  Write-Host "Validating anonymous object access..."
  $publicGetFile = Join-Path $env:TEMP "hs-policy-cond-public-get.txt"
  $publicGet = & $curlCommand -sS -o $publicGetFile -w "%{http_code}" "$BaseUrl/s3/$bucket/public/readme.txt"
  Assert-Status -Actual $publicGet -Expected "200" -Message "anonymous public object get should succeed"
  if ((Get-Content -Path $publicGetFile -Raw) -ne "policy public data") {
    throw "anonymous public get returned unexpected body"
  }

  $privateGetFile = Join-Path $env:TEMP "hs-policy-cond-private-get.xml"
  $privateGet = & $curlCommand -sS -o $privateGetFile -w "%{http_code}" "$BaseUrl/s3/$bucket/private/secret.txt"
  Assert-Status -Actual $privateGet -Expected "401" -Message "anonymous private object get should be denied"
  if ((Get-Content -Path $privateGetFile -Raw) -notmatch "AccessDenied") {
    throw "anonymous private get denial should return AccessDenied"
  }

  [pscustomobject]@{
    bucket = $bucket
    prefixConditionList = $true
    publicAnonymousRead = $true
    privateAnonymousDenied = $true
  } | ConvertTo-Json -Compress
}
finally {
  $tempFiles = @(
    $policyFile, $publicFile, $privateFile, $publicListFile, $privateListFile, $publicGetFile, $privateGetFile
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
