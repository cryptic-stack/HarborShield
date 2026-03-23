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
- `STORAGE_BACKEND=local` remains the default and broad-release candidate path
- `STORAGE_BACKEND=distributed` is a beta multi-node backend with live node admission, placement records, and local-to-distributed migration while still keeping metadata in PostgreSQL

## Optional distributed storage direction

The distributed mode is intentionally optional:

- single-node Docker Compose remains the default path for homelab and SMB installs
- the distributed backend will be selected explicitly through `STORAGE_BACKEND=distributed`
- the local encrypted filesystem backend remains supported as the default and simplest deployment option
- distributed mode already includes:
  - live storage-node catalog updates from the control plane
  - object placement records in PostgreSQL
  - per-object storage backend tracking during mixed local and distributed operation
  - operator-driven local-to-distributed migration from the `Storage` page
  - node health, TLS identity, and operator-state visibility in the admin UI
- distributed mode still needs more proof before GA:
  - stronger repair and rebalance reliability evidence
  - topology and recovery runbooks
  - wider failure-path regression coverage

See [`distributed-storage.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\distributed-storage.md) for the current beta design and [`distributed-operations.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\distributed-operations.md) for the supported operator workflow.

## Auth planes

- Platform auth: email/password, JWT access token, refresh token, bootstrap admin
- S3 credentials: access key, hashed secret key, scoped policy records, last-used tracking
