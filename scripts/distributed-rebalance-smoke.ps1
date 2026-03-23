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

function Wait-ForRebalance {
    param(
        [hashtable]$Headers,
        [string]$Key,
        [int]$MaxAttempts = 12
    )

    for ($attempt = 1; $attempt -le $MaxAttempts; $attempt++) {
        $placements = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/storage/placements?key=$([uri]::EscapeDataString($Key))" -Headers $Headers
        $states = @($placements.items | ForEach-Object { $_.state })
        if ($states.Count -ge 3 -and ($states | Where-Object { $_ -ne "stored" }).Count -eq 0) {
            return $placements
        }
        Start-Sleep -Seconds 5
    }

    throw "timed out waiting for placement rebalance"
}

$bucketName = "dist-rebalance-" + ([guid]::NewGuid().ToString("N").Substring(0, 8))
$objectKey = "rebalance-" + ([guid]::NewGuid().ToString("N").Substring(0, 8)) + ".txt"
$sourceFile = Join-Path $PSScriptRoot "dist-rebalance-source.txt"
Set-Content -Path $sourceFile -Value "hello from rebalance smoke" -NoNewline

$login = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/auth/login" -ContentType "application/json" -Body '{"email":"admin@example.com","password":"change_me_now"}'
$headers = @{ Authorization = "Bearer $($login.accessToken)" }

$credential = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/credentials" -Headers $headers -ContentType "application/json" -Body (@{
    role        = "admin"
    description = "distributed-rebalance-smoke"
} | ConvertTo-Json -Compress)

$createStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X PUT "http://localhost/s3/$bucketName"
Assert-Status -Actual $createStatus -Expected "200" -Message "rebalance bucket create failed"

$putStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -T "$sourceFile" "http://localhost/s3/$bucketName/$objectKey"
Assert-Status -Actual $putStatus -Expected "200" -Message "rebalance put failed"

$initialPlacements = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/storage/placements?key=$([uri]::EscapeDataString($objectKey))" -Headers $headers
if (@($initialPlacements.items).Count -lt 3) {
    throw "expected at least 3 placement rows before rebalance test"
}

$targetPlacement = @($initialPlacements.items | Where-Object { $_.nodeName -eq "blobnode-c" })[0]
if (-not $targetPlacement) {
    throw "expected a blobnode-c placement to exist before rebalance test"
}

$locator = $targetPlacement.locator
$objectId = $targetPlacement.objectId
$deletedReplica = docker compose exec -T blobnode-c sh -lc "rm -f /blobdata/$locator && test ! -f /blobdata/$locator && echo removed"
if ($deletedReplica.Trim() -ne "removed") {
    throw "failed to remove blobnode-c replica"
}

$deletePlacementSql = "DELETE FROM object_placements WHERE object_id = '$objectId'::uuid AND storage_node_id = (SELECT id FROM storage_nodes WHERE name = 'blobnode-c' LIMIT 1);"
$deletedPlacement = docker compose exec -T postgres psql -U s3platform -d s3platform -Atc "$deletePlacementSql"
if ($LASTEXITCODE -ne 0) {
    throw "failed to remove blobnode-c placement row"
}

$countSql = "SELECT COUNT(*) FROM object_placements WHERE object_id = '$objectId'::uuid;"
$placementCountAfterDelete = docker compose exec -T postgres psql -U s3platform -d s3platform -Atc "$countSql"
if ($LASTEXITCODE -ne 0 -or [int]$placementCountAfterDelete -ge 3) {
    throw "expected placement count to drop in postgres before rebalance"
}

$currentPlacements = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/storage/placements?key=$([uri]::EscapeDataString($objectKey))" -Headers $headers
$rebalancedPlacements = Wait-ForRebalance -Headers $headers -Key $objectKey
$blobC = docker compose exec -T blobnode-c sh -lc "test -f /blobdata/$locator && echo restored"
if ($blobC.Trim() -ne "restored") {
    throw "expected blobnode-c replica to be restored by rebalance"
}

[pscustomobject]@{
    bucket            = $bucketName
    key               = $objectKey
    locator           = $locator
    objectId          = $objectId
    droppedPlacements = @($currentPlacements.items).Count
    finalPlacements   = @($rebalancedPlacements.items).Count
    restoredReplica   = $true
} | ConvertTo-Json -Compress

Remove-Item -Path $sourceFile -ErrorAction SilentlyContinue
