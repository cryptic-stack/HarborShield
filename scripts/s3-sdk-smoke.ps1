param(
  [string]$ProjectRoot = (Split-Path -Parent $PSScriptRoot),
  [string]$BaseUrl = "http://localhost",
  [string]$BootstrapEmail = "admin@example.com",
  [string]$BootstrapPassword = "change_me_now",
  [string]$NewAdminPassword = "SdkSmoke!234",
  [string]$BucketName = ("sdk-smoke-" + ([guid]::NewGuid().ToString("N").Substring(0, 10)))
)

$ErrorActionPreference = "Stop"

Set-Location $ProjectRoot

$payloadPath = $null
$downloadPath = $null

try {
  Write-Host "Waiting for HarborShield health..."
  $deadline = (Get-Date).AddMinutes(5)
  do {
    try {
      $health = Invoke-RestMethod -Uri "$BaseUrl/healthz"
      if ($health.status -eq "ok") {
        break
      }
    } catch {
    }
    Start-Sleep -Seconds 3
  } while ((Get-Date) -lt $deadline)

  if (-not $health -or $health.status -ne "ok") {
    throw "HarborShield did not become healthy within timeout."
  }

  Write-Host "Logging in with bootstrap admin..."
  $login = Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/login" -Method Post -ContentType "application/json" -Body (@{
    email = $BootstrapEmail
    password = $BootstrapPassword
  } | ConvertTo-Json -Compress)

  $headers = @{ Authorization = "Bearer $($login.accessToken)" }

  if ($login.mustChangePassword) {
    Write-Host "Rotating bootstrap password for smoke..."
    Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/change-password" -Method Post -Headers $headers -ContentType "application/json" -Body (@{
      currentPassword = $BootstrapPassword
      newPassword = $NewAdminPassword
    } | ConvertTo-Json -Compress) | Out-Null

    $login = Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/login" -Method Post -ContentType "application/json" -Body (@{
      email = $BootstrapEmail
      password = $NewAdminPassword
    } | ConvertTo-Json -Compress)

    $headers = @{ Authorization = "Bearer $($login.accessToken)" }
  }

  Write-Host "Creating SDK smoke credential..."
  $credential = Invoke-RestMethod -Uri "$BaseUrl/api/v1/credentials" -Method Post -Headers $headers -ContentType "application/json" -Body (@{
    userId = ""
    role = "admin"
    description = "sdk smoke credential"
  } | ConvertTo-Json -Compress)

  $payloadPath = Join-Path $env:TEMP "harborshield-sdk-smoke.txt"
  $downloadPath = Join-Path $env:TEMP "harborshield-sdk-smoke-down.txt"
  "hello from boto3 smoke" | Set-Content -Path $payloadPath -NoNewline

  $python = @'
import boto3
import json
import os
from botocore.config import Config

endpoint = os.environ["HS_ENDPOINT"]
bucket = os.environ["HS_BUCKET"]
payload_path = os.environ["HS_PAYLOAD_PATH"]
download_path = os.environ["HS_DOWNLOAD_PATH"]

client = boto3.client(
    "s3",
    endpoint_url=endpoint,
    aws_access_key_id=os.environ["AWS_ACCESS_KEY_ID"],
    aws_secret_access_key=os.environ["AWS_SECRET_ACCESS_KEY"],
    region_name="us-east-1",
    config=Config(signature_version="s3v4", s3={"addressing_style": "path"}),
)

list_buckets = client.list_buckets()
assert "Buckets" in list_buckets

client.create_bucket(Bucket=bucket)

with open(payload_path, "rb") as handle:
    client.put_object(
        Bucket=bucket,
        Key="sdk.txt",
        Body=handle,
        ContentType="text/plain",
        Metadata={"suite": "boto3-smoke"},
    )

head = client.head_object(Bucket=bucket, Key="sdk.txt")
assert head["ContentType"] == "text/plain"

obj = client.get_object(Bucket=bucket, Key="sdk.txt")
body = obj["Body"].read()
with open(download_path, "wb") as handle:
    handle.write(body)
assert body == b"hello from boto3 smoke"

listing = client.list_objects_v2(Bucket=bucket)
assert any(item["Key"] == "sdk.txt" for item in listing.get("Contents", []))

client.copy_object(Bucket=bucket, Key="sdk-copy.txt", CopySource={"Bucket": bucket, "Key": "sdk.txt"})

client.put_object_tagging(
    Bucket=bucket,
    Key="sdk-copy.txt",
    Tagging={"TagSet": [{"Key": "suite", "Value": "boto3"}]},
)
tags = client.get_object_tagging(Bucket=bucket, Key="sdk-copy.txt")
assert any(tag["Key"] == "suite" and tag["Value"] == "boto3" for tag in tags.get("TagSet", []))

client.delete_object(Bucket=bucket, Key="sdk.txt")
client.delete_object(Bucket=bucket, Key="sdk-copy.txt")

versions = client.list_object_versions(Bucket=bucket)
for marker in versions.get("DeleteMarkers", []):
    client.delete_object(Bucket=bucket, Key=marker["Key"], VersionId=marker["VersionId"])
for version in versions.get("Versions", []):
    client.delete_object(Bucket=bucket, Key=version["Key"], VersionId=version["VersionId"])

client.delete_bucket(Bucket=bucket)
print(json.dumps({"status": "ok", "bucket": bucket}))
'@

  $env:HS_ENDPOINT = "$BaseUrl/s3"
  $env:HS_BUCKET = $BucketName
  $env:HS_PAYLOAD_PATH = $payloadPath
  $env:HS_DOWNLOAD_PATH = $downloadPath
  $env:AWS_ACCESS_KEY_ID = $credential.accessKey
  $env:AWS_SECRET_ACCESS_KEY = $credential.secretKey

  Write-Host "Validating S3 flows via boto3..."
  $result = $python | python -
  if ($LASTEXITCODE -ne 0) {
    throw "boto3 SDK smoke failed."
  }

  $smoke = $result | ConvertFrom-Json
  if ($smoke.status -ne "ok") {
    throw "Unexpected boto3 smoke result."
  }

  $downloaded = Get-Content -Raw -Path $downloadPath
  if ($downloaded -ne "hello from boto3 smoke") {
    throw "Downloaded object content did not match expected payload."
  }

  Write-Host "S3 SDK smoke passed."
} finally {
  Remove-Item Env:HS_ENDPOINT, Env:HS_BUCKET, Env:HS_PAYLOAD_PATH, Env:HS_DOWNLOAD_PATH, Env:AWS_ACCESS_KEY_ID, Env:AWS_SECRET_ACCESS_KEY -ErrorAction SilentlyContinue
  if ($payloadPath) {
    Remove-Item -Force $payloadPath -ErrorAction SilentlyContinue
  }
  if ($downloadPath) {
    Remove-Item -Force $downloadPath -ErrorAction SilentlyContinue
  }
  Write-Host "Resetting HarborShield to first-run baseline..."
  docker compose down -v --remove-orphans | Out-Host
  docker compose --env-file .env up --build -d | Out-Host
}
