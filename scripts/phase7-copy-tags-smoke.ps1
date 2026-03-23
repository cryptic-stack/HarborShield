$ErrorActionPreference = "Stop"

$sourceFile = Join-Path $PSScriptRoot "phase7-copy-source.txt"
"copy and tag payload" | Set-Content -Path $sourceFile -NoNewline

Push-Location (Join-Path $PSScriptRoot "..")
try {
    $loginBody = @{ email = "admin@example.com"; password = "change_me_now" } | ConvertTo-Json
    $login = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/auth/login" -ContentType "application/json" -Body $loginBody
    $token = $login.accessToken
    $headers = @{ Authorization = "Bearer $token" }

    $bucketName = "phase7-copy-tags-" + ([guid]::NewGuid().ToString("N").Substring(0, 8))
    $bucket = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/buckets" -Headers $headers -ContentType "application/json" -Body (@{ name = $bucketName } | ConvertTo-Json)
    $bucketId = $bucket.id

    $credentialBody = @{
        userId = $login.user.id
        role = "admin"
        description = "phase7-copy-tags-smoke"
    } | ConvertTo-Json
    $credential = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/credentials" -Headers $headers -ContentType "application/json" -Body $credentialBody

    $putStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X PUT -H "x-amz-tagging: team=platform&classification=internal" --data-binary "@$sourceFile" "http://localhost/s3/$bucketName/source.txt"
    if ($putStatus -ne "200") { throw "expected source upload 200, got $putStatus" }

    $sourceTagsFile = Join-Path $PSScriptRoot "phase7-source-tags.xml"
    $tagStatus = curl.exe -sS -o $sourceTagsFile -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" "http://localhost/s3/$bucketName/source.txt?tagging"
    if ($tagStatus -ne "200") { throw "expected source tagging fetch 200, got $tagStatus" }
    $sourceTags = Get-Content -Path $sourceTagsFile -Raw
    if ($sourceTags -notmatch "<Key>team</Key>" -or $sourceTags -notmatch "<Value>platform</Value>") { throw "expected source tags in tagging response" }

    $copyStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X PUT -H "x-amz-copy-source: /$bucketName/source.txt" "http://localhost/s3/$bucketName/copied.txt"
    if ($copyStatus -ne "200") { throw "expected copy object 200, got $copyStatus" }

    $copyBodyFile = Join-Path $PSScriptRoot "phase7-copied.txt"
    $copyGetStatus = curl.exe -sS -o $copyBodyFile -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" "http://localhost/s3/$bucketName/copied.txt"
    if ($copyGetStatus -ne "200") { throw "expected copied object download 200, got $copyGetStatus" }
    $copyBody = Get-Content -Path $copyBodyFile -Raw
    if ($copyBody -ne "copy and tag payload") { throw "expected copied object body to match source" }

    $copiedTagsFile = Join-Path $PSScriptRoot "phase7-copied-tags.xml"
    $copiedTagStatus = curl.exe -sS -o $copiedTagsFile -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" "http://localhost/s3/$bucketName/copied.txt?tagging"
    if ($copiedTagStatus -ne "200") { throw "expected copied tagging fetch 200, got $copiedTagStatus" }
    $copiedTags = Get-Content -Path $copiedTagsFile -Raw
    if ($copiedTags -notmatch "<Key>classification</Key>" -or $copiedTags -notmatch "<Value>internal</Value>") { throw "expected copied tags to inherit source tags" }

    $taggingBodyFile = Join-Path $PSScriptRoot "phase7-put-tags.xml"
    @"
<Tagging>
  <TagSet>
    <Tag><Key>team</Key><Value>platform</Value></Tag>
    <Tag><Key>lifecycle</Key><Value>archival</Value></Tag>
  </TagSet>
</Tagging>
"@ | Set-Content -Path $taggingBodyFile -NoNewline

    $putTagStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X PUT -H "Content-Type: application/xml" --data-binary "@$taggingBodyFile" "http://localhost/s3/$bucketName/copied.txt?tagging"
    if ($putTagStatus -ne "200") { throw "expected put object tagging 200, got $putTagStatus" }

    $updatedTagsFile = Join-Path $PSScriptRoot "phase7-updated-tags.xml"
    $updatedTagStatus = curl.exe -sS -o $updatedTagsFile -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" "http://localhost/s3/$bucketName/copied.txt?tagging"
    if ($updatedTagStatus -ne "200") { throw "expected updated tagging fetch 200, got $updatedTagStatus" }
    $updatedTags = Get-Content -Path $updatedTagsFile -Raw
    if ($updatedTags -notmatch "<Key>lifecycle</Key>" -or $updatedTags -notmatch "<Value>archival</Value>") { throw "expected updated tags to include lifecycle=archival" }

    $uiStatus = curl.exe -sS -o NUL -w "%{http_code}" "http://localhost/buckets/$bucketId"
    if ($uiStatus -ne "200") { throw "expected bucket detail UI 200, got $uiStatus" }

    [pscustomobject]@{
        bucketName = $bucketName
        copiedObject = "copied.txt"
        inheritedTags = $true
        updatedTagStatus = $updatedTagStatus
        uiStatus = $uiStatus
    } | ConvertTo-Json -Compress
}
finally {
    Remove-Item -Path $sourceFile -ErrorAction SilentlyContinue
    Remove-Item -Path (Join-Path $PSScriptRoot "phase7-source-tags.xml") -ErrorAction SilentlyContinue
    Remove-Item -Path (Join-Path $PSScriptRoot "phase7-copied-tags.xml") -ErrorAction SilentlyContinue
    Remove-Item -Path (Join-Path $PSScriptRoot "phase7-put-tags.xml") -ErrorAction SilentlyContinue
    Remove-Item -Path (Join-Path $PSScriptRoot "phase7-updated-tags.xml") -ErrorAction SilentlyContinue
    Remove-Item -Path (Join-Path $PSScriptRoot "phase7-copied.txt") -ErrorAction SilentlyContinue
    Pop-Location
}
