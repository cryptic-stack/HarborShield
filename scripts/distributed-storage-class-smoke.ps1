$ErrorActionPreference = "Stop"

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

function New-RandomName([string]$prefix) {
    return $prefix + "-" + ([guid]::NewGuid().ToString("N").Substring(0, 8))
}

$inheritBucket = New-RandomName "dist-inherit"
$archiveBucket = New-RandomName "dist-archive"
$overrideBucket = New-RandomName "dist-override"
$inheritKey = "inherit-" + ([guid]::NewGuid().ToString("N").Substring(0, 8)) + ".txt"
$archiveKey = "archive-" + ([guid]::NewGuid().ToString("N").Substring(0, 8)) + ".txt"
$overrideKey = "override-" + ([guid]::NewGuid().ToString("N").Substring(0, 8)) + ".txt"
$sourceFile = Join-Path $PSScriptRoot "dist-storage-class-source.txt"
Set-Content -Path $sourceFile -Value "hello from storage class smoke" -NoNewline

$login = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/auth/login" -ContentType "application/json" -Body '{"email":"admin@example.com","password":"change_me_now"}'
$headers = @{ Authorization = "Bearer $($login.accessToken)" }

$settings = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/settings" -Headers $headers
if ($settings.storageDefaultClass -ne "standard") {
    throw "expected cluster default storage class to be standard"
}

$inheritCreated = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/buckets" -Headers $headers -ContentType "application/json" -Body (@{ name = $inheritBucket } | ConvertTo-Json -Compress)
if ($inheritCreated.storageClass -ne "inherit") {
    throw "expected new buckets to default to inherit"
}
if ($inheritCreated.effectiveStorageClass -ne "standard") {
    throw "expected inherited storage class to resolve to standard"
}
if ($inheritCreated.effectiveReplicaTarget -ne 3) {
    throw "expected inherited replica target to resolve to 3"
}

$archiveCreated = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/buckets" -Headers $headers -ContentType "application/json" -Body (@{
    name          = $archiveBucket
    storageClass  = "archive-ready"
    replicaTarget = 0
} | ConvertTo-Json -Compress)
if ($archiveCreated.effectiveStorageClass -ne "archive-ready") {
    throw "expected archive bucket to resolve archive-ready"
}
if ($archiveCreated.effectiveReplicaTarget -ne 1) {
    throw "expected archive-ready bucket to resolve to one replica"
}

$overrideCreated = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/buckets" -Headers $headers -ContentType "application/json" -Body (@{ name = $overrideBucket } | ConvertTo-Json -Compress)
$overrideUpdated = Invoke-RestMethod -Method Patch -Uri "http://localhost/api/v1/buckets/$($overrideCreated.id)/durability" -Headers $headers -ContentType "application/json" -Body (@{
    storageClass  = "inherit"
    replicaTarget = 2
} | ConvertTo-Json -Compress)
if ($overrideUpdated.effectiveReplicaTarget -ne 2) {
    throw "expected explicit override bucket to resolve to two replicas"
}

$credential = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/credentials" -Headers $headers -ContentType "application/json" -Body (@{
    role        = "admin"
    description = "distributed-storage-class-smoke"
} | ConvertTo-Json -Compress)

$putInherit = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -T "$sourceFile" "http://localhost/s3/$inheritBucket/$inheritKey"
Assert-Status -Actual $putInherit -Expected "200" -Message "inherit bucket put failed"
$putArchive = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -T "$sourceFile" "http://localhost/s3/$archiveBucket/$archiveKey"
Assert-Status -Actual $putArchive -Expected "200" -Message "archive bucket put failed"
$putOverride = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -T "$sourceFile" "http://localhost/s3/$overrideBucket/$overrideKey"
Assert-Status -Actual $putOverride -Expected "200" -Message "override bucket put failed"

$inheritPlacements = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/storage/placements?key=$([uri]::EscapeDataString($inheritKey))" -Headers $headers
$archivePlacements = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/storage/placements?key=$([uri]::EscapeDataString($archiveKey))" -Headers $headers
$overridePlacements = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/storage/placements?key=$([uri]::EscapeDataString($overrideKey))" -Headers $headers

if (@($inheritPlacements.items).Count -ne 3) {
    throw "expected inherited bucket to write 3 placements"
}
if (@($archivePlacements.items).Count -ne 1) {
    throw "expected archive-ready bucket to write 1 placement"
}
if (@($overridePlacements.items).Count -ne 2) {
    throw "expected override bucket to write 2 placements"
}

[pscustomobject]@{
    inheritBucket    = $inheritBucket
    archiveBucket    = $archiveBucket
    overrideBucket   = $overrideBucket
    inheritPlacements = @($inheritPlacements.items).Count
    archivePlacements = @($archivePlacements.items).Count
    overridePlacements = @($overridePlacements.items).Count
} | ConvertTo-Json -Compress

Remove-Item -Path $sourceFile -ErrorAction SilentlyContinue
