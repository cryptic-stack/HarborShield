# Troubleshooting Guide

This guide is the first-response playbook for common HarborShield operator issues.

Use it together with:

- [`docs/operations.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\operations.md)
- [`scripts/support-bundle.ps1`](c:\Users\JBrown\Documents\Project\s3-platform\scripts\support-bundle.ps1)

## Before you start

For any issue that is not immediately obvious, capture a support bundle first:

```powershell
make support-bundle
```

Or:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\support-bundle.ps1
```

Check these baseline signals before going deeper:

```powershell
docker compose ps
curl http://localhost/healthz
curl http://localhost/readyz
```

Expected:

- core services are healthy
- `/healthz` returns `{"status":"ok"}`
- `/readyz` returns `{"status":"ready"}`

## Login issues

### Symptom: login page is blank or crashes after paint

Check:

- `frontend` and `caddy` are healthy in `docker compose ps`
- browser hard refresh works
- support bundle includes recent `frontend.log` and `caddy.log`

First actions:

```powershell
docker compose logs frontend --tail=200
docker compose logs caddy --tail=200
```

Likely causes:

- stale frontend bundle in the browser cache
- frontend runtime error on first paint
- Caddy serving stale or broken frontend assets

### Symptom: login returns `Invalid credentials`

Check:

- whether the bootstrap password was already rotated
- whether OIDC is enabled and the operator is using the intended login path
- `Audit` for recent `auth.login` failures

First actions:

- verify the bootstrap credentials in `.env`
- if first-run was already completed, use the rotated admin password instead
- if needed, run the bootstrap reset helper documented in the repo workflow

### Symptom: stale-session loop or forced logout

Check:

- `Audit` for logout or refresh failures
- whether the JWT secret was rotated recently
- browser local storage/session state

First actions:

- sign out and sign back in
- if the issue persists, clear local browser storage for the site
- review `api.log` for repeated `401` responses

## OIDC issues

### Symptom: OIDC test connection fails

Check:

- issuer URL
- redirect URL
- client ID
- whether a client secret is stored
- outbound network access from the API container to the issuer discovery endpoint

First actions:

- open `Settings`
- verify the saved OIDC fields
- use `Test Connection`
- review the resulting `settings.oidc.update` and test-related audit entries

Useful commands:

```powershell
docker compose logs api --tail=200
```

Likely causes:

- wrong issuer URL
- wrong redirect URL
- bad client secret
- provider discovery blocked by network policy or DNS

### Symptom: OIDC login is not available on the login page

Check:

- `Settings` shows OIDC as enabled
- `Settings` shows client secret configured
- `GET /api/v1/auth/oidc` in the support bundle reports `loginReady: true`

Likely causes:

- OIDC saved but not fully configured
- missing client secret
- invalid role mapping or provider discovery failure

## Storage issues

### Symptom: uploads fail or objects cannot be read back

Check:

- `Health` page
- `Dashboard` storage indicators
- `api.log` and `worker.log`
- object and quota audit entries

First actions:

```powershell
docker compose logs api --tail=200
docker compose logs worker --tail=200
curl http://localhost/readyz
```

Likely causes:

- database or Redis availability issue
- quota enforcement
- storage backend permissions or volume issue
- retention or legal-hold enforcement on delete operations

### Symptom: distributed nodes show degraded or offline

Check:

- `Storage` page for node health, operator state, TLS identity, and degraded placements
- `Dashboard` for offline-node and degraded-placement counts
- storage-related audit entries

First actions:

- confirm the node is reachable on its configured endpoint
- review `worker.log` for refresh, repair, or rebalance failures
- confirm TLS identity state if mTLS is enabled
- confirm the node is not intentionally `draining` or `maintenance`

Likely causes:

- node endpoint unreachable
- TLS identity mismatch
- node drained or not yet activated
- replica shortfall during rebalance or repair

### Symptom: distributed remote setup says apply required forever

Check:

- saved deployment setup in the support bundle
- runtime storage backend in `settings.json`
- remote endpoint list in `setup-status.json`

Likely causes:

- runtime still started in local mode
- saved endpoints do not match runtime endpoints exactly
- operator changed the plan but did not restart with matching configuration

## Quota issues

### Symptom: upload or object write returns `Quota exceeded`

Check:

- `Quotas` page for current limits and warning thresholds
- bucket and user usage
- recent audit entries for quota-related failures

First actions:

- confirm whether this is a bucket-bytes, bucket-objects, or user-bytes limit
- review recent usage changes in `Audit`
- adjust the quota or remove old versions if appropriate

Likely causes:

- bucket storage limit reached
- bucket object-count limit reached
- per-user storage limit reached

## Webhook and event issues

### Symptom: webhooks are not being delivered

Check:

- `Events` page for delivery status, retries, and dead-letter state
- `worker.log` for delivery retry errors
- target URL reachability from the worker container

First actions:

- confirm the target endpoint is reachable
- review delivery history in `Events`
- inspect whether failures are auth-related, timeout-related, or DNS-related

Likely causes:

- target service unavailable
- bad signing secret or receiver verification mismatch
- DNS or network routing failure
- repeated retries exhausting into dead-letter state

## Malware pipeline issues

### Symptom: objects stay in pending-scan or scan-failed

Check:

- `Malware` page
- `worker.log`
- ClamAV profile status if enabled

First actions:

- confirm the worker is healthy
- if ClamAV is enabled, confirm the `clamav` service is healthy
- review `malware.scan.completed` audit events

Likely causes:

- worker not running
- ClamAV unavailable
- scan timeouts or pipeline errors

## Support bundle triage map

When you have a support bundle, these files are the fastest starting points:

- `summary/settings-summary.json`
  - quick runtime feature posture without digging through the full settings payload
- `summary/setup-summary.json`
  - first-run completion and runtime-vs-desired deployment mismatch
- `summary/audit-summary.json`
  - recent audit volume, severity mix, and top actions
- `compose/ps.txt`
  - container health and restart state
- `compose/volumes.txt`
  - persistent Docker volume inventory
- `health/healthz.json`
  - basic service availability
- `health/readyz.txt`
  - database and Redis readiness
- `database/row-counts.txt`
  - whether core tables are unexpectedly empty or exploding
- `database/quota-state.txt`
  - quick quota-policy presence check
- `logs/api.log`
  - auth, API, and object-plane errors
- `logs/worker.log`
  - background job, eventing, malware, repair, and rebalance issues
- `admin/settings.json`
  - effective runtime settings
- `admin/setup-status.json`
  - first-run deployment state
- `admin/storage-nodes.json`
  - distributed node and TLS identity state
- `admin/audit-recent.json`
  - recent sensitive actions and failures

## Escalation guidance

Escalate immediately when:

- `postgres` or `redis` is unhealthy
- repeated object writes fail with internal errors
- storage nodes are degraded and repair does not recover
- OIDC is locked out and no local admin path remains
- audit visibility is missing for sensitive changes

When escalating, include:

- the support bundle zip
- the time window of the failure
- the operator action being attempted
- whether the deployment is single-node or distributed
