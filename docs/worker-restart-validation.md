# Worker Restart Validation

This guide covers the current release validation for HarborShield worker restart resilience.

## Goal

Prove that recurring background jobs recover safely when the worker is restarted while a job appears to be in progress.

## Automated Smoke

Run:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\release-worker-restart-smoke.ps1
```

Or:

```powershell
make release-worker-restart-smoke
```

## What The Smoke Validates

The smoke performs this flow:

1. reset to a clean stack
2. bootstrap the admin account
3. rotate the bootstrap password
4. create a bucket
5. upload an object through the admin plane
6. stop the worker
7. force the `bucket_quota_recalc` recurring job into a stale `running` state
8. restart the worker
9. verify that the worker:
   - marks the stale job recoverable
   - reclaims and reruns it
   - returns it to `pending`
   - clears the stale error state
   - updates quota usage for the seeded bucket

At the end, the script resets the environment back to first-run baseline.

## Why This Matters

HarborShield relies on recurring jobs for:

- multipart cleanup
- soft-delete purge
- lifecycle expiration
- quota recalculation
- event delivery retry
- malware scanning
- distributed repair and rebalance in distributed mode

If stale `running` jobs are not recovered correctly after restart, operators can end up with silently stuck background work.

## Current Scope

This validation currently proves the recovery mechanism using `bucket_quota_recalc`, because it is deterministic and easy to verify from persisted state.

It does not yet prove:

- mid-stream recovery for every individual job type
- crash recovery while a long-running distributed repair is actively transferring data
- cross-worker concurrency behavior with multiple worker replicas

## Related Guides

- [session-validation.md](./session-validation.md)
- [upgrade-validation.md](./upgrade-validation.md)
- [operations.md](./operations.md)
- [release-backlog.md](./release-backlog.md)
