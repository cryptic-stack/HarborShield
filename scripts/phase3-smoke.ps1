$ErrorActionPreference = "Stop"

$tempUpload = Join-Path $env:TEMP "harborshield-upload.txt"
$tempDownload = Join-Path $env:TEMP "harborshield-download.txt"
"hello from admin upload flow" | Set-Content -Path $tempUpload -NoNewline

$loginBody = @{ email = "admin@example.com"; password = "change_me_now" } | ConvertTo-Json
$login = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/auth/login" -ContentType "application/json" -Body $loginBody
$token = $login.accessToken
$headers = @{ Authorization = "Bearer $token" }

$bucketName = "phase3-e2e-" + ([guid]::NewGuid().ToString("N").Substring(0, 8))
$bucket = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/buckets" -Headers $headers -ContentType "application/json" -Body (@{ name = $bucketName } | ConvertTo-Json)
$bucketId = $bucket.id

$settings = Invoke-RestMethod -Method Get -Uri "http://localhost/api/v1/settings" -Headers $headers
if (-not $settings.storageEncrypted) { throw "expected storageEncrypted=true" }
if ($settings.defaultTenant -ne "default") { throw "unexpected default tenant: $($settings.defaultTenant)" }

$uploadStatus = curl.exe -sS -o NUL -w "%{http_code}" -X POST "http://localhost/api/v1/buckets/$bucketId/objects/upload" -H "Authorization: Bearer $token" -F "key=notes/admin.txt" -F "file=@$tempUpload;type=text/plain"
if ($uploadStatus -ne "201") { throw "unexpected upload status: $uploadStatus" }

$objectList = Invoke-RestMethod -Method Get -Uri ("http://localhost/api/v1/buckets/{0}/objects" -f $bucketId) -Headers $headers
$items = @($objectList.items)
if ($items.Count -ne 1) { throw "expected 1 object, got $($items.Count)" }

curl.exe -sS -o $tempDownload -H "Authorization: Bearer $token" "http://localhost/api/v1/buckets/$bucketId/objects/download?key=notes%2Fadmin.txt" | Out-Null
$downloaded = Get-Content -Path $tempDownload -Raw
if ($downloaded -ne "hello from admin upload flow") { throw "download mismatch: $downloaded" }

$deleteStatus = curl.exe -sS -o NUL -w "%{http_code}" -X DELETE -H "Authorization: Bearer $token" "http://localhost/api/v1/buckets/$bucketId/objects?key=notes%2Fadmin.txt"
if ($deleteStatus -ne "204") { throw "unexpected delete status: $deleteStatus" }

$objectListAfterDelete = Invoke-RestMethod -Method Get -Uri ("http://localhost/api/v1/buckets/{0}/objects" -f $bucketId) -Headers $headers
$itemsAfterDelete = @($objectListAfterDelete.items)
if ($itemsAfterDelete.Count -ne 0) { throw "expected 0 objects after delete, got $($itemsAfterDelete.Count)" }

$settingsPage = (Invoke-WebRequest -Uri "http://localhost/settings" -UseBasicParsing).StatusCode
$uploadsPage = (Invoke-WebRequest -Uri "http://localhost/uploads" -UseBasicParsing).StatusCode
$bucketPage = (Invoke-WebRequest -Uri ("http://localhost/buckets/{0}" -f $bucketId) -UseBasicParsing).StatusCode
$quotasPage = (Invoke-WebRequest -Uri "http://localhost/quotas" -UseBasicParsing).StatusCode

[pscustomobject]@{
    settingsPage     = $settingsPage
    uploadsPage      = $uploadsPage
    bucketPage       = $bucketPage
    quotasPage       = $quotasPage
    bucketId         = $bucketId
    uploadedKey      = "notes/admin.txt"
    storageEncrypted = $settings.storageEncrypted
    defaultTenant    = $settings.defaultTenant
} | ConvertTo-Json -Compress
