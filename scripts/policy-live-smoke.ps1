$ErrorActionPreference = "Stop"

$login = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/auth/login" -ContentType "application/json" -Body '{"email":"admin@example.com","password":"change_me_now"}'
$headers = @{ Authorization = "Bearer $($login.accessToken)" }
$bucket = "s3-policy-live-" + ([guid]::NewGuid().ToString("N").Substring(0, 8))

$allowed = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/credentials" -Headers $headers -ContentType "application/json" -Body (@{
    role        = "admin"
    description = "allowed-live"
} | ConvertTo-Json -Compress)

$other = Invoke-RestMethod -Method Post -Uri "http://localhost/api/v1/credentials" -Headers $headers -ContentType "application/json" -Body (@{
    role        = "admin"
    description = "other-live"
} | ConvertTo-Json -Compress)

$putBucket = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($allowed.accessKey):$($allowed.secretKey)" -X PUT "http://localhost/s3/$bucket"
if ($putBucket -ne "200") { throw "bucket create failed: $putBucket" }

$publicFile = Join-Path $env:TEMP "hs-public-live.txt"
$privateFile = Join-Path $env:TEMP "hs-private-live.txt"
Set-Content -Path $publicFile -Value "public data" -NoNewline
Set-Content -Path $privateFile -Value "private data" -NoNewline

$put1 = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($allowed.accessKey):$($allowed.secretKey)" -T "$publicFile" "http://localhost/s3/$bucket/public/visible.txt"
$put2 = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($allowed.accessKey):$($allowed.secretKey)" -T "$privateFile" "http://localhost/s3/$bucket/private/secret.txt"
if ($put1 -ne "200" -or $put2 -ne "200") { throw "object put failed: $put1 / $put2" }

$policyFile = Join-Path $env:TEMP "hs-policy-live.json"
$policy = @"
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowPublicListExceptPrivate",
      "Effect": "Allow",
      "Principal": "*",
      "Action": "s3:ListBucket",
      "Resource": "arn:aws:s3:::$bucket",
      "Condition": {
        "StringNotEquals": {
          "s3:prefix": "private/"
        }
      }
    },
    {
      "Sid": "DenyOthersPrivateRead",
      "Effect": "Deny",
      "NotPrincipal": {
        "AWS": "$($allowed.accessKey)"
      },
      "Action": "s3:GetObject",
      "Resource": "arn:aws:s3:::$bucket/private/*"
    }
  ]
}
"@
Set-Content -Path $policyFile -Value $policy -NoNewline
$putPolicy = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($allowed.accessKey):$($allowed.secretKey)" -X PUT --upload-file "$policyFile" "http://localhost/s3/$bucket`?policy"
if ($putPolicy -ne "204") { throw "put policy failed: $putPolicy" }

$anonListAllowed = curl.exe -sS -o NUL -w "%{http_code}" "http://localhost/s3/${bucket}?list-type=2&prefix=public/"
$anonListDenied = curl.exe -sS -o NUL -w "%{http_code}" "http://localhost/s3/${bucket}?list-type=2&prefix=private/"
$allowedGet = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($allowed.accessKey):$($allowed.secretKey)" "http://localhost/s3/$bucket/private/secret.txt"
$otherGet = curl.exe -sS -o NUL -w "%{http_code}" --aws-sigv4 "aws:amz:us-east-1:s3" --user "$($other.accessKey):$($other.secretKey)" "http://localhost/s3/$bucket/private/secret.txt"

[pscustomobject]@{
    bucket          = $bucket
    anonListAllowed = $anonListAllowed
    anonListDenied  = $anonListDenied
    allowedGet      = $allowedGet
    otherGet        = $otherGet
} | ConvertTo-Json -Compress
