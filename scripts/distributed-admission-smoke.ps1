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

function Wait-ForReplacement {
    param(
        [hashtable]$Headers,
        [string]$Key,
        [int]$MaxAttempts = 12
    )

    for ($attempt = 1; $attempt -le $MaxAttempts; $attempt++) {
        $placements = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/storage/placements?key=$([uri]::EscapeDataString($Key))" -Headers $Headers
        $nodeNames = @($placements.items | ForEach-Object { $_.nodeName })
        if (@($placements.items).Count -eq 3 -and ($nodeNames -contains "blobnode-d") -and ($nodeNames -notcontains "blobnode-c")) {
            return $placements
        }
        Start-Sleep -Seconds 5
    }

    throw "timed out waiting for node replacement"
}

$bucketName = "dist-admit-" + ([guid]::NewGuid().ToString("N").Substring(0, 8))
$objectKey = "admit-" + ([guid]::NewGuid().ToString("N").Substring(0, 8)) + ".txt"
$sourceFile = Join-Path $PSScriptRoot "dist-admit-source.txt"
Set-Content -Path $sourceFile -Value "hello from admission smoke" -NoNewline

$login = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/auth/login" -ContentType "application/json" -Body '{"email":"admin@example.com","password":"change_me_now"}'
$headers = @{ Authorization = "Bearer $($login.accessToken)" }

$credential = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/credentials" -Headers $headers -ContentType "application/json" -Body (@{
    role        = "admin"
    description = "distributed-admission-smoke"
} | ConvertTo-Json -Compress)

$createStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X PUT "http://localhost/s3/$bucketName"
Assert-Status -Actual $createStatus -Expected "200" -Message "admission bucket create failed"

$putStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -T "$sourceFile" "http://localhost/s3/$bucketName/$objectKey"
Assert-Status -Actual $putStatus -Expected "200" -Message "admission put failed"

$registeredNode = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/storage/nodes" -Headers $headers -ContentType "application/json" -Body (@{
    name          = "blobnode-d"
    endpoint      = "http://blobnode-d:9100"
    zone          = "default"
    operatorState = "maintenance"
} | ConvertTo-Json -Compress)

if ($registeredNode.operatorState -ne "maintenance") {
    throw "expected blobnode-d to register in maintenance mode"
}

$nodes = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/storage/nodes" -Headers $headers
$drainingNode = @($nodes.items | Where-Object { $_.name -eq "blobnode-c" })[0]
$candidateNode = @($nodes.items | Where-Object { $_.name -eq "blobnode-d" })[0]
if (-not $drainingNode -or -not $candidateNode) {
    throw "expected blobnode-c and blobnode-d nodes to exist"
}

$activatedNode = Invoke-RestMethod -Method Patch -Uri "http://localhost/api/v1/storage/nodes/$($candidateNode.id)" -Headers $headers -ContentType "application/json" -Body '{"operatorState":"active"}'
if ($activatedNode.operatorState -ne "active") {
    throw "expected blobnode-d to become active"
}

$drainedNode = Invoke-RestMethod -Method Patch -Uri "http://localhost/api/v1/storage/nodes/$($drainingNode.id)" -Headers $headers -ContentType "application/json" -Body '{"operatorState":"draining"}'
if ($drainedNode.operatorState -ne "draining") {
    throw "expected blobnode-c to become draining"
}

$replaced = Wait-ForReplacement -Headers $headers -Key $objectKey
$blobD = docker compose exec -T blobnode-d sh -lc "find /blobdata -type f | wc -l"

[pscustomobject]@{
    bucket         = $bucketName
    key            = $objectKey
    placementNodes = @($replaced.items | ForEach-Object { $_.nodeName })
    blobnodeDFiles = [int]$blobD
} | ConvertTo-Json -Compress

Remove-Item -Path $sourceFile -ErrorAction SilentlyContinue
