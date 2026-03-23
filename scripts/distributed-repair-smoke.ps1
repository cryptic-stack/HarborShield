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

function Wait-ForRepair {
    param(
        [hashtable]$Headers,
        [string]$Key,
        [string]$Locator,
        [int]$MaxAttempts = 12
    )

    for ($attempt = 1; $attempt -le $MaxAttempts; $attempt++) {
        $placements = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/storage/placements?key=$([uri]::EscapeDataString($Key))" -Headers $Headers
        $states = @($placements.items | ForEach-Object { $_.state })
        $blobB = docker compose exec -T blobnode-b sh -lc "test -f /blobdata/$Locator && echo restored"
        if ($states.Count -ge 3 -and ($states | Where-Object { $_ -ne "stored" }).Count -eq 0 -and $blobB -and $blobB.Trim() -eq "restored") {
            return $placements
        }
        Start-Sleep -Seconds 5
    }

    throw "timed out waiting for repaired placements"
}

$bucketName = "dist-repair-" + ([guid]::NewGuid().ToString("N").Substring(0, 8))
$objectKey = "repair-" + ([guid]::NewGuid().ToString("N").Substring(0, 8)) + ".txt"
$sourceFile = Join-Path $PSScriptRoot "dist-repair-source.txt"
Set-Content -Path $sourceFile -Value "hello from repair smoke" -NoNewline

$login = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/auth/login" -ContentType "application/json" -Body '{"email":"admin@example.com","password":"change_me_now"}'
$headers = @{ Authorization = "Bearer $($login.accessToken)" }

$credential = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/credentials" -Headers $headers -ContentType "application/json" -Body (@{
    role        = "admin"
    description = "distributed-repair-smoke"
} | ConvertTo-Json -Compress)

$createStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X PUT "http://localhost/s3/$bucketName"
Assert-Status -Actual $createStatus -Expected "200" -Message "repair bucket create failed"

$putStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -T "$sourceFile" "http://localhost/s3/$bucketName/$objectKey"
Assert-Status -Actual $putStatus -Expected "200" -Message "repair put failed"

$initialPlacements = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/storage/placements?key=$([uri]::EscapeDataString($objectKey))" -Headers $headers
if (@($initialPlacements.items).Count -lt 3) {
    throw "expected at least 3 placement rows before repair test"
}

$locator = $initialPlacements.items[0].locator
$deletedReplica = docker compose exec -T blobnode-b sh -lc "rm -f /blobdata/$locator && test ! -f /blobdata/$locator && echo removed"
if ($deletedReplica.Trim() -ne "removed") {
    throw "failed to remove blobnode-b replica"
}

$missingReplica = docker compose exec -T blobnode-b sh -lc "test ! -f /blobdata/$locator && echo missing"
if ($missingReplica.Trim() -ne "missing") {
    throw "expected blobnode-b replica to be missing before repair"
}

$repairedPlacements = Wait-ForRepair -Headers $headers -Key $objectKey -Locator $locator
$blobB = docker compose exec -T blobnode-b sh -lc "test -f /blobdata/$locator && echo restored"
if (-not $blobB -or $blobB.Trim() -ne "restored") {
    throw "expected blobnode-b replica to be restored by repair"
}

[pscustomobject]@{
    bucket          = $bucketName
    key             = $objectKey
    locator         = $locator
    missingObserved = $true
    repairedCount   = @($repairedPlacements.items).Count
    restoredReplica = $true
} | ConvertTo-Json -Compress

Remove-Item -Path $sourceFile -ErrorAction SilentlyContinue
