# Backup And Restore

This runbook covers the minimum supported HarborShield backup and restore flow for single-node operation.

Current support level:

- `single-node`: primary documented recovery path
- `distributed`: backup and restore still require additional topology-aware validation and should be treated as operator-managed beta recovery

## What must be backed up

For a meaningful HarborShield recovery, back up all of the following:

1. PostgreSQL metadata
2. object blob storage
3. deployment configuration

In practice, that means:

- PostgreSQL data in the `postgres_data` volume
- object data in the `object_data` volume
- your project `.env`

Optional but useful:

- exported support logs
- screenshots or notes of key live settings

## Recommended backup cadence

Minimum recommendation for smaller deployments:

- PostgreSQL: daily
- object data: daily or aligned to your retention policy
- `.env`: whenever secrets or deployment settings change

For more active environments:

- PostgreSQL: more frequent logical backups
- object data: snapshot-based or host-level filesystem backup

## Pre-backup checks

Before taking a scheduled backup:

1. confirm `docker compose ps` is healthy
2. confirm `/healthz` and `/readyz` succeed
3. review recent `Audit` and `Health` pages for major failures

## PostgreSQL backup

Example logical backup:

```powershell
docker compose exec -T postgres pg_dump -U s3platform -d s3platform > backup-postgres.sql
```

Recommended naming:

```text
backup-postgres-YYYYMMDD-HHMM.sql
```

If you need the whole container volume instead, use your host or Docker volume backup tooling in addition to or instead of a logical dump.

## Object data backup

Single-node object blobs live in the `object_data` volume.

To archive that volume from Docker:

```powershell
docker run --rm ^
  -v s3-platform_object_data:/source ^
  -v ${PWD}:/backup ^
  alpine sh -c "cd /source && tar czf /backup/backup-object-data.tar.gz ."
```

Recommended naming:

```text
backup-object-data-YYYYMMDD-HHMM.tar.gz
```

## Configuration backup

Keep a secure copy of:

- `.env`

Treat it as sensitive because it contains:

- database password
- Redis password
- JWT secret
- storage master key
- node shared secret
- bootstrap admin password

## Restore sequence

Restore in this order:

1. configuration
2. PostgreSQL metadata
3. object blobs
4. stack startup
5. verification

### Step 1: restore `.env`

Put the correct `.env` back in place first.

Important:

- `STORAGE_MASTER_KEY` must match the original deployment or encrypted blobs cannot be read
- `JWT_SECRET` should match if you want continuity for tokens, though resetting sessions is also acceptable

### Step 2: restore PostgreSQL

Bring up only the database first if needed, then restore:

```powershell
docker compose up -d postgres
Get-Sleep 5
Get-Content .\backup-postgres.sql | docker compose exec -T postgres psql -U s3platform -d s3platform
```

If the database is not empty, clear or recreate it first according to your restore target.

### Step 3: restore object blobs

Restore the object-data archive back into the `object_data` volume:

```powershell
docker run --rm ^
  -v s3-platform_object_data:/target ^
  -v ${PWD}:/backup ^
  alpine sh -c "cd /target && tar xzf /backup/backup-object-data.tar.gz"
```

### Step 4: bring up the full stack

```powershell
docker compose --env-file .env up --build -d
```

### Step 5: verify recovery

Verify:

1. `docker compose ps`
2. `http://localhost/healthz`
3. admin login
4. bucket list
5. object download
6. audit list

Recommended smoke:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\release-clean-install-smoke.ps1
```

If you are restoring a real environment rather than a clean smoke environment, use a lighter validation set that does not intentionally reset data.

## Recovery validation checklist

Recovery is successful only if all are true:

- API is healthy
- worker is healthy
- admin login works
- buckets are present
- objects download successfully
- metadata matches blob availability
- audit remains readable

## Important caveats

- rebuild and restart validation is covered separately in [`upgrade-validation.md`](./upgrade-validation.md)

- object blobs and metadata must stay in sync; restoring only one side is not sufficient
- losing the original `STORAGE_MASTER_KEY` breaks blob decryption
- distributed-mode recovery needs extra node and placement validation beyond this single-node runbook

## Recommended next improvement

Before broad release, HarborShield should add:

- an official support bundle
- a versioned backup validation script
- a documented upgrade + restore drill
