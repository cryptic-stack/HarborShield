$ErrorActionPreference = "Stop"

function Wait-Until {
    param(
        [scriptblock]$Condition,
        [string]$Description,
        [int]$TimeoutSeconds = 30,
        [int]$IntervalSeconds = 2
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        if (& $Condition) {
            return
        }
        Start-Sleep -Seconds $IntervalSeconds
    }

    throw "timed out waiting for $Description"
}

$tempUpload = Join-Path $env:TEMP "harborshield-lifecycle-upload.txt"
"hello from lifecycle flow" | Set-Content -Path $tempUpload -NoNewline

$loginBody = @{ email = "admin@example.com"; password = "change_me_now" } | ConvertTo-Json
$login = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/auth/login" -ContentType "application/json" -Body $loginBody
$token = $login.accessToken
$headers = @{ Authorization = "Bearer $token" }

$bucketName = "phase4-lifecycle-" + ([guid]::NewGuid().ToString("N").Substring(0, 8))
$bucket = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/buckets" -Headers $headers -ContentType "application/json" -Body (@{ name = $bucketName } | ConvertTo-Json)
$bucketId = $bucket.id

$retainedStatus = curl.exe -sS -o NUL -w "%{http_code}" -X POST "http://localhost/api/v1/buckets/$bucketId/objects/upload" -H "Authorization: Bearer $token" -F "key=retain/object.txt" -F "file=@$tempUpload;type=text/plain"
if ($retainedStatus -ne "201") { throw "unexpected retained upload status: $retainedStatus" }

$lifecycleStatus = curl.exe -sS -o NUL -w "%{http_code}" -X POST "http://localhost/api/v1/buckets/$bucketId/objects/upload" -H "Authorization: Bearer $token" -F "key=lifecycle/object.txt" -F "file=@$tempUpload;type=text/plain"
if ($lifecycleStatus -ne "201") { throw "unexpected lifecycle upload status: $lifecycleStatus" }

docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "UPDATE objects SET retention_until = NOW() + INTERVAL '2 hours' WHERE bucket_id = '$bucketId'::uuid AND object_key = 'retain/object.txt';" | Out-Null
docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "UPDATE objects SET lifecycle_delete_at = NOW() - INTERVAL '2 minutes' WHERE bucket_id = '$bucketId'::uuid AND object_key = 'lifecycle/object.txt';" | Out-Null
docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "UPDATE jobs SET next_run_at = NOW() - INTERVAL '1 second', status = 'pending' WHERE job_type = 'lifecycle_expiration';" | Out-Null

$retainDeleteStatus = curl.exe -sS -o NUL -w "%{http_code}" -X DELETE -H "Authorization: Bearer $token" "http://localhost/api/v1/buckets/$bucketId/objects?key=retain%2Fobject.txt"
if ($retainDeleteStatus -ne "409") { throw "expected retained delete to return 409, got $retainDeleteStatus" }

docker compose restart worker | Out-Null
Wait-Until -Description "lifecycle expiration" -Condition {
    $lifecycleProbe = docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "SELECT CASE WHEN deleted_at IS NULL THEN 'active' ELSE 'deleted' END FROM objects WHERE bucket_id = '$bucketId'::uuid AND object_key = 'lifecycle/object.txt';"
    return $lifecycleProbe.Trim() -eq "deleted"
}

$lifecycleDeleted = docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "SELECT CASE WHEN deleted_at IS NULL THEN 'active' ELSE 'deleted' END FROM objects WHERE bucket_id = '$bucketId'::uuid AND object_key = 'lifecycle/object.txt';"
$lifecycleDeleted = $lifecycleDeleted.Trim()
if ($lifecycleDeleted -ne "deleted") { throw "expected lifecycle object to be soft deleted" }

$retainedDeleted = docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "SELECT CASE WHEN deleted_at IS NULL THEN 'active' ELSE 'deleted' END FROM objects WHERE bucket_id = '$bucketId'::uuid AND object_key = 'retain/object.txt';"
$retainedDeleted = $retainedDeleted.Trim()
if ($retainedDeleted -ne "active") { throw "expected retained object to remain active" }

[pscustomobject]@{
    bucketId            = $bucketId
    retainedDeleteCode  = $retainDeleteStatus
    lifecycleState      = $lifecycleDeleted
    retainedObjectState = $retainedDeleted
} | ConvertTo-Json -Compress
