# Optional Distributed Storage Plan

This document outlines the phased plan for an optional distributed blob backend inspired by the problem space of Garage-style object placement, while keeping HarborShield original in implementation and operator model.

## Goals

- keep `local` storage as the default and simplest path
- make distributed storage opt-in through configuration
- preserve PostgreSQL as the authoritative metadata system
- preserve the current S3, admin, audit, quota, malware, and worker layers
- add multi-node blob durability without forcing a distributed deployment on every user

## Non-goals

- replacing PostgreSQL metadata with a distributed metadata plane
- implementing erasure coding in the first distributed release
- forcing Kubernetes or a separate control plane
- changing the admin UX into a cluster-first product before the backend exists

## Proposed mode switch

- `STORAGE_BACKEND=local`
  - current encrypted filesystem backend
- `STORAGE_BACKEND=distributed`
  - future multi-node backend
- `STORAGE_DISTRIBUTED_ENDPOINTS=http://node-a:9100,http://node-b:9100,http://node-c:9100`
  - reserved for future node membership bootstrap

## Proposed architecture

### Metadata plane

- PostgreSQL remains authoritative for:
  - bucket metadata
  - object metadata
  - version records
  - placement records
  - repair state
  - quotas
  - lifecycle and retention state
  - audit and eventing

### Blob plane

- the distributed backend stores encrypted object chunks on multiple storage nodes
- each object version gets a placement plan written to PostgreSQL
- reads resolve placement from PostgreSQL and fetch from a healthy replica
- writes commit metadata only after the required replica set acknowledges durability
- blob-node RPC authentication is separated from object encryption:
  - `STORAGE_MASTER_KEY` remains the at-rest blob encryption key
  - `STORAGE_NODE_SHARED_SECRET` protects blob-node `PUT`/`GET`/`HEAD`/`DELETE` traffic
- new nodes can be admitted with one-time join tokens before they are switched into an active operator state

### Worker plane

The worker will eventually own:

- placement reconciliation
- replica repair
- rebalance after node membership changes
- orphan cleanup
- capacity accounting
- background verification

## Proposed schema additions

Future migrations should introduce:

- `storage_nodes`
  - node id
  - endpoint
  - availability state
  - capacity metrics
  - placement zone
- `object_placements`
  - object version id
  - chunk ordinal
  - node id
  - physical locator
  - replica index
  - checksum
  - state
- `storage_repairs`
  - repair job id
  - object version id
  - target node id
  - outcome

## Phased implementation

### Phase A: Configuration and abstraction

- add `STORAGE_BACKEND` selection
- keep `local` as the default
- fail fast on unsupported distributed mode until the backend exists
- expose storage-backend status in settings and docs

### Phase B: Placement metadata

- add `storage_nodes` and `object_placements`
- write placement records alongside object versions
- keep actual writes on local storage while the metadata model is proven

### Phase C: Node service

- add a dedicated blob-node service with:
  - encrypted put/get/delete
  - checksum validation
  - health endpoint
  - authenticated internal API
  - one-time join-token enrollment

### Phase D: Replicated writes

- add a distributed storage driver that:
  - chooses replica targets
  - writes to multiple nodes
  - commits metadata after durability threshold
  - supports read failover

### Phase E: Repair and rebalance

- add worker-driven repair jobs
- add node drain and rebalance
- expose health and placement drift in the admin UI

### Phase F: Advanced durability

- optional erasure coding
- topology-aware placement
- cross-site replication hooks

## Docker direction

Distributed mode should stay optional in Docker:

- keep the current Compose stack unchanged for `local`
- add a second Compose file or profile for distributed labs
- avoid publishing storage-node ports externally by default

## Security model

- retain application-level blob encryption by default
- use signed internal node-to-node requests
- keep secret material out of object metadata rows
- audit placement mutations and repair outcomes

## Recommended first implementation tasks

1. Add `STORAGE_BACKEND` config and a storage factory.
2. Expose backend status in the admin settings snapshot.
3. Add this design doc and roadmap references.
4. Add schema for `storage_nodes` and `object_placements` without activating them.
5. Build a dev-only blob-node service behind an internal network.
