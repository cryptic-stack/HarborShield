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

function Wait-ForDrain {
    param(
        [hashtable]$Headers,
        [string]$Key,
        [int]$MaxAttempts = 12
    )

    for ($attempt = 1; $attempt -le $MaxAttempts; $attempt++) {
        $placements = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/storage/placements?key=$([uri]::EscapeDataString($Key))" -Headers $Headers
        $nodes = @($placements.items | ForEach-Object { $_.nodeName })
        if (@($placements.items).Count -eq 2 -and ($nodes -notcontains "blobnode-c")) {
            return $placements
        }
        Start-Sleep -Seconds 5
    }

    throw "timed out waiting for drained node evacuation"
}

$bucketName = "dist-drain-" + ([guid]::NewGuid().ToString("N").Substring(0, 8))
$objectKey = "drain-" + ([guid]::NewGuid().ToString("N").Substring(0, 8)) + ".txt"
$sourceFile = Join-Path $PSScriptRoot "dist-drain-source.txt"
Set-Content -Path $sourceFile -Value "hello from drain smoke" -NoNewline

$login = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/auth/login" -ContentType "application/json" -Body '{"email":"admin@example.com","password":"change_me_now"}'
$headers = @{ Authorization = "Bearer $($login.accessToken)" }

$credential = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/credentials" -Headers $headers -ContentType "application/json" -Body (@{
    role        = "admin"
    description = "distributed-drain-smoke"
} | ConvertTo-Json -Compress)

$createStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X PUT "http://localhost/s3/$bucketName"
Assert-Status -Actual $createStatus -Expected "200" -Message "drain bucket create failed"

$putStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -T "$sourceFile" "http://localhost/s3/$bucketName/$objectKey"
Assert-Status -Actual $putStatus -Expected "200" -Message "drain put failed"

$nodes = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/storage/nodes" -Headers $headers
$targetNode = @($nodes.items | Where-Object { $_.name -eq "blobnode-c" })[0]
if (-not $targetNode) {
    throw "expected blobnode-c node to exist"
}

$patchedNode = Invoke-RestMethod -Method Patch -Uri "http://localhost/api/v1/storage/nodes/$($targetNode.id)" -Headers $headers -ContentType "application/json" -Body '{"operatorState":"draining"}'
if ($patchedNode.operatorState -ne "draining") {
    throw "expected blobnode-c operator state to become draining"
}

$evacuated = Wait-ForDrain -Headers $headers -Key $objectKey
$blobC = docker compose exec -T blobnode-c sh -lc "find /blobdata -type f | wc -l"

[pscustomobject]@{
    bucket          = $bucketName
    key             = $objectKey
    remainingCopies = @($evacuated.items).Count
    nodeNames       = @($evacuated.items | ForEach-Object { $_.nodeName })
    blobnodeCFiles  = [int]$blobC
} | ConvertTo-Json -Compress

Remove-Item -Path $sourceFile -ErrorAction SilentlyContinue
