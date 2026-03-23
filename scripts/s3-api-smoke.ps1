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

function New-TestFile {
    param(
        [string]$Path,
        [string]$Content
    )
    Set-Content -Path $Path -Value $Content -NoNewline
    return $Path
}

try {
    $root = Split-Path -Parent $PSScriptRoot
    $bucketName = "s3-smoke-" + ([guid]::NewGuid().ToString("N").Substring(0, 8))
    $emptyBucketName = "s3-smoke-empty-" + ([guid]::NewGuid().ToString("N").Substring(0, 8))
    $login = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/auth/login" -ContentType "application/json" -Body '{"email":"admin@example.com","password":"change_me_now"}'
    $headers = @{ Authorization = "Bearer $($login.accessToken)" }

    $credentialBody = @{
        role        = "admin"
        description = "s3-api-smoke"
    } | ConvertTo-Json -Compress
    $credential = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/credentials" -Headers $headers -ContentType "application/json" -Body $credentialBody

    $sourceFile = New-TestFile -Path (Join-Path $PSScriptRoot "s3-smoke-source.txt") -Content "hello from s3 smoke"
    $secondFile = New-TestFile -Path (Join-Path $PSScriptRoot "s3-smoke-second.txt") -Content "second object body"
    $multipartOne = New-TestFile -Path (Join-Path $PSScriptRoot "s3-smoke-multipart-1.txt") -Content "hello "
    $multipartTwo = New-TestFile -Path (Join-Path $PSScriptRoot "s3-smoke-multipart-2.txt") -Content "multipart world"
    $taggingXml = Join-Path $PSScriptRoot "s3-smoke-tagging.xml"
@"
<Tagging>
  <TagSet>
    <Tag><Key>team</Key><Value>platform</Value></Tag>
    <Tag><Key>tier</Key><Value>gold</Value></Tag>
  </TagSet>
</Tagging>
"@ | Set-Content -Path $taggingXml -NoNewline

    $bucketCreateStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X PUT "http://localhost/s3/$bucketName"
    Assert-Status -Actual $bucketCreateStatus -Expected "200" -Message "bucket create failed"
    $emptyBucketCreateStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X PUT "http://localhost/s3/$emptyBucketName"
    Assert-Status -Actual $emptyBucketCreateStatus -Expected "200" -Message "empty bucket create failed"

    $listBucketsFile = Join-Path $PSScriptRoot "s3-smoke-list-buckets.xml"
    $listBucketsStatus = curl.exe -sS -o $listBucketsFile -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" "http://localhost/s3/"
    Assert-Status -Actual $listBucketsStatus -Expected "200" -Message "list buckets failed"
    $listBuckets = Get-Content -Path $listBucketsFile -Raw
    if ($listBuckets -notmatch $bucketName) { throw "list buckets response did not include created bucket" }

    $putStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -T "$sourceFile" "http://localhost/s3/$bucketName/demo.txt"
    Assert-Status -Actual $putStatus -Expected "200" -Message "put object failed"

    $headHeaders = Join-Path $PSScriptRoot "s3-smoke-head.txt"
    $headStatus = curl.exe -sS -D $headHeaders -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -I "http://localhost/s3/$bucketName/demo.txt"
    Assert-Status -Actual $headStatus -Expected "200" -Message "head object failed"
    $headBody = Get-Content -Path $headHeaders -Raw
    if ($headBody -notmatch "ETag:" -or $headBody -notmatch "x-amz-version-id:") { throw "head object did not return expected headers" }

    $getFile = Join-Path $PSScriptRoot "s3-smoke-get.txt"
    $getStatus = curl.exe -sS -o $getFile -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" "http://localhost/s3/$bucketName/demo.txt"
    Assert-Status -Actual $getStatus -Expected "200" -Message "get object failed"
    if ((Get-Content -Path $getFile -Raw) -ne "hello from s3 smoke") { throw "get object returned unexpected payload" }

    $copyHeaders = Join-Path $PSScriptRoot "s3-smoke-copy.xml"
    $copyStatus = curl.exe -sS -o $copyHeaders -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X PUT -H "x-amz-copy-source: /$bucketName/demo.txt" "http://localhost/s3/$bucketName/copied.txt"
    Assert-Status -Actual $copyStatus -Expected "200" -Message "copy object failed"
    $copyBody = Get-Content -Path $copyHeaders -Raw
    if ($copyBody -notmatch "CopyObjectResult") { throw "copy object did not return CopyObjectResult XML" }

    $putTagStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X PUT -H "Content-Type: application/xml" --data-binary "@$taggingXml" "http://localhost/s3/${bucketName}/copied.txt?tagging"
    Assert-Status -Actual $putTagStatus -Expected "200" -Message "put object tagging failed"

    $tagFile = Join-Path $PSScriptRoot "s3-smoke-tags.xml"
    $getTagStatus = curl.exe -sS -o $tagFile -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" "http://localhost/s3/${bucketName}/copied.txt?tagging"
    Assert-Status -Actual $getTagStatus -Expected "200" -Message "get object tagging failed"
    $tagBody = Get-Content -Path $tagFile -Raw
    if ($tagBody -notmatch "<Key>team</Key>" -or $tagBody -notmatch "<Value>gold</Value>") { throw "tagging response missing expected tags" }

    $listObjectsFile = Join-Path $PSScriptRoot "s3-smoke-list-objects.xml"
    $listObjectsStatus = curl.exe -sS -o $listObjectsFile -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" "http://localhost/s3/${bucketName}?list-type=2&max-keys=1"
    Assert-Status -Actual $listObjectsStatus -Expected "200" -Message "list objects failed"
    [xml]$listObjectsXml = Get-Content -Path $listObjectsFile -Raw
    if ($listObjectsXml.ListBucketResult.KeyCount -ne "1") { throw "list objects v2 did not return expected KeyCount" }
    if ($listObjectsXml.ListBucketResult.MaxKeys -ne "1") { throw "list objects v2 did not honor max-keys" }
    if ($listObjectsXml.ListBucketResult.IsTruncated -ne "true") { throw "list objects v2 should have been truncated" }
    $nextToken = [string]$listObjectsXml.ListBucketResult.NextContinuationToken
    if (-not $nextToken) { throw "list objects v2 did not return a continuation token" }

    $listObjectsPage2File = Join-Path $PSScriptRoot "s3-smoke-list-objects-page-2.xml"
    $listObjectsPage2Status = curl.exe -sS -o $listObjectsPage2File -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" "http://localhost/s3/${bucketName}?list-type=2&continuation-token=$nextToken"
    Assert-Status -Actual $listObjectsPage2Status -Expected "200" -Message "list objects continuation failed"
    $listObjectsPage2 = Get-Content -Path $listObjectsPage2File -Raw
    if ($listObjectsPage2 -notmatch "demo.txt" -and $listObjectsPage2 -notmatch "copied.txt") { throw "list objects continuation response missing expected keys" }

    $policyDoc = @{
        Version   = "2012-10-17"
        Statement = @(
            @{
                Sid       = "PublicRead"
                Effect    = "Allow"
                Principal = "*"
                Action    = "s3:GetObject"
                Resource  = "arn:aws:s3:::$bucketName/demo.txt"
            }
        )
    } | ConvertTo-Json -Depth 6
    $policyFile = Join-Path $PSScriptRoot "s3-smoke-policy.json"
    Set-Content -Path $policyFile -Value $policyDoc -NoNewline
    $putPolicyStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X PUT --upload-file "$policyFile" "http://localhost/s3/$bucketName`?policy"
    Assert-Status -Actual $putPolicyStatus -Expected "204" -Message "put bucket policy failed"

    $getPolicyFile = Join-Path $PSScriptRoot "s3-smoke-policy-read.json"
    $getPolicyStatus = curl.exe -sS -o $getPolicyFile -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" "http://localhost/s3/$bucketName`?policy"
    Assert-Status -Actual $getPolicyStatus -Expected "200" -Message "get bucket policy failed"
    $getPolicyBody = Get-Content -Path $getPolicyFile -Raw
    if ($getPolicyBody -notmatch "2012-10-17") { throw "get bucket policy response missing version" }

    $anonPublic = Invoke-WebRequest -Uri "http://localhost/s3/$bucketName/demo.txt" -UseBasicParsing
    if ([int]$anonPublic.StatusCode -ne 200) { throw "public read bucket policy did not allow anonymous get" }

    $presignPutBody = @{ method = "PUT"; path = "/s3/$bucketName/presigned.txt" } | ConvertTo-Json -Compress
    $presignedPut = Invoke-RestMethod -Method Post -Uri "http://localhost/s3/presign" -Headers @{ "X-S3P-Access-Key" = $credential.accessKey; "X-S3P-Secret" = $credential.secretKey } -ContentType "application/json" -Body $presignPutBody
    $presignedPutStatus = curl.exe -sS -o NUL -w "%{http_code}" -X PUT --data-binary "@$secondFile" "$($presignedPut.url)"
    Assert-Status -Actual $presignedPutStatus -Expected "200" -Message "presigned put failed"

    $presignGetBody = @{ method = "GET"; path = "/s3/$bucketName/presigned.txt" } | ConvertTo-Json -Compress
    $presignedGet = Invoke-RestMethod -Method Post -Uri "http://localhost/s3/presign" -Headers @{ "X-S3P-Access-Key" = $credential.accessKey; "X-S3P-Secret" = $credential.secretKey } -ContentType "application/json" -Body $presignGetBody
    $presignedGetFile = Join-Path $PSScriptRoot "s3-smoke-presigned.txt"
    $presignedGetStatus = curl.exe -sS -o $presignedGetFile -w "%{http_code}" "$($presignedGet.url)"
    Assert-Status -Actual $presignedGetStatus -Expected "200" -Message "presigned get failed"
    if ((Get-Content -Path $presignedGetFile -Raw) -ne "second object body") { throw "presigned get returned unexpected payload" }

    $initiateFile = Join-Path $PSScriptRoot "s3-smoke-initiate.xml"
    $initiateStatus = curl.exe -sS -o $initiateFile -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X POST "http://localhost/s3/${bucketName}/multipart.txt?uploads"
    Assert-Status -Actual $initiateStatus -Expected "200" -Message "initiate multipart failed"
    [xml]$initiateXml = Get-Content -Path $initiateFile -Raw
    $uploadId = $initiateXml.InitiateMultipartUploadResult.UploadId
    if (-not $uploadId) { throw "multipart initiate did not return UploadId" }

    $part1Headers = Join-Path $PSScriptRoot "s3-smoke-part1.headers"
    $part1Status = curl.exe -sS -D $part1Headers -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X PUT --data-binary "@$multipartOne" "http://localhost/s3/${bucketName}/multipart.txt?partNumber=1&uploadId=${uploadId}"
    Assert-Status -Actual $part1Status -Expected "200" -Message "multipart part 1 failed"
    $part1Etag = ((Get-Content -Path $part1Headers | Select-String -Pattern '^ETag:\s*(.+)$').Matches[0].Groups[1].Value).Trim()

    $part2Headers = Join-Path $PSScriptRoot "s3-smoke-part2.headers"
    $part2Status = curl.exe -sS -D $part2Headers -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X PUT --data-binary "@$multipartTwo" "http://localhost/s3/${bucketName}/multipart.txt?partNumber=2&uploadId=${uploadId}"
    Assert-Status -Actual $part2Status -Expected "200" -Message "multipart part 2 failed"
    $part2Etag = ((Get-Content -Path $part2Headers | Select-String -Pattern '^ETag:\s*(.+)$').Matches[0].Groups[1].Value).Trim()

    $completeXml = Join-Path $PSScriptRoot "s3-smoke-complete.xml"
@"
<CompleteMultipartUpload>
  <Part><PartNumber>1</PartNumber><ETag>$part1Etag</ETag></Part>
  <Part><PartNumber>2</PartNumber><ETag>$part2Etag</ETag></Part>
</CompleteMultipartUpload>
"@ | Set-Content -Path $completeXml -NoNewline
    $completeResp = Join-Path $PSScriptRoot "s3-smoke-complete-response.xml"
    $completeStatus = curl.exe -sS -o $completeResp -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X POST --data-binary "@$completeXml" "http://localhost/s3/${bucketName}/multipart.txt?uploadId=${uploadId}"
    Assert-Status -Actual $completeStatus -Expected "200" -Message "complete multipart failed"
    $completeBody = Get-Content -Path $completeResp -Raw
    if ($completeBody -notmatch "CompleteMultipartUploadResult") { throw "multipart complete did not return expected xml" }

    $multipartGetFile = Join-Path $PSScriptRoot "s3-smoke-multipart-get.txt"
    $multipartGetStatus = curl.exe -sS -o $multipartGetFile -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" "http://localhost/s3/$bucketName/multipart.txt"
    Assert-Status -Actual $multipartGetStatus -Expected "200" -Message "multipart object get failed"
    if ((Get-Content -Path $multipartGetFile -Raw) -ne "hello multipart world") { throw "multipart object body mismatch" }

    $abortInitFile = Join-Path $PSScriptRoot "s3-smoke-abort-init.xml"
    $abortInitStatus = curl.exe -sS -o $abortInitFile -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X POST "http://localhost/s3/${bucketName}/abort.txt?uploads"
    Assert-Status -Actual $abortInitStatus -Expected "200" -Message "abort multipart initiate failed"
    [xml]$abortXml = Get-Content -Path $abortInitFile -Raw
    $abortUploadId = $abortXml.InitiateMultipartUploadResult.UploadId
    $abortStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X DELETE "http://localhost/s3/${bucketName}/abort.txt?uploadId=${abortUploadId}"
    Assert-Status -Actual $abortStatus -Expected "204" -Message "abort multipart failed"

    $deleteCopiedStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X DELETE "http://localhost/s3/$bucketName/copied.txt"
    Assert-Status -Actual $deleteCopiedStatus -Expected "204" -Message "delete copied object failed"

    $deletePolicyStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X DELETE "http://localhost/s3/$bucketName`?policy"
    Assert-Status -Actual $deletePolicyStatus -Expected "204" -Message "delete bucket policy failed"

    $deleteUsedBucketStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X DELETE "http://localhost/s3/$bucketName"
    Assert-Status -Actual $deleteUsedBucketStatus -Expected "409" -Message "delete non-empty bucket should fail"

    $versionsFile = Join-Path $PSScriptRoot "s3-smoke-versions.xml"
    $versionsStatus = curl.exe -sS -o $versionsFile -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" "http://localhost/s3/${bucketName}?versions"
    Assert-Status -Actual $versionsStatus -Expected "200" -Message "list object versions failed"
    [xml]$versionsXml = Get-Content -Path $versionsFile -Raw
    $versionEntries = @($versionsXml.ListVersionsResult.Version)
    $deleteMarkers = @($versionsXml.ListVersionsResult.DeleteMarker)
    if ($versionEntries.Count -lt 4) { throw "expected multiple object versions in version listing" }
    if ($deleteMarkers.Count -lt 1) { throw "expected at least one delete marker in version listing" }

    foreach ($marker in $deleteMarkers) {
        $markerDeleteStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X DELETE "http://localhost/s3/${bucketName}/$($marker.Key)?versionId=$($marker.VersionId)"
        Assert-Status -Actual $markerDeleteStatus -Expected "204" -Message "delete marker version delete failed"
    }
    foreach ($version in $versionEntries) {
        $versionDeleteStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X DELETE "http://localhost/s3/${bucketName}/$($version.Key)?versionId=$($version.VersionId)"
        Assert-Status -Actual $versionDeleteStatus -Expected "204" -Message "object version delete failed"
    }

    $deleteBucketStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X DELETE "http://localhost/s3/$emptyBucketName"
    Assert-Status -Actual $deleteBucketStatus -Expected "204" -Message "delete empty bucket failed"

    $deleteVersionedBucketStatus = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($credential.accessKey):$($credential.secretKey)" -X DELETE "http://localhost/s3/$bucketName"
    Assert-Status -Actual $deleteVersionedBucketStatus -Expected "204" -Message "delete cleaned versioned bucket failed"

    [pscustomobject]@{
        bucketCreated      = $bucketName
        listBuckets        = $true
        putGetHeadDelete   = $true
        listObjects        = $true
        copyAndTagging     = $true
        bucketPolicy       = $true
        presignedUrls      = $true
        multipart          = $true
        deleteBucketConflict = $true
        versionedDelete     = $true
        bucketDeleted       = $true
    } | ConvertTo-Json -Compress
}
finally {
    Remove-Item -Path (Join-Path $PSScriptRoot "s3-smoke-*") -ErrorAction SilentlyContinue
}
