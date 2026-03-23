# Distributed Storage Beta

This document describes HarborShield's current distributed-storage beta path.

The key operator goal is simple: once the stack is already running in `STORAGE_BACKEND=distributed`, adding remote storage nodes and migrating existing local objects should not require editing backend config and restarting the API or worker.

## Current status

- `local` remains the default and broad-release candidate path
- `distributed` is implemented as a beta backend
- PostgreSQL remains authoritative for:
  - buckets
  - object metadata
  - object placements
  - quotas
  - audit history
  - operator state
- the `Storage` page is the main control surface for distributed operation

## What works now

- explicit backend selection through `STORAGE_BACKEND`
- distributed blob writes when active storage nodes are available
- live node registration and operator-state changes through the admin API and UI
- node health refresh plus TLS identity observation and re-pin support
- placement records stored in PostgreSQL
- per-object `storage_backend` tracking so reads, deletes, and malware scans work during mixed local and distributed operation
- local write fallback when the distributed backend is enabled but no active placement targets are available
- operator-driven local-to-distributed migration in batches without restarting the API or worker

## Operator model

HarborShield treats distributed storage as a control-plane-driven catalog:

- storage nodes are registered in `storage_nodes`
- object replicas are tracked in `object_placements`
- active placement targets are resolved from the live node catalog, not only from startup config
- existing local objects can be copied into distributed storage later while preserving metadata continuity

That means a running distributed deployment can evolve this way:

1. start the control plane in distributed mode
2. keep new or remote nodes in `maintenance`
3. promote healthy nodes to `active`
4. let new writes target the active distributed set
5. migrate older local objects in batches from the `Storage` page

## Security model

- `STORAGE_MASTER_KEY` still protects encrypted blobs at rest
- `STORAGE_NODE_SHARED_SECRET` is separate and protects blob-node RPC traffic
- one-time join tokens can be used to enroll nodes before they receive RPC credentials
- HTTPS and TLS identity pinning can be layered onto blob-node connections

## Supported beta workflow

The supported runtime workflow is documented in [`distributed-operations.md`](./distributed-operations.md).

In short:

- do not treat `STORAGE_DISTRIBUTED_ENDPOINTS` as the only day-2 source of truth after the stack is up
- use the `Storage` page or storage admin API to register and manage nodes live
- use migration status and history to drain older local objects gradually

## Current limitations

- `distributed` is still beta and not part of the `v1.0` GA promise
- migration progress is currently object-count based, not byte-accurate
- migration is batch-driven, not a long-running managed workflow with pause and resume semantics
- distributed repair and rebalance need more operational proof before promotion
- distributed backup and restore still require topology-aware validation beyond the single-node runbook

## Public regression coverage

Use the supported smoke for this path:

- [`scripts/distributed-migration-smoke.sh`](../scripts/distributed-migration-smoke.sh)

That smoke validates:

- admin login
- storage-node visibility
- local write fallback with no active nodes
- live node activation
- local-to-distributed migration
- post-migration object readback and placement visibility

## Promotion criteria

Distributed mode should remain beta until all are true:

- operator docs are complete
- migration and recovery behavior are repeatedly validated
- repair and rebalance reliability are proven under failure
- support level and known limitations are explicit in release notes
