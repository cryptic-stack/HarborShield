# Distributed Operations

This runbook covers the supported beta workflow for HarborShield deployments already running with `STORAGE_BACKEND=distributed`.

The goal is to let operators add remote storage gradually without editing backend env vars, bringing the stack down, and starting again every time topology changes.

## Scope

Use this runbook when:

- the control plane is already running in distributed mode
- you want to add or admit storage nodes live
- you want to migrate older locally stored objects into the distributed node set

Do not use this runbook as a replacement for the initial stack bootstrap. The control plane still has to start in distributed mode first.

## Before you begin

Confirm:

- `Settings` shows `storageBackend: distributed`
- the `Storage` page loads successfully
- the blob nodes you plan to use are reachable from the API and worker
- `STORAGE_NODE_SHARED_SECRET` matches across the control plane and blob nodes
- if TLS is enabled, the presented node identity is expected

Recommended:

- capture a support bundle before making topology changes
- use unique node names and stable node endpoints
- keep distributed work on a lab or beta path until you have recovery evidence

## Live node admission

### Register a node

From the `Storage` page:

1. enter the node name
2. enter the node endpoint
3. optionally enter a zone label
4. use `Register Maintenance Node`

Expected result:

- the node appears in `maintenance`
- the node is visible in the live node catalog immediately
- the API and worker do not need a restart for the catalog change to be seen

### Validate the node

Check:

- node health
- last heartbeat
- TLS identity state
- capacity reporting if available

If TLS is intentionally new or rotated:

1. verify the certificate externally
2. use `Re-pin TLS Identity`
3. confirm the storage audit event

### Promote the node

When the node is ready for placement:

1. change operator state from `maintenance` to `active`
2. confirm the `Storage` page reflects the new state
3. confirm healthy node count and replica targets look reasonable

New writes will start using the active distributed node set without restarting the stack.

## Live local-to-distributed migration

This is the key day-2 workflow when the stack started in distributed mode but existing objects still live on the local backend.

### What migration does

- reads older local objects from the local encrypted store
- writes them to the current active distributed node set
- updates object metadata so future reads use the distributed backend
- records placements in PostgreSQL
- removes the old local blob after a successful metadata transition

### Migration workflow

1. make sure at least one healthy node is `active`
2. open `Storage`
3. review:
   - `Pending Local Objects`
   - `Pending Local Bytes`
   - `Distributed Objects`
   - `Distributed Bytes`
   - recent migration history
4. use `Migrate 100 Local Objects`
5. repeat until `Pending Local Objects` reaches `0`

### What success looks like

- pending local count trends downward
- pending local bytes trend downward
- distributed object count trends upward
- distributed bytes trend upward
- new migration history entries appear
- migrated objects still read back successfully
- placement records appear for migrated objects
- the Storage page shows `Local drain complete`

## Safe operating pattern

Use this sequence for remote expansion:

1. register new nodes in `maintenance`
2. validate reachability and TLS identity
3. promote nodes to `active`
4. confirm health and placement posture
5. migrate older local objects in batches
6. keep watching degraded placement and rebalance-gap signals
7. only consider local storage drained when:
   - `Pending Local Objects` is `0`
   - `Pending Local Bytes` is `0 B`
   - the Storage page shows `Local drain complete`
   - at least one healthy node remains `active`

## Rollback expectations

Current beta behavior:

- migrated objects remain readable from distributed storage even if you later return nodes to `maintenance`
- if there are no active nodes, new writes can fall back to local storage
- this is not yet a full topology rollback product feature with automated reverse migration

So the safest rollback posture is:

- stop admitting more nodes
- return unhealthy nodes to `maintenance`
- keep investigating with support-bundle evidence
- avoid assuming automatic distributed-to-local reversal exists

## Failure signs

Investigate immediately if:

- nodes remain `offline` or `tls-mismatch`
- `Pending Local Objects` stops decreasing unexpectedly
- placement records remain missing after a migration pass
- reads fail for recently migrated objects
- degraded placement or rebalance-gap counts rise after node changes

Use:

- [`troubleshooting.md`](./troubleshooting.md)
- [`support-bundle.ps1`](../scripts/support-bundle.ps1)

## Public smoke validation

For a disposable beta validation pass, use:

```sh
./scripts/distributed-migration-smoke.sh
```

This smoke:

- puts all discovered nodes into `maintenance`
- creates a local object while no active distributed targets exist
- promotes nodes live
- runs local-to-distributed migration
- verifies placement records and readback

The GitHub-hosted release-validation workflow now runs this smoke successfully as part of the distributed beta lane, so regressions in this path should show up before release.

Because it changes node operator state and leaves a uniquely named test bucket behind, treat it as a lab or prerelease validation helper rather than a production day-2 command.
