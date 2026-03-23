# Operations Guide

This guide covers normal HarborShield operation after first-run setup is complete.

Troubleshooting companion:

- [`docs/troubleshooting.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\troubleshooting.md)
- [`docs/metrics-alerting.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\metrics-alerting.md)
- [`docs/audit-coverage.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\audit-coverage.md)
- [`docs/sensitive-data-review.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\sensitive-data-review.md)

## Daily operator checks

Start with these pages:

1. `Dashboard`
   - bucket count
   - object count
   - stored bytes
   - recent audit volume
   - storage health indicators
2. `Health`
   - confirm API, database, Redis, and worker posture
3. `Audit`
   - review authentication, settings, policy, and storage events
4. `Events`
   - review delivery failures and dead-letter activity

## Bucket and object administration

Use `Buckets`, `Bucket Detail`, `Objects`, and `Uploads` for normal data operations.

Supported tasks include:
- create buckets
- choose storage class and replica behavior for buckets
- upload and download objects
- delete and restore objects
- review object versions
- edit object tags
- copy objects

Operational reminders:
- deletes can be blocked by retention or legal hold
- version history must be cleaned if you want a versioned bucket to become fully empty
- large uploads may be rejected by quotas or upload-size limits

## Credential and access management

Use:
- `Users`
- `Credentials`
- `Roles`
- `Policy Lab`
- `Admin Tokens`

Common day-2 tasks:
- rotate or replace S3 credentials
- adjust user roles
- bind scoped roles to subjects
- test permissions before rollout
- issue or revoke admin automation tokens

Recommended practice:
- keep object-plane credentials and admin tokens separate
- use least privilege by default
- verify policy changes in `Policy Lab` before granting broad access

## Quotas

Use `Quotas` to manage:
- per-bucket storage limits
- per-bucket object-count limits
- per-user storage limits
- warning thresholds

Notes:
- storage values are entered in human-readable units
- enforcement happens on the write path
- warnings are surfaced before hard denial when thresholds are crossed

## OIDC operation

Use `Settings` to manage OIDC after bootstrap.

Recommended workflow:

1. Save issuer, client ID, redirect URL, scopes, and role mapping
2. Store the client secret
3. Use `Test Connection`
4. Review the resulting audit record
5. Enable OIDC for production use

If you rotate the provider secret:

1. open `Settings`
2. enter the new secret
3. save the OIDC settings
4. use `Test Connection`

If you want to remove the current secret:

1. use `Clear Stored Secret`
2. confirm that `OIDC Client Secret` shows as not configured
3. review the audit event

## Audit review

The `Audit` page now supports:
- actor filter
- action filter
- outcome filter
- category filter
- severity filter
- free-text search

Audit categories include:
- authentication
- settings
- storage
- data
- access control
- quota
- malware
- eventing
- deployment
- system

Use severity to triage:
- `info`: routine reads and normal auth
- `low`: standard writes and configuration updates
- `medium`: destructive or higher-impact operations
- `high`: failures and more urgent operator attention

Settings changes render as before/after diffs rather than raw JSON.

## Storage operations

Distributed-specific companion:

- [`docs/distributed-operations.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\distributed-operations.md)

### Single-node mode

In single-node mode:
- blobs live on the local encrypted filesystem backend
- `Storage` is still useful for visibility, but distributed placement does not apply

### Distributed mode

In distributed mode, use `Storage` for:
- node health
- operator state
- TLS identity state
- placement visibility
- replica policy visibility
- migration status and migration history

Operator states:
- `active`: normal placement target
- `draining`: existing placements should move away
- `maintenance`: not used for active placement

Typical workflows:

#### Admit a node

1. register the node
2. confirm it appears in `maintenance`
3. activate it when ready
4. confirm health and TLS identity
5. watch rebalance and placement signals

#### Drain a node

1. set the node to `draining`
2. wait for placements to move
3. confirm no critical shortfall remains
4. move to `maintenance` or retire it

#### Migrate older local objects

1. confirm at least one healthy node is `active`
2. review `Pending Local Objects`
3. use `Migrate 100 Local Objects`
4. verify migration history and placement visibility
5. repeat until the pending local count reaches `0`

#### Re-pin TLS identity

If a node certificate changes intentionally:

1. verify the new certificate externally
2. use `Re-pin TLS Identity`
3. confirm the storage audit event

## Malware and eventing

Use `Malware` to review:
- pending scans
- clean objects
- infected objects
- failed scans

Use `Events` to review:
- webhook targets
- delivery success and failure
- retry state
- dead-letter activity

## Maintenance and troubleshooting

Useful commands:

```powershell
docker compose ps
docker compose logs api --tail=200
docker compose logs worker --tail=200
curl http://localhost/healthz
curl http://localhost/readyz
curl http://localhost/metrics
```

If you need a clean restart:

```powershell
docker compose down --remove-orphans
docker compose --env-file .env up --build -d
```

## Support bundle

For a single operator-ready diagnostic snapshot, generate a support bundle:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\support-bundle.ps1
```

Or use:

```powershell
make support-bundle
```

The bundle includes:
- Docker and Compose version details
- `docker compose ps`, service list, and image list
- Docker volume inventory and `docker system df`
- `/healthz`, `/readyz`, and a `/metrics` sample
- recent logs for `api`, `worker`, `frontend`, `caddy`, `postgres`, and `redis`
- migration file inventory and current database table inventory
- database row counts and quota-state summaries
- selected admin snapshots when login succeeds:
  - `auth/me`
  - setup status
  - settings
  - dashboard
  - health
  - storage nodes
  - recent audit
- operator-ready summaries for:
  - effective settings
  - setup state
  - recent audit volume and top actions

By default the script tries the bootstrap admin credentials. You can pass a different password if the admin password has already been rotated.

If you need a full first-run reset:

```powershell
docker compose down -v --remove-orphans
docker compose --env-file .env up --build -d
```

## Recommended operating pattern

1. Check `Dashboard`
2. Review `Health`
3. Scan `Audit`
4. Review `Events` and `Malware`
5. Make access or storage changes
6. Confirm the resulting audit trail

For initial bootstrap and wizard-driven deployment selection, use [`first-run.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\first-run.md).
