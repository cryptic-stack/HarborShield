#!/usr/bin/env sh
set -eu

BASE_URL="${BASE_URL:-http://localhost}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@example.com}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-change_me_now}"
PYTHON_BIN="${PYTHON_BIN:-python3}"
BUCKET="distributed-migration-$(date +%s)-$$"
OBJECT_KEY="${OBJECT_KEY:-migration.txt}"
OBJECT_BODY="${OBJECT_BODY:-hello from distributed migration smoke}"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'Required command not found: %s\n' "$1" >&2
    exit 1
  fi
}

api_json() {
  method="$1"
  path="$2"
  body="${3-}"
  if [ -n "$body" ]; then
    curl -fsS -X "$method" "$BASE_URL$path" \
      -H "Authorization: Bearer $TOKEN" \
      -H 'Content-Type: application/json' \
      -d "$body"
  else
    curl -fsS -X "$method" "$BASE_URL$path" \
      -H "Authorization: Bearer $TOKEN"
  fi
}

restore_node_states() {
  if [ -z "${ORIGINAL_NODES_FILE:-}" ] || [ ! -f "$ORIGINAL_NODES_FILE" ]; then
    return 0
  fi
  "$PYTHON_BIN" - "$ORIGINAL_NODES_FILE" <<'PY' | while IFS='|' read -r node_id operator_state; do
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    data = json.load(handle)

for item in data.get("items", []):
    node_id = str(item.get("id", "")).strip()
    operator_state = str(item.get("operatorState", "")).strip()
    if node_id and operator_state:
        print(f"{node_id}|{operator_state}")
PY
    curl -fsS -X PATCH "$BASE_URL/api/v1/storage/nodes/$node_id" \
      -H "Authorization: Bearer $TOKEN" \
      -H 'Content-Type: application/json' \
      -d "{\"operatorState\":\"$operator_state\"}" >/dev/null || true
  done
}

cleanup() {
  restore_node_states
  rm -f "${LOGIN_FILE:-}" "${NODES_FILE:-}" "${ORIGINAL_NODES_FILE:-}" "${STATUS_FILE:-}" "${STATUS_AFTER_PUT_FILE:-}" \
    "${STATUS_AFTER_MIGRATION_FILE:-}" "${CREDENTIAL_FILE:-}" "${PAYLOAD_FILE:-}" "${PLACEMENTS_FILE:-}" "${DOWNLOAD_FILE:-}"
}

trap cleanup EXIT INT TERM

require_command curl
require_command "$PYTHON_BIN"

LOGIN_FILE="$(mktemp)"
NODES_FILE="$(mktemp)"
ORIGINAL_NODES_FILE="$(mktemp)"
STATUS_FILE="$(mktemp)"
STATUS_AFTER_PUT_FILE="$(mktemp)"
STATUS_AFTER_MIGRATION_FILE="$(mktemp)"
CREDENTIAL_FILE="$(mktemp)"
PAYLOAD_FILE="$(mktemp)"
PLACEMENTS_FILE="$(mktemp)"
DOWNLOAD_FILE="$(mktemp)"

curl -fsS -X POST "$BASE_URL/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}" > "$LOGIN_FILE"

TOKEN="$("$PYTHON_BIN" - "$LOGIN_FILE" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    print(json.load(handle)["accessToken"])
PY
)"

api_json GET "/storage/nodes" > "$NODES_FILE"
cp "$NODES_FILE" "$ORIGINAL_NODES_FILE"

NODE_COUNT="$("$PYTHON_BIN" - "$NODES_FILE" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    print(len(json.load(handle).get("items", [])))
PY
)"

if [ "$NODE_COUNT" -lt 1 ]; then
  printf 'No distributed storage nodes are configured. Start the distributed profile first.\n' >&2
  exit 1
fi

api_json GET "/storage/migration-status" > "$STATUS_FILE"
BASELINE_PENDING="$("$PYTHON_BIN" - "$STATUS_FILE" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    print(int(json.load(handle).get("pendingLocalObjects", 0)))
PY
)"

if [ "$BASELINE_PENDING" -gt 900 ]; then
  printf 'Pending local object backlog is too large for this smoke (%s). Use a cleaner beta stack.\n' "$BASELINE_PENDING" >&2
  exit 1
fi

"$PYTHON_BIN" - "$NODES_FILE" <<'PY' | while IFS='|' read -r node_id; do
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    data = json.load(handle)

for item in data.get("items", []):
    node_id = str(item.get("id", "")).strip()
    if node_id:
        print(node_id)
PY
  api_json PATCH "/storage/nodes/$node_id" '{"operatorState":"maintenance"}' >/dev/null
done

CREDENTIAL_BODY='{"role":"admin","description":"distributed-migration-smoke"}'
api_json POST "/credentials" "$CREDENTIAL_BODY" > "$CREDENTIAL_FILE"

ACCESS_KEY="$("$PYTHON_BIN" - "$CREDENTIAL_FILE" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    print(json.load(handle)["accessKey"])
PY
)"

SECRET_KEY="$("$PYTHON_BIN" - "$CREDENTIAL_FILE" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    print(json.load(handle)["secretKey"])
PY
)"

curl -fsS -X PUT "$BASE_URL/s3/$BUCKET" \
  -H "X-S3P-Access-Key: $ACCESS_KEY" \
  -H "X-S3P-Secret: $SECRET_KEY" >/dev/null

printf '%s' "$OBJECT_BODY" > "$PAYLOAD_FILE"

curl -fsS -X PUT "$BASE_URL/s3/$BUCKET/$OBJECT_KEY" \
  -H "X-S3P-Access-Key: $ACCESS_KEY" \
  -H "X-S3P-Secret: $SECRET_KEY" \
  -H 'Content-Type: text/plain' \
  --data-binary @"$PAYLOAD_FILE" >/dev/null

api_json GET "/storage/migration-status" > "$STATUS_AFTER_PUT_FILE"
PENDING_AFTER_PUT="$("$PYTHON_BIN" - "$STATUS_AFTER_PUT_FILE" "$BASELINE_PENDING" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    pending = int(json.load(handle).get("pendingLocalObjects", 0))

baseline = int(sys.argv[2])
if pending < baseline + 1:
    raise SystemExit(1)
print(pending)
PY
)" || {
  printf 'Expected the new object to remain local while all nodes were in maintenance.\n' >&2
  exit 1
}

"$PYTHON_BIN" - "$NODES_FILE" <<'PY' | while IFS='|' read -r node_id; do
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    data = json.load(handle)

for item in data.get("items", []):
    node_id = str(item.get("id", "")).strip()
    if node_id:
        print(node_id)
PY
  api_json PATCH "/storage/nodes/$node_id" '{"operatorState":"active"}' >/dev/null
done

MIGRATION_LIMIT=$((BASELINE_PENDING + 10))
if [ "$MIGRATION_LIMIT" -lt 10 ]; then
  MIGRATION_LIMIT=10
fi
if [ "$MIGRATION_LIMIT" -gt 1000 ]; then
  MIGRATION_LIMIT=1000
fi

MIGRATION_RESULT="$(api_json POST "/storage/migrations/local-to-distributed" "{\"limit\":$MIGRATION_LIMIT}")"

MIGRATED_COUNT="$(printf '%s' "$MIGRATION_RESULT" | "$PYTHON_BIN" - <<'PY'
import json
import sys

print(int(json.load(sys.stdin).get("migratedCount", 0)))
PY
)"

if [ "$MIGRATED_COUNT" -lt 1 ]; then
  printf 'Migration endpoint did not migrate any objects.\n' >&2
  exit 1
fi

api_json GET "/storage/migration-status" > "$STATUS_AFTER_MIGRATION_FILE"
"$PYTHON_BIN" - "$STATUS_AFTER_MIGRATION_FILE" "$PENDING_AFTER_PUT" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    pending = int(json.load(handle).get("pendingLocalObjects", 0))

before = int(sys.argv[2])
if pending >= before:
    raise SystemExit(1)
PY

FILTERED_KEY="$("$PYTHON_BIN" - "$OBJECT_KEY" <<'PY'
import sys
import urllib.parse

print(urllib.parse.quote(sys.argv[1], safe=""))
PY
)"

api_json GET "/storage/placements?limit=20&key=$FILTERED_KEY" > "$PLACEMENTS_FILE"
"$PYTHON_BIN" - "$PLACEMENTS_FILE" "$OBJECT_KEY" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    items = json.load(handle).get("items", [])

target_key = sys.argv[2]
matching = [item for item in items if item.get("objectKey") == target_key]
if not matching:
    raise SystemExit(1)
PY

curl -fsS "$BASE_URL/s3/$BUCKET/$OBJECT_KEY" \
  -H "X-S3P-Access-Key: $ACCESS_KEY" \
  -H "X-S3P-Secret: $SECRET_KEY" > "$DOWNLOAD_FILE"

DOWNLOADED_BODY="$(cat "$DOWNLOAD_FILE")"
if [ "$DOWNLOADED_BODY" != "$OBJECT_BODY" ]; then
  printf 'Downloaded payload did not match the migrated object body.\n' >&2
  exit 1
fi

printf 'Distributed migration smoke passed. Bucket: %s, object: %s\n' "$BUCKET" "$OBJECT_KEY"
