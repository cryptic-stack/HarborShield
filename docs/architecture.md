# Architecture

HarborShield separates platform administration from object access:

- `frontend`: React admin UI
- `api`: Go HTTP service exposing `/api/v1/*`, `/s3/*`, `/healthz`, `/readyz`, and `/metrics`
- `worker`: Go background processor for idempotent lifecycle and maintenance jobs
- `postgres`: authoritative metadata store
- `redis`: job, cache, and coordination store
- `caddy`: reverse proxy and edge headers

## Storage model

- Object blobs are stored on local disk at `${STORAGE_ROOT}/tenants/<tenant>/buckets/<bucket-id>/objects/<object-id>`
- PostgreSQL remains authoritative for object metadata, auditability, quotas, and future backend abstractions
- Storage backends implement `storage.BlobStore`
- `STORAGE_BACKEND=local` is the only production-ready backend today
- `STORAGE_BACKEND=distributed` is reserved for a future optional multi-node backend with Garage-like replication and placement semantics, while still keeping metadata in PostgreSQL

## Optional distributed storage direction

The planned distributed mode is intentionally optional:

- single-node Docker Compose remains the default path for homelab and SMB installs
- the distributed backend will be selected explicitly through `STORAGE_BACKEND=distributed`
- the local encrypted filesystem backend remains supported as the default and simplest deployment option
- distributed mode is expected to add:
  - object-part placement across storage nodes
  - per-object replication policy
  - background repair and rebalance jobs
  - node health and capacity reporting
  - a storage-driver abstraction that preserves the current metadata schema

See [`distributed-storage.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\distributed-storage.md) for the phased implementation program.

## Auth planes

- Platform auth: email/password, JWT access token, refresh token, bootstrap admin
- S3 credentials: access key, hashed secret key, scoped policy records, last-used tracking
