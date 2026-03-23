$ErrorActionPreference = "Stop"

$uploadFile = "c:\Users\JBrown\Documents\Project\s3-platform\scripts\phase5-quota-upload.txt"
"12345678901234567890" | Set-Content -Path $uploadFile -NoNewline

$loginBody = @{ email = "admin@example.com"; password = "change_me_now" } | ConvertTo-Json
$login = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/auth/login" -ContentType "application/json" -Body $loginBody
$token = $login.accessToken
$headers = @{ Authorization = "Bearer $token" }

$users = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/users" -Headers $headers
$adminUser = @($users.items | Where-Object { $_.email -eq "admin@example.com" })[0]
if (-not $adminUser) { throw "expected admin user" }

$quotaSnapshot = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/quotas" -Headers $headers
$existingUserQuota = @($quotaSnapshot.userItems | Where-Object { $_.userId -eq $adminUser.id })[0]
$userCurrentBytes = 0
if ($existingUserQuota) {
    $userCurrentBytes = [int64]$existingUserQuota.currentBytes
}

$bucketName = "phase5-quota-" + ([guid]::NewGuid().ToString("N").Substring(0, 8))
$bucket = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/buckets" -Headers $headers -ContentType "application/json" -Body (@{ name = $bucketName } | ConvertTo-Json)
$bucketId = $bucket.id

$bucketQuotaBody = @{
    maxBytes = 30
    maxObjects = 1
    warningThresholdPercent = 50
} | ConvertTo-Json
Invoke-RestMethod -Method Put -Uri "http://localhost/api/v1/quotas/buckets/$bucketId" -Headers $headers -ContentType "application/json" -Body $bucketQuotaBody | Out-Null

$userQuotaBody = @{
    maxBytes = ($userCurrentBytes + 30)
    warningThresholdPercent = 50
} | ConvertTo-Json
Invoke-RestMethod -Method Put -Uri "http://localhost/api/v1/quotas/users/$($adminUser.id)" -Headers $headers -ContentType "application/json" -Body $userQuotaBody | Out-Null

$credentialBody = @{
    userId = $adminUser.id
    role = "admin"
    description = "phase5-quota-smoke"
} | ConvertTo-Json
$credential = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/credentials" -Headers $headers -ContentType "application/json" -Body $credentialBody

$adminUploadHeadersFile = "c:\Users\JBrown\Documents\Project\s3-platform\scripts\phase5-admin-upload-headers.txt"
$adminUploadStatus = curl.exe -sS -D $adminUploadHeadersFile -o NUL -w "%{http_code}" -X POST "http://localhost/api/v1/buckets/$bucketId/objects/upload" -H "Authorization: Bearer $token" -F "key=first.txt" -F "file=@$uploadFile;type=text/plain"
if ($adminUploadStatus -ne "201") { throw "unexpected admin upload status: $adminUploadStatus" }

$adminUploadHeaders = Get-Content -Path $adminUploadHeadersFile -Raw
if ($adminUploadHeaders -notmatch "X-S3P-Quota-Warning-Bucket-Bytes: true") { throw "expected bucket-bytes warning header" }
if ($adminUploadHeaders -notmatch "X-S3P-Quota-Warning-Bucket-Objects: true") { throw "expected bucket-objects warning header" }
if ($adminUploadHeaders -notmatch "X-S3P-Quota-Warning-User-Bytes: true") { throw "expected user-bytes warning header" }

$s3ResponseFile = "c:\Users\JBrown\Documents\Project\s3-platform\scripts\phase5-s3-quota-denial.xml"
$s3Status = curl.exe -sS -o $s3ResponseFile -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X PUT --data-binary "@$uploadFile" "http://localhost/s3/$bucketName/second.txt"
if ($s3Status -ne "409") { throw "expected quota denial status 409, got $s3Status" }

$s3Response = Get-Content -Path $s3ResponseFile -Raw
if ($s3Response -notmatch "<Code>QuotaExceeded</Code>") { throw "expected QuotaExceeded response code" }

$quotaRows = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/quotas" -Headers $headers
$bucketQuota = @($quotaRows.bucketItems | Where-Object { $_.bucketId -eq $bucketId })[0]
if (-not $bucketQuota) { throw "expected bucket quota row" }

[pscustomobject]@{
    bucketId = $bucketId
    warningBucketBytes = $true
    warningBucketObjects = $true
    warningUserBytes = $true
    s3QuotaDenialStatus = $s3Status
    currentObjects = $bucketQuota.currentObjects
} | ConvertTo-Json -Compress
