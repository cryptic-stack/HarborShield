param(
  [string]$ProjectRoot = (Split-Path -Parent $PSScriptRoot),
  [string]$BaseUrl = "http://localhost",
  [string]$BootstrapEmail = "admin@example.com",
  [string]$BootstrapPassword = "change_me_now",
  [string]$NewAdminPassword = "S3EdgeSmoke!234",
  [string]$BucketName = ("s3-edge-" + ([guid]::NewGuid().ToString("N").Substring(0, 10)))
)

$ErrorActionPreference = "Stop"

Set-Location $ProjectRoot
. (Join-Path $PSScriptRoot "common.ps1")
$curlCommand = Get-CurlCommand
$nullDevice = Get-NullDevice
$tempDir = Get-TempDir

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

$v1Path = $null
$v2Path = $null
$badPartPath = $null
$part1Path = $null
$part2Path = $null
$versionsFile = $null

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

  Write-Host "Logging in with bootstrap admin..."
  $login = Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/login" -Method Post -ContentType "application/json" -Body (@{
    email = $BootstrapEmail
    password = $BootstrapPassword
  } | ConvertTo-Json -Compress)

  $headers = @{ Authorization = "Bearer $($login.accessToken)" }
  if ($login.mustChangePassword) {
    Write-Host "Rotating bootstrap password for edge smoke..."
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

  Write-Host "Creating edge smoke credential..."
  $credential = Invoke-RestMethod -Uri "$BaseUrl/api/v1/credentials" -Method Post -Headers $headers -ContentType "application/json" -Body (@{
    userId = ""
    role = "admin"
    description = "s3 edge smoke credential"
  } | ConvertTo-Json -Compress)

  $v1Path = Join-Path $tempDir "hs-edge-v1.txt"
  $v2Path = Join-Path $tempDir "hs-edge-v2.txt"
  $badPartPath = Join-Path $tempDir "hs-edge-bad-part.txt"
  $part1Path = Join-Path $tempDir "hs-edge-part1.txt"
  $part2Path = Join-Path $tempDir "hs-edge-part2.txt"
  $versionsFile = Join-Path $tempDir "hs-edge-versions.xml"

  "version one" | Set-Content -Path $v1Path -NoNewline
  "version two" | Set-Content -Path $v2Path -NoNewline
  "digest mismatch body" | Set-Content -Path $badPartPath -NoNewline
  "hello " | Set-Content -Path $part1Path -NoNewline
  "world" | Set-Content -Path $part2Path -NoNewline

  Write-Host "Creating edge smoke bucket..."
  $createBucketStatus = SignedStatus -Method "PUT" -Url "$BaseUrl/s3/$BucketName" -AccessKey $credential.accessKey -SecretKey $credential.secretKey
  Assert-Status -Actual $createBucketStatus -Expected "200" -Message "create bucket failed"

  $bucketList = Invoke-RestMethod -Uri "$BaseUrl/api/v1/buckets" -Headers $headers
  $bucketRecord = @($bucketList.items | Where-Object { $_.name -eq $BucketName })[0]
  if (-not $bucketRecord) { throw "expected admin bucket listing to include edge smoke bucket" }

  Write-Host "Creating versions and delete marker..."
  $putOne = SignedStatus -Method "PUT" -Url "$BaseUrl/s3/$BucketName/versioned.txt" -AccessKey $credential.accessKey -SecretKey $credential.secretKey -ExtraArgs @("-T", $v1Path)
  Assert-Status -Actual $putOne -Expected "200" -Message "put object v1 failed"
  Start-Sleep -Seconds 1
  $putTwo = SignedStatus -Method "PUT" -Url "$BaseUrl/s3/$BucketName/versioned.txt" -AccessKey $credential.accessKey -SecretKey $credential.secretKey -ExtraArgs @("-T", $v2Path)
  Assert-Status -Actual $putTwo -Expected "200" -Message "put object v2 failed"
  $deleteCurrent = SignedStatus -Method "DELETE" -Url "$BaseUrl/s3/$BucketName/versioned.txt" -AccessKey $credential.accessKey -SecretKey $credential.secretKey
  Assert-Status -Actual $deleteCurrent -Expected "204" -Message "delete current object failed"

  $currentGetStatus = SignedStatus -Method "GET" -Url "$BaseUrl/s3/$BucketName/versioned.txt" -AccessKey $credential.accessKey -SecretKey $credential.secretKey
  Assert-Status -Actual $currentGetStatus -Expected "404" -Message "current object should be hidden by delete marker"

  $adminVersions = Invoke-RestMethod -Uri "$BaseUrl/api/v1/buckets/$($bucketRecord.id)/objects/versions?key=versioned.txt" -Headers $headers
  $deleteMarkerItem = @($adminVersions.items | Where-Object { $_.isDeleteMarker })[0]
  if (-not $deleteMarkerItem) { throw "expected a delete marker for versioned.txt" }
  $deleteMarkerVersionId = $deleteMarkerItem.versionId

  $dataVersions = @($adminVersions.items | Where-Object { -not $_.isDeleteMarker })
  if ($dataVersions.Count -lt 2) { throw "expected at least two data versions for versioned.txt" }

  $missingVersionId = [guid]::NewGuid().ToString()
  $missingVersionStatus = SignedStatus -Method "DELETE" -Url "$BaseUrl/s3/$BucketName/versioned.txt?versionId=$missingVersionId" -AccessKey $credential.accessKey -SecretKey $credential.secretKey
  Assert-Status -Actual $missingVersionStatus -Expected "404" -Message "missing version delete should return 404"

  $deleteMarkerStatus = SignedStatus -Method "DELETE" -Url "$BaseUrl/s3/$BucketName/versioned.txt?versionId=$deleteMarkerVersionId" -AccessKey $credential.accessKey -SecretKey $credential.secretKey
  Assert-Status -Actual $deleteMarkerStatus -Expected "204" -Message "delete marker removal failed"

  $restoredPath = Join-Path $tempDir "hs-edge-restored.txt"
  $restoredGetStatus = SignedStatus -Method "GET" -Url "$BaseUrl/s3/$BucketName/versioned.txt" -AccessKey $credential.accessKey -SecretKey $credential.secretKey -OutputFile $restoredPath
  Assert-Status -Actual $restoredGetStatus -Expected "200" -Message "removing delete marker should restore current object"
  if ((Get-Content -Path $restoredPath -Raw) -ne "version two") {
    throw "expected restored current object to return version two"
  }

  $olderVersionId = @($dataVersions | Where-Object { -not $_.isLatest })[-1].versionId
  if (-not $olderVersionId) { throw "expected an older non-latest version id" }
  $olderPath = Join-Path $tempDir "hs-edge-older.txt"
  $olderGetStatus = SignedStatus -Method "GET" -Url "$BaseUrl/s3/$BucketName/versioned.txt?versionId=$olderVersionId" -AccessKey $credential.accessKey -SecretKey $credential.secretKey -OutputFile $olderPath
  Assert-Status -Actual $olderGetStatus -Expected "200" -Message "older version get failed"
  if ((Get-Content -Path $olderPath -Raw) -ne "version one") {
    throw "expected older version to return version one"
  }

  Write-Host "Validating multipart failure cases..."
  $initPath = Join-Path $tempDir "hs-edge-initiate.xml"
  $initStatus = SignedStatus -Method "POST" -Url "$BaseUrl/s3/$BucketName/multi.txt?uploads" -AccessKey $credential.accessKey -SecretKey $credential.secretKey -OutputFile $initPath
  Assert-Status -Actual $initStatus -Expected "200" -Message "multipart initiate failed"
  [xml]$initXml = Get-Content -Path $initPath -Raw
  $uploadId = [string]$initXml.InitiateMultipartUploadResult.UploadId
  if (-not $uploadId) { throw "expected upload id from multipart initiate" }

  $badDigestStatus = SignedStatus -Method "PUT" -Url "$BaseUrl/s3/$BucketName/multi.txt?partNumber=1&uploadId=$uploadId" -AccessKey $credential.accessKey -SecretKey $credential.secretKey -ExtraArgs @("-H", "Content-MD5: AAAAAAAAAAAAAAAAAAAAAA==", "--data-binary", "@$badPartPath")
  Assert-Status -Actual $badDigestStatus -Expected "400" -Message "multipart bad digest should fail"

  $part1Headers = Join-Path $tempDir "hs-edge-part1.headers"
  $part1Status = SignedStatus -Method "PUT" -Url "$BaseUrl/s3/$BucketName/multi.txt?partNumber=1&uploadId=$uploadId" -AccessKey $credential.accessKey -SecretKey $credential.secretKey -OutputFile "NUL" -ExtraArgs @("-D", $part1Headers, "--data-binary", "@$part1Path")
  Assert-Status -Actual $part1Status -Expected "200" -Message "multipart part 1 failed"
  $part1Etag = ((Get-Content -Path $part1Headers | Select-String -Pattern '^ETag:\s*(.+)$').Matches[0].Groups[1].Value).Trim()

  $invalidPartBody = Join-Path $tempDir "hs-edge-invalid-part-order.xml"
@"
<CompleteMultipartUpload>
  <Part><PartNumber>2</PartNumber><ETag>\"missing\"</ETag></Part>
  <Part><PartNumber>1</PartNumber><ETag>$part1Etag</ETag></Part>
</CompleteMultipartUpload>
"@ | Set-Content -Path $invalidPartBody -NoNewline
  $invalidPartOrderResponse = Join-Path $tempDir "hs-edge-invalid-part-order-response.xml"
  $invalidPartOrderStatus = SignedStatus -Method "POST" -Url "$BaseUrl/s3/$BucketName/multi.txt?uploadId=$uploadId" -AccessKey $credential.accessKey -SecretKey $credential.secretKey -OutputFile $invalidPartOrderResponse -ExtraArgs @("--data-binary", "@$invalidPartBody")
  Assert-Status -Actual $invalidPartOrderStatus -Expected "400" -Message "multipart invalid part order should fail"
  if ((Get-Content -Path $invalidPartOrderResponse -Raw) -notmatch "InvalidPartOrder") {
    throw "expected InvalidPartOrder response body"
  }

  $missingPartBody = Join-Path $tempDir "hs-edge-missing-part.xml"
@"
<CompleteMultipartUpload>
  <Part><PartNumber>1</PartNumber><ETag>$part1Etag</ETag></Part>
  <Part><PartNumber>2</PartNumber><ETag>\"missing\"</ETag></Part>
</CompleteMultipartUpload>
"@ | Set-Content -Path $missingPartBody -NoNewline
  $missingPartResponse = Join-Path $tempDir "hs-edge-missing-part-response.xml"
  $missingPartStatus = SignedStatus -Method "POST" -Url "$BaseUrl/s3/$BucketName/multi.txt?uploadId=$uploadId" -AccessKey $credential.accessKey -SecretKey $credential.secretKey -OutputFile $missingPartResponse -ExtraArgs @("--data-binary", "@$missingPartBody")
  Assert-Status -Actual $missingPartStatus -Expected "400" -Message "multipart missing part should fail"
  if ((Get-Content -Path $missingPartResponse -Raw) -notmatch "InvalidPart") {
    throw "expected InvalidPart response body"
  }

  $abortStatus = SignedStatus -Method "DELETE" -Url "$BaseUrl/s3/$BucketName/multi.txt?uploadId=$uploadId" -AccessKey $credential.accessKey -SecretKey $credential.secretKey
  Assert-Status -Actual $abortStatus -Expected "204" -Message "multipart abort failed"

  Write-Host "Cleaning bucket versions..."
  $finalAdminVersions = Invoke-RestMethod -Uri "$BaseUrl/api/v1/buckets/$($bucketRecord.id)/objects/versions?key=versioned.txt" -Headers $headers
  foreach ($marker in @($finalAdminVersions.items | Where-Object { $_.isDeleteMarker })) {
    $status = SignedStatus -Method "DELETE" -Url "$BaseUrl/s3/$BucketName/$($marker.key)?versionId=$($marker.versionId)" -AccessKey $credential.accessKey -SecretKey $credential.secretKey
    Assert-Status -Actual $status -Expected "204" -Message "delete marker cleanup failed"
  }
  foreach ($version in @($finalAdminVersions.items | Where-Object { -not $_.isDeleteMarker })) {
    $status = SignedStatus -Method "DELETE" -Url "$BaseUrl/s3/$BucketName/$($version.key)?versionId=$($version.versionId)" -AccessKey $credential.accessKey -SecretKey $credential.secretKey
    Assert-Status -Actual $status -Expected "204" -Message "version cleanup failed"
  }
  $deleteBucketStatus = SignedStatus -Method "DELETE" -Url "$BaseUrl/s3/$BucketName" -AccessKey $credential.accessKey -SecretKey $credential.secretKey
  Assert-Status -Actual $deleteBucketStatus -Expected "204" -Message "edge smoke bucket delete failed"

  [pscustomobject]@{
    bucket = $BucketName
    deleteMarkerFlow = $true
    noSuchVersion = $true
    restoredCurrentObject = $true
    multipartBadDigest = $true
    multipartInvalidPart = $true
    multipartInvalidPartOrder = $true
    cleanedBucket = $true
  } | ConvertTo-Json -Compress
}
finally {
  $tempFiles = @(
    $v1Path, $v2Path, $badPartPath, $part1Path, $part2Path, $versionsFile,
    (Join-Path $tempDir "hs-edge-restored.txt"),
    (Join-Path $tempDir "hs-edge-older.txt"),
    (Join-Path $tempDir "hs-edge-initiate.xml"),
    (Join-Path $tempDir "hs-edge-part1.headers"),
    (Join-Path $tempDir "hs-edge-invalid-part-order.xml"),
    (Join-Path $tempDir "hs-edge-invalid-part-order-response.xml"),
    (Join-Path $tempDir "hs-edge-missing-part.xml"),
    (Join-Path $tempDir "hs-edge-missing-part-response.xml")
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
