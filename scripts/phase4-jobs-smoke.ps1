$ErrorActionPreference = "Stop"

$rows = docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -F '|' -c "SELECT job_type, status, attempts, COALESCE(last_error, ''), next_run_at::text FROM jobs ORDER BY job_type;"
$jobLines = @($rows | Where-Object { $_.Trim() -ne "" })
if ($jobLines.Count -lt 5) { throw "expected recurring jobs to be seeded" }

$before = @{}
foreach ($line in $jobLines) {
    $parts = $line.Split("|")
    $before[$parts[0]] = @{
        status = $parts[1]
        attempts = [int]$parts[2]
        lastError = $parts[3]
        nextRunAt = $parts[4]
    }
}

docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "UPDATE jobs SET next_run_at = NOW() - INTERVAL '1 second' WHERE job_type = 'bucket_quota_recalc';" | Out-Null
Start-Sleep -Seconds 7

$afterLine = docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -F '|' -c "SELECT job_type, status, attempts, COALESCE(last_error, ''), next_run_at::text FROM jobs WHERE job_type = 'bucket_quota_recalc';"
$afterLine = $afterLine.Trim()
if (-not $afterLine) { throw "expected bucket_quota_recalc job row" }
$afterParts = $afterLine.Split("|")
$afterAttempts = [int]$afterParts[2]
if ($afterParts[1] -ne "pending") { throw "expected job status to return to pending" }
if ($afterAttempts -le $before["bucket_quota_recalc"].attempts) { throw "expected job attempts to increase" }
if ($afterParts[3] -ne "") { throw "expected last_error to remain empty" }

[pscustomobject]@{
    seededJobs = $jobLines.Count
    bucketQuotaAttemptsBefore = $before["bucket_quota_recalc"].attempts
    bucketQuotaAttemptsAfter = $afterAttempts
    bucketQuotaStatusAfter = $afterParts[1]
} | ConvertTo-Json -Compress
