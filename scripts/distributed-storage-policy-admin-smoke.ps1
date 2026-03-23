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

$bucketName = New-RandomName "dist-policy-admin"
$objectKey = "policy-admin-" + ([guid]::NewGuid().ToString("N").Substring(0, 8)) + ".txt"
$sourceFile = Join-Path $PSScriptRoot "dist-policy-admin-source.txt"
Set-Content -Path $sourceFile -Value "hello from storage policy admin smoke" -NoNewline

$login = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/auth/login" -ContentType "application/json" -Body '{"email":"admin@example.com","password":"change_me_now"}'
$headers = @{ Authorization = "Bearer $($login.accessToken)" }

$policy = Invoke-RestMethod -Method Patch -Uri "http://localhost/api/v1/settings/storage-policy" -Headers $headers -ContentType "application/json" -Body (@{
    defaultStorageClass = "reduced-redundancy"
    standardReplicas = 3
    reducedRedundancyReplicas = 2
    archiveReadyReplicas = 1
} | ConvertTo-Json -Compress)

if ($policy.defaultClass -ne "reduced-redundancy") {
    throw "expected default class to change to reduced-redundancy"
}

$bucket = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/buckets" -Headers $headers -ContentType "application/json" -Body (@{
    name = $bucketName
} | ConvertTo-Json -Compress)

if ($bucket.storageClass -ne "inherit") {
    throw "expected inherited bucket storage class"
}
if ($bucket.effectiveStorageClass -ne "reduced-redundancy") {
    throw "expected inherited bucket to use updated cluster default class"
}
if ($bucket.effectiveReplicaTarget -ne 2) {
    throw "expected inherited bucket to use updated default replica target"
}

$credential = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/credentials" -Headers $headers -ContentType "application/json" -Body (@{
    role        = "admin"
    description = "distributed-storage-policy-admin-smoke"
} | ConvertTo-Json -Compress)

$putStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -T "$sourceFile" "http://localhost/s3/$bucketName/$objectKey"
Assert-Status -Actual $putStatus -Expected "200" -Message "policy-updated inherited bucket put failed"

$placements = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/storage/placements?key=$([uri]::EscapeDataString($objectKey))" -Headers $headers
if (@($placements.items).Count -ne 2) {
    throw "expected inherited bucket under updated policy to write 2 placements"
}

[pscustomobject]@{
    bucket = $bucketName
    key = $objectKey
    defaultClass = $policy.defaultClass
    placements = @($placements.items).Count
} | ConvertTo-Json -Compress

Remove-Item -Path $sourceFile -ErrorAction SilentlyContinue
