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

function Wait-ForPlacements {
    param(
        [hashtable]$Headers,
        [string]$Key,
        [int]$ExpectedCount = 3,
        [int]$MaxAttempts = 12
    )

    for ($attempt = 1; $attempt -le $MaxAttempts; $attempt++) {
        $placements = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/storage/placements?key=$([uri]::EscapeDataString($Key))" -Headers $Headers
        if (@($placements.items).Count -ge $ExpectedCount) {
            return $placements
        }
        Start-Sleep -Seconds 5
    }

    throw "timed out waiting for $ExpectedCount placement rows for key $Key"
}

$bucketName = "dist-smoke-" + ([guid]::NewGuid().ToString("N").Substring(0, 8))
$objectKey = "demo-" + ([guid]::NewGuid().ToString("N").Substring(0, 8)) + ".txt"
$sourceFile = Join-Path $PSScriptRoot "dist-smoke-source.txt"
Set-Content -Path $sourceFile -Value "hello from distributed mode" -NoNewline

$login = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/auth/login" -ContentType "application/json" -Body '{"email":"admin@example.com","password":"change_me_now"}'
$headers = @{ Authorization = "Bearer $($login.accessToken)" }

$credential = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/credentials" -Headers $headers -ContentType "application/json" -Body (@{
    role        = "admin"
    description = "distributed-storage-smoke"
} | ConvertTo-Json -Compress)

$createStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X PUT "http://localhost/s3/$bucketName"
Assert-Status -Actual $createStatus -Expected "200" -Message "distributed bucket create failed"

$putStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -T "$sourceFile" "http://localhost/s3/$bucketName/$objectKey"
Assert-Status -Actual $putStatus -Expected "200" -Message "distributed put failed"

$nodes = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/storage/nodes" -Headers $headers
$placements = Wait-ForPlacements -Headers $headers -Key $objectKey -ExpectedCount 3
if (@($nodes.items).Count -lt 3) { throw "expected at least 3 configured storage nodes" }
if (@($placements.items).Count -lt 3) { throw "expected at least 3 placement rows" }

$blobA = docker compose exec -T blobnode-a sh -lc "find /blobdata -type f | wc -l"
$blobB = docker compose exec -T blobnode-b sh -lc "find /blobdata -type f | wc -l"
$blobC = docker compose exec -T blobnode-c sh -lc "find /blobdata -type f | wc -l"
if ([int]$blobA -lt 1 -or [int]$blobB -lt 1 -or [int]$blobC -lt 1) {
    throw "expected mirrored blob files on all blob nodes"
}

[pscustomobject]@{
    bucket     = $bucketName
    key        = $objectKey
    nodeCount  = @($nodes.items).Count
    placements = @($placements.items).Count
    blobnodeA  = [int]$blobA
    blobnodeB  = [int]$blobB
    blobnodeC  = [int]$blobC
} | ConvertTo-Json -Compress

Remove-Item -Path $sourceFile -ErrorAction SilentlyContinue
