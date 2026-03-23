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

$bucketName = "dist-bucket-policy-" + ([guid]::NewGuid().ToString("N").Substring(0, 8))
$objectKey = "policy-" + ([guid]::NewGuid().ToString("N").Substring(0, 8)) + ".txt"
$sourceFile = Join-Path $PSScriptRoot "dist-bucket-policy-source.txt"
Set-Content -Path $sourceFile -Value "hello from bucket durability smoke" -NoNewline

$login = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/auth/login" -ContentType "application/json" -Body '{"email":"admin@example.com","password":"change_me_now"}'
$headers = @{ Authorization = "Bearer $($login.accessToken)" }

$createdBucket = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/buckets" -Headers $headers -ContentType "application/json" -Body (@{ name = $bucketName } | ConvertTo-Json -Compress)
$updatedBucket = Invoke-RestMethod -Method Patch -Uri "http://localhost/api/v1/buckets/$($createdBucket.id)/durability" -Headers $headers -ContentType "application/json" -Body (@{
    storageClass  = "standard"
    replicaTarget = 2
} | ConvertTo-Json -Compress)

if ($updatedBucket.replicaTarget -ne 2) {
    throw "expected bucket replica target to be updated to 2"
}

$credential = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/credentials" -Headers $headers -ContentType "application/json" -Body (@{
    role        = "admin"
    description = "distributed-bucket-durability-smoke"
} | ConvertTo-Json -Compress)

$putStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -T "$sourceFile" "http://localhost/s3/$bucketName/$objectKey"
Assert-Status -Actual $putStatus -Expected "200" -Message "bucket durability put failed"

$placements = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/storage/placements?key=$([uri]::EscapeDataString($objectKey))" -Headers $headers
if (@($placements.items).Count -ne 2) {
    throw "expected exactly 2 placement rows for reduced bucket durability"
}

[pscustomobject]@{
    bucket        = $bucketName
    key           = $objectKey
    storageClass  = $updatedBucket.storageClass
    replicaTarget = $updatedBucket.replicaTarget
    placements    = @($placements.items).Count
    nodeNames     = @($placements.items | ForEach-Object { $_.nodeName })
} | ConvertTo-Json -Compress

Remove-Item -Path $sourceFile -ErrorAction SilentlyContinue
