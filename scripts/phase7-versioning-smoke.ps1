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

Add-Type -AssemblyName System.Net.Http

function Upload-Object {
    param(
        [string]$Token,
        [string]$BucketId,
        [string]$Key,
        [string]$Path
    )

    $client = [System.Net.Http.HttpClient]::new()
    $client.DefaultRequestHeaders.Authorization = [System.Net.Http.Headers.AuthenticationHeaderValue]::new("Bearer", $Token)
    $content = [System.Net.Http.MultipartFormDataContent]::new()
    $content.Add([System.Net.Http.StringContent]::new($Key), "key")
    $stream = [System.IO.File]::OpenRead($Path)
    try {
        $fileContent = [System.Net.Http.StreamContent]::new($stream)
        $fileContent.Headers.ContentType = [System.Net.Http.Headers.MediaTypeHeaderValue]::Parse("text/plain")
        $content.Add($fileContent, "file", [System.IO.Path]::GetFileName($Path))
        $response = $client.PostAsync("http://localhost/api/v1/buckets/$BucketId/objects/upload", $content).GetAwaiter().GetResult()
        $body = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
        if (-not $response.IsSuccessStatusCode) {
            throw "upload failed for ${Key}: $($response.StatusCode) $body"
        }
    }
    finally {
        $stream.Dispose()
        $content.Dispose()
        $client.Dispose()
    }
}

Push-Location (Join-Path $PSScriptRoot "..")
try {
    $v1 = Join-Path $PSScriptRoot "phase7-v1.txt"
    $v2 = Join-Path $PSScriptRoot "phase7-v2.txt"
    Set-Content -Path $v1 -Value "version one" -NoNewline
    Set-Content -Path $v2 -Value "version two" -NoNewline

    $login = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/auth/login" -ContentType "application/json" -Body (@{ email = "admin@example.com"; password = "change_me_now" } | ConvertTo-Json)
    $token = $login.accessToken
    $headers = @{ Authorization = "Bearer $token" }

    Invoke-RestMethod -Method Put -Uri "http://localhost/api/v1/quotas/users/$($login.user.id)" -Headers $headers -ContentType "application/json" -Body (@{
        maxBytes = 1073741824
        warningThresholdPercent = 95
    } | ConvertTo-Json) | Out-Null

    $bucketName = "phase7-versioning-" + ([guid]::NewGuid().ToString("N").Substring(0, 8))
    $bucket = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/buckets" -Headers $headers -ContentType "application/json" -Body (@{ name = $bucketName } | ConvertTo-Json)
    $bucketId = $bucket.id

    Upload-Object -Token $token -BucketId $bucketId -Key "doc.txt" -Path $v1
    Start-Sleep -Seconds 1
    Upload-Object -Token $token -BucketId $bucketId -Key "doc.txt" -Path $v2

    $objects = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/buckets/$bucketId/objects" -Headers $headers
    if (@($objects.items | Where-Object { $_.key -eq "doc.txt" }).Count -ne 1) { throw "expected a single current object listing entry" }

    $versions = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/buckets/$bucketId/objects/versions?key=doc.txt" -Headers $headers
    if (@($versions.items).Count -lt 2) { throw "expected at least two versions after overwrite" }
    if ((@($versions.items | Where-Object { -not $_.isDeleteMarker }).Count) -lt 2) { throw "expected two data versions" }

    $download = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/buckets/$bucketId/objects/download?key=doc.txt" -Headers $headers
    if ($download -ne "version two") { throw "expected latest download to return version two" }

    Invoke-RestMethod -Method Delete -Uri "http://localhost/api/v1/buckets/$bucketId/objects?key=doc.txt" -Headers $headers | Out-Null
    $objectsAfterDelete = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/buckets/$bucketId/objects" -Headers $headers
    if (@($objectsAfterDelete.items | Where-Object { $_.key -eq "doc.txt" }).Count -ne 0) { throw "expected current listing to hide delete-marked object" }

    $versionsAfterDelete = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/buckets/$bucketId/objects/versions?key=doc.txt" -Headers $headers
    if ((@($versionsAfterDelete.items | Where-Object { $_.isDeleteMarker }).Count) -lt 1) { throw "expected a delete marker version" }
    $olderVersion = @($versionsAfterDelete.items | Where-Object { -not $_.isDeleteMarker })[-1]
    $oldDownload = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/buckets/$bucketId/objects/download?key=doc.txt&versionId=$($olderVersion.versionId)" -Headers $headers
    if ($oldDownload -ne "version one") { throw "expected older version download to return version one" }

    $missingStatus = curl.exe -sS -o NUL -w "%{http_code}" -H "Authorization: Bearer $token" "http://localhost/api/v1/buckets/$bucketId/objects/download?key=doc.txt"
    if ($missingStatus -ne "404") { throw "expected deleted current version to return 404, got $missingStatus" }

    Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/buckets/$bucketId/objects/restore" -Headers $headers -ContentType "application/json" -Body (@{ key = "doc.txt" } | ConvertTo-Json) | Out-Null
    $restored = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/buckets/$bucketId/objects/download?key=doc.txt" -Headers $headers
    if ($restored -ne "version two") { throw "expected restore to recover latest data version" }

    [pscustomobject]@{
        bucketName = $bucketName
        currentCount = @($objects.items).Count
        versionCount = @($versionsAfterDelete.items).Count
        deleteMarkerCount = @($versionsAfterDelete.items | Where-Object { $_.isDeleteMarker }).Count
        missingStatus = $missingStatus
        restoredContent = $restored
    } | ConvertTo-Json -Compress
}
finally {
    Remove-Item -Path (Join-Path $PSScriptRoot "phase7-v1.txt") -ErrorAction SilentlyContinue
    Remove-Item -Path (Join-Path $PSScriptRoot "phase7-v2.txt") -ErrorAction SilentlyContinue
    Pop-Location
}
