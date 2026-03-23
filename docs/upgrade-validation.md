# Upgrade Validation

This guide defines the current supported upgrade validation flow for HarborShield broad-release readiness.

## Current Support Level

Today, HarborShield validates upgrades by preserving real Docker volumes, rebuilding the current stack, rerunning startup migrations, and confirming seeded data still works.

This is intentionally narrower than a full cross-version upgrade matrix because:

- migrations are currently replayed from `backend/migrations` at startup
- there is not yet a dedicated `schema_migrations` tracking table
- the release process does not yet keep historical install snapshots in-repo

That means the current release gate proves:

- existing metadata survives rebuilds
- existing blob data survives rebuilds
- startup migrations remain idempotent on initialized volumes
- the running stack can continue serving writes after rebuild

## Automated Upgrade Smoke

Run:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\release-upgrade-smoke.ps1
```

Or:

```powershell
make release-upgrade-smoke
```

## What The Smoke Validates

The script performs this flow:

1. reset to a clean stack
2. bootstrap the admin account
3. rotate the bootstrap password
4. complete first-run setup for a local deployment
5. create a bucket
6. create an S3 credential
7. upload and download an object
8. capture setup, bucket, and audit state
9. stop the stack without deleting volumes
10. rebuild and restart the stack
11. verify:
    - rotated admin login still works
    - setup remains complete
    - seeded bucket remains present
    - audit history remains present
    - pre-upgrade object still downloads correctly
    - new post-upgrade writes succeed

At the end, the script resets the environment back to first-run baseline so the local workspace stays predictable.

## What This Does Not Yet Prove

This smoke does not yet prove:

- upgrades from a tagged previous release build
- downgrade safety
- partial migration failure recovery
- historical schema compatibility across multiple release jumps

Those remain follow-on release-hardening items.

## Recommended Manual Follow-Up

After the automated smoke passes, spot-check:

1. `docker compose ps`
2. `http://localhost/healthz`
3. admin login
4. `Audit`
5. `Buckets`
6. one S3 upload and one download

## Related Guides

- [backup-restore.md](./backup-restore.md)
- [worker-restart-validation.md](./worker-restart-validation.md)
- [operations.md](./operations.md)
- [release-backlog.md](./release-backlog.md)
