$ErrorActionPreference = "Stop"

function Wait-Until {
    param(
        [scriptblock]$Condition,
        [string]$Description,
        [int]$TimeoutSeconds = 45,
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

$receiverPort = 19091
$receiverLog = "c:\Users\JBrown\Documents\Project\s3-platform\scripts\phase5-event-receiver.log"
if (Test-Path $receiverLog) { Remove-Item $receiverLog -Force }

Push-Location (Join-Path $PSScriptRoot "..")
try {
    docker compose --profile smoke up -d webhook-receiver | Out-Null
    Start-Sleep -Seconds 3

    $loginBody = @{ email = "admin@example.com"; password = "change_me_now" } | ConvertTo-Json
    $login = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/auth/login" -ContentType "application/json" -Body $loginBody
    $token = $login.accessToken
    $headers = @{ Authorization = "Bearer $token" }
    docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "TRUNCATE TABLE event_deliveries, event_targets RESTART IDENTITY CASCADE;" | Out-Null

    $successTarget = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/event-targets" -Headers $headers -ContentType "application/json" -Body (@{
        name = "success-webhook"
        endpointUrl = "http://webhook-receiver:$receiverPort/hook"
        signingSecret = "phase5-secret"
        eventTypes = @("bucket.created")
    } | ConvertTo-Json)

    $failureTarget = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/event-targets" -Headers $headers -ContentType "application/json" -Body (@{
        name = "failing-webhook"
        endpointUrl = "http://no-such-webhook:19092/fail"
        signingSecret = ""
        eventTypes = @("bucket.created")
    } | ConvertTo-Json)

    $bucketName = "phase5-events-" + ([guid]::NewGuid().ToString("N").Substring(0, 8))
    Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/buckets" -Headers $headers -ContentType "application/json" -Body (@{ name = $bucketName } | ConvertTo-Json) | Out-Null

    for ($i = 0; $i -lt 8; $i++) {
        docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "UPDATE jobs SET next_run_at = NOW() - INTERVAL '1 second', status = 'pending' WHERE job_type = 'event_delivery_retry';" | Out-Null
        docker compose exec -T postgres psql -q -U s3platform -d s3platform -t -A -c "UPDATE event_deliveries SET next_attempt_at = NOW() - INTERVAL '1 second' WHERE status = 'retrying';" | Out-Null
        docker compose restart worker | Out-Null
        Start-Sleep -Seconds 3
    }

    $deliveries = $null
    Wait-Until -Description "event deliveries to settle" -Condition {
        $script:deliveries = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/event-deliveries?limit=20" -Headers $headers
        $success = @($script:deliveries.items | Where-Object { $_.targetName -eq "success-webhook" -and $_.status -eq "delivered" })[0]
        $dead = @($script:deliveries.items | Where-Object { $_.targetName -eq "failing-webhook" -and $_.status -eq "dead_letter" })[0]
        return $success -and $dead
    }

    $auditSuccess = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/audit?action=event.delivery.success&limit=20" -Headers $headers
    $auditFailure = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/audit?action=event.delivery.failure&limit=20" -Headers $headers
    $pageStatus = curl.exe -sS -o NUL -w "%{http_code}" "http://localhost/events"

    if ($pageStatus -ne "200") { throw "unexpected events page status: $pageStatus" }
    if (@($auditSuccess.items).Count -lt 1) { throw "expected event delivery success audit rows" }
    if (@($auditFailure.items).Count -lt 1) { throw "expected event delivery failure audit rows" }
    if (-not (Test-Path $receiverLog)) { throw "expected webhook receiver log" }

    $receiverLines = @(Get-Content -Path $receiverLog | Where-Object { $_.Trim() -ne "" })
    if ($receiverLines.Count -lt 1) { throw "expected at least one delivered webhook" }
    $lastRecord = $receiverLines[-1] | ConvertFrom-Json
    if (-not $lastRecord.headers.'X-S3P-Signature') { throw "expected signed webhook header" }

    [pscustomobject]@{
        bucketName = $bucketName
        successTargetId = $successTarget.id
        failureTargetId = $failureTarget.id
        deliveredCount = @($deliveries.items | Where-Object { $_.status -eq "delivered" }).Count
        deadLetterCount = @($deliveries.items | Where-Object { $_.status -eq "dead_letter" }).Count
        eventsPageStatus = $pageStatus
    } | ConvertTo-Json -Compress
}
finally {
    docker compose --profile smoke stop webhook-receiver | Out-Null
    Pop-Location
}
