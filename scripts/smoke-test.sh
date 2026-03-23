#!/usr/bin/env sh
set -eu

BASE_URL="${BASE_URL:-http://localhost}"
EMAIL="${ADMIN_EMAIL:-admin@example.com}"
PASSWORD="${ADMIN_PASSWORD:-change_me_now}"
BUCKET="${SMOKE_BUCKET:-smoke-bucket}"

LOGIN_RESPONSE="$(curl -fsS -X POST "$BASE_URL/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")"

TOKEN="$(printf '%s' "$LOGIN_RESPONSE" | sed -n 's/.*"accessToken":"\([^"]*\)".*/\1/p')"
[ -n "$TOKEN" ]

curl -fsS -X POST "$BASE_URL/api/v1/buckets" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"$BUCKET\"}" >/dev/null 2>&1 || true

curl -fsS "$BASE_URL/api/v1/buckets" \
  -H "Authorization: Bearer $TOKEN" >/dev/null

CREDENTIAL_RESPONSE="$(curl -fsS -X POST "$BASE_URL/api/v1/credentials" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"userId":"","role":"admin","description":"smoke test credential"}')"

ACCESS_KEY="$(printf '%s' "$CREDENTIAL_RESPONSE" | sed -n 's/.*"accessKey":"\([^"]*\)".*/\1/p')"
SECRET_KEY="$(printf '%s' "$CREDENTIAL_RESPONSE" | sed -n 's/.*"secretKey":"\([^"]*\)".*/\1/p')"
[ -n "$ACCESS_KEY" ]
[ -n "$SECRET_KEY" ]

PAYLOAD_FILE="$(mktemp)"
printf 'hello from smoke test' > "$PAYLOAD_FILE"

curl -fsS -X PUT "$BASE_URL/s3/$BUCKET/test.txt" \
  -H "X-S3P-Access-Key: $ACCESS_KEY" \
  -H "X-S3P-Secret: $SECRET_KEY" \
  -H 'Content-Type: text/plain' \
  --data-binary @"$PAYLOAD_FILE" >/dev/null

DOWNLOADED="$(curl -fsS "$BASE_URL/s3/$BUCKET/test.txt" \
  -H "X-S3P-Access-Key: $ACCESS_KEY" \
  -H "X-S3P-Secret: $SECRET_KEY")"

[ "$DOWNLOADED" = "hello from smoke test" ]

curl -fsS "$BASE_URL/api/v1/audit" \
  -H "Authorization: Bearer $TOKEN" >/dev/null

rm -f "$PAYLOAD_FILE"
printf 'Smoke test passed.\n'
