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

$tempUpload = Join-Path $env:TEMP "harborshield-governance-upload.txt"
"hello from governance flow" | Set-Content -Path $tempUpload -NoNewline

$loginBody = @{ email = "admin@example.com"; password = "change_me_now" } | ConvertTo-Json
$login = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/auth/login" -ContentType "application/json" -Body $loginBody
$token = $login.accessToken
$headers = @{ Authorization = "Bearer $token" }

$bucketName = "phase4-governance-" + ([guid]::NewGuid().ToString("N").Substring(0, 8))
$bucket = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/buckets" -Headers $headers -ContentType "application/json" -Body (@{ name = $bucketName } | ConvertTo-Json)
$bucketId = $bucket.id

$uploadStatus = curl.exe -sS -o NUL -w "%{http_code}" -X POST "http://localhost/api/v1/buckets/$bucketId/objects/upload" -H "Authorization: Bearer $token" -F "key=retained/object.txt" -F "file=@$tempUpload;type=text/plain"
if ($uploadStatus -ne "201") { throw "unexpected upload status: $uploadStatus" }

docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "UPDATE jobs SET next_run_at = NOW() - INTERVAL '1 second', status = 'pending' WHERE job_type = 'bucket_quota_recalc';" | Out-Null
docker compose restart worker | Out-Null
Wait-Until -Description "quota recalculation after upload" -Condition {
    $quotaProbe = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/quotas" -Headers $headers
    $bucketQuotaProbe = @($quotaProbe.bucketItems | Where-Object { $_.bucketId -eq $bucketId })[0]
    return $bucketQuotaProbe -and $bucketQuotaProbe.currentObjects -ge 1 -and $bucketQuotaProbe.currentBytes -ge 1 -and $bucketQuotaProbe.recalculatedAt
}

$quotaRows = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/quotas" -Headers $headers
$bucketQuota = @($quotaRows.bucketItems | Where-Object { $_.bucketId -eq $bucketId })[0]
if (-not $bucketQuota) { throw "expected bucket quota row after recalculation" }
if ($bucketQuota.currentObjects -lt 1) { throw "expected currentObjects >= 1 after upload" }
if ($bucketQuota.currentBytes -lt 1) { throw "expected currentBytes >= 1 after upload" }
if (-not $bucketQuota.recalculatedAt) { throw "expected recalculatedAt after upload" }

$deleteStatus = curl.exe -sS -o NUL -w "%{http_code}" -X DELETE -H "Authorization: Bearer $token" "http://localhost/api/v1/buckets/$bucketId/objects?key=retained%2Fobject.txt"
if ($deleteStatus -ne "204") { throw "unexpected delete status: $deleteStatus" }

$objectRow = docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "SELECT id::text || '|' || storage_path FROM objects WHERE bucket_id = '$bucketId'::uuid AND object_key = 'retained/object.txt';"
$objectRow = $objectRow.Trim()
if (-not $objectRow) { throw "expected deleted object row to remain for purge" }
$parts = $objectRow.Split("|")
$objectId = $parts[0]
$storagePath = $parts[1]

docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "UPDATE objects SET deleted_at = NOW() - INTERVAL '25 hours' WHERE id = '$objectId'::uuid;" | Out-Null
docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "UPDATE jobs SET next_run_at = NOW() - INTERVAL '1 second', status = 'pending' WHERE job_type IN ('soft_delete_purge', 'bucket_quota_recalc');" | Out-Null
docker compose restart worker | Out-Null
Wait-Until -Description "soft-deleted object purge" -Condition {
    $remainingProbe = docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "SELECT COUNT(1) FROM objects WHERE id = '$objectId'::uuid;"
    return $remainingProbe.Trim() -eq "0"
}

$remainingObjects = docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "SELECT COUNT(1) FROM objects WHERE id = '$objectId'::uuid;"
$remainingObjects = $remainingObjects.Trim()
if ($remainingObjects -ne "0") { throw "expected soft-deleted object row to be purged" }

$blobState = docker compose exec -T api sh -lc "if [ -e /data/$storagePath ]; then echo present; else echo absent; fi"
$blobState = $blobState.Trim()
if ($blobState -ne "absent") { throw "expected blob file to be purged" }

$quotaRowsAfterPurge = $null
Wait-Until -Description "quota recalculation after purge" -Condition {
    $script:quotaRowsAfterPurge = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/quotas" -Headers $headers
    $bucketQuotaProbeAfterPurge = @($script:quotaRowsAfterPurge.bucketItems | Where-Object { $_.bucketId -eq $bucketId })[0]
    return $bucketQuotaProbeAfterPurge -and $bucketQuotaProbeAfterPurge.currentObjects -eq 0
}

$bucketQuotaAfterPurge = @($quotaRowsAfterPurge.bucketItems | Where-Object { $_.bucketId -eq $bucketId })[0]
if ($bucketQuotaAfterPurge.currentObjects -ne 0) { throw "expected currentObjects to return to 0 after purge" }

[pscustomobject]@{
    bucketId               = $bucketId
    quotaObjectsAfterPut   = $bucketQuota.currentObjects
    quotaBytesAfterPut     = $bucketQuota.currentBytes
    quotaObjectsAfterPurge = $bucketQuotaAfterPurge.currentObjects
    purgedObjectId         = $objectId
    blobRemoved            = $true
} | ConvertTo-Json -Compress
