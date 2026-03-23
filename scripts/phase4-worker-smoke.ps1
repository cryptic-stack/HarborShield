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

$loginBody = @{ email = "admin@example.com"; password = "change_me_now" } | ConvertTo-Json
$login = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/auth/login" -ContentType "application/json" -Body $loginBody
$token = $login.accessToken
$headers = @{ Authorization = "Bearer $token" }

$bucketName = "phase4-worker-" + ([guid]::NewGuid().ToString("N").Substring(0, 8))
$bucket = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/buckets" -Headers $headers -ContentType "application/json" -Body (@{ name = $bucketName } | ConvertTo-Json)
$bucketId = $bucket.id

$uploadId = docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "INSERT INTO multipart_uploads (bucket_id, object_key, expires_at) VALUES ('$bucketId'::uuid, 'expired/worker.bin', NOW() + INTERVAL '10 minutes') RETURNING id;"
$uploadId = $uploadId.Trim()
if (-not $uploadId) { throw "failed to seed expired multipart upload" }

$partPath = "tenants/default/buckets/$bucketId/multipart/$uploadId/1"
docker compose exec -T api sh -lc "mkdir -p /data/tenants/default/buckets/$bucketId/multipart/$uploadId && printf 'stale multipart data' > /data/$partPath" | Out-Null
docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "INSERT INTO multipart_upload_parts (upload_id, part_number, size_bytes, etag, storage_path) VALUES ('$uploadId'::uuid, 1, 19, 'stale', '$partPath');" | Out-Null
docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "UPDATE multipart_uploads SET expires_at = NOW() - INTERVAL '1 minute' WHERE id = '$uploadId'::uuid;" | Out-Null
docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "UPDATE jobs SET next_run_at = NOW() - INTERVAL '1 second', status = 'pending' WHERE job_type = 'multipart_cleanup';" | Out-Null

docker compose restart worker | Out-Null
Wait-Until -Description "expired multipart cleanup" -Condition {
    $remainingProbe = docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "SELECT COUNT(1) FROM multipart_uploads WHERE id = '$uploadId'::uuid;"
    return $remainingProbe.Trim() -eq "0"
}

$remaining = docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "SELECT COUNT(1) FROM multipart_uploads WHERE id = '$uploadId'::uuid;"
$remaining = $remaining.Trim()
if ($remaining -ne "0") { throw "expected expired multipart upload to be cleaned, remaining count: $remaining" }

$fileCheck = docker compose exec -T api sh -lc "if [ -e /data/$partPath ]; then echo present; else echo absent; fi"
$fileCheck = $fileCheck.Trim()
if ($fileCheck -ne "absent") { throw "expected multipart part file to be removed" }

[pscustomobject]@{
    bucketId         = $bucketId
    expiredUploadId  = $uploadId
    remainingUploads = $remaining
    partRemoved      = $true
} | ConvertTo-Json -Compress
