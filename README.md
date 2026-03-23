# HarborShield Object Platform

HarborShield is an original, Docker-first object storage platform built for homelabs, SMBs, cyber ranges, and enterprise labs. It provides an S3-compatible object API, a separate admin API and web UI, PostgreSQL-backed metadata, Redis-backed background coordination, local-disk blob storage, and a security-first foundation for future policy, scanning, retention, and identity integrations.

Object blobs are encrypted at rest by default using the `STORAGE_MASTER_KEY` application key. Distributed blob RPCs now use a separate `STORAGE_NODE_SHARED_SECRET`, and optional storage-node enrollment can be driven through one-time join tokens instead of raw metadata registration alone. Database and Redis volume encryption still depend on the host, Docker storage layer, or an external encrypted deployment pattern and remain a follow-on hardening item.

## Current platform scope

Implemented now:

- admin login, refresh, session lookup, and distinct admin API tokens
- bucket list and create flows in the admin API and UI
- object upload, download, delete, restore, copy, version history, and tagging in the admin API and UI
- S3-compatible `CreateBucket`, `DeleteBucket`, `ListBuckets`, `PutObject`, `GetObject`, `DeleteObject`, `HeadObject`, `ListObjectsV2`, presigned `GET` and `PUT`, multipart initiate/upload/complete/abort, `CopyObject`, and object tagging
- PostgreSQL-backed metadata with filesystem blob storage and encrypted blobs at rest
- deny-by-default roles, statements, scoped bindings, and policy evaluation tooling
- searchable audit logs
- quota visibility and enforcement
- webhook event targets and delivery history
- malware scan pipeline with worker processing and UI visibility
- Prometheus metrics, structured logs, `/healthz`, `/readyz`, and admin health views

Not complete yet:

- OpenAPI generation beyond the scaffold in [`docs/openapi-admin.yaml`](c:\Users\JBrown\Documents\Project\s3-platform\docs\openapi-admin.yaml)
- richer OIDC federation polish such as live provider onboarding guidance, advanced claim sync, and temporary object-plane credentials
- stronger object-lock or legal-hold semantics beyond current retention hooks
- replication, storage tiering, and non-filesystem backends
- full AWS IAM and full AWS S3 compatibility parity
- end-to-end encrypted PostgreSQL and Redis data stores by default

Optional distributed storage is now explicitly planned through the `STORAGE_BACKEND` selector, but only `local` is implemented today. A future `distributed` backend is intended to add Garage-like multi-node placement and replication while keeping PostgreSQL authoritative for metadata.

## Included components

- Go API with separate admin and S3-compatible object planes
- Go worker for recurring cleanup, governance, malware, lifecycle, quota, and event work
- React + TypeScript admin UI with dashboard, buckets, objects, uploads, users, credentials, roles, audit, events, quotas, malware, health, and settings pages
- PostgreSQL migrations for users, credentials, buckets, objects, audit logs, quotas, refresh tokens, multipart sessions, event targets, job state, versioning, and tagging
- local filesystem storage driver with DB-backed metadata and non-user-facing object paths
- Docker Compose stack with Caddy, PostgreSQL, Redis, API, worker, frontend, and optional ClamAV and Prometheus profiles

## Quick start

1. Copy `.env.example` to `.env` and rotate all secrets.
2. Run `make up`.
3. Open `http://localhost`.
4. Log in with the bootstrap admin configured in `.env`.

## Operator guides

- First-run bootstrap and setup wizard: [`docs/first-run.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\first-run.md)
- Day-2 operation and admin workflows: [`docs/operations.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\operations.md)
- First-run release walkthrough checklist: [`docs/first-run-checklist.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\first-run-checklist.md)
- Deployment details and secret handling: [`docs/deployment.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\deployment.md)
- Backup and restore runbook: [`docs/backup-restore.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\backup-restore.md)
- Secret rotation runbook: [`docs/secret-rotation.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\secret-rotation.md)
- Troubleshooting guide: [`docs/troubleshooting.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\troubleshooting.md)
- Metrics and alerting reference: [`docs/metrics-alerting.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\metrics-alerting.md)
- Audit coverage reference: [`docs/audit-coverage.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\audit-coverage.md)
- Sensitive-data exposure review: [`docs/sensitive-data-review.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\sensitive-data-review.md)
- Architecture and storage model: [`docs/architecture.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\architecture.md)
- S3 compatibility matrix: [`docs/s3-compatibility.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\s3-compatibility.md)
- Broad-release quality execution backlog: [`docs/release-backlog.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\release-backlog.md)

## Verification

Run the local verification script after the stack is up:

`./scripts/smoke-test.sh`

It validates:

- admin login
- bucket create/list
- credential creation
- object upload/download
- audit API fetch

Additional phase smokes live under [`scripts/`](c:\Users\JBrown\Documents\Project\s3-platform\scripts) for quotas, worker behavior, lifecycle, malware, versioning, and copy/tagging flows.

Release-quality verification also includes:

- consolidated release-readiness gate: [`scripts/release-readiness.ps1`](c:\Users\JBrown\Documents\Project\s3-platform\scripts\release-readiness.ps1)
  - default run covers Compose validation, backend tests, and frontend production build
  - optional switches add release smokes, resilience regression, and S3 compatibility regression
- GitHub Actions fast CI: [`.github/workflows/ci.yml`](c:\Users\JBrown\Documents\Project\s3-platform\.github\workflows\ci.yml)
  - runs the default release-readiness gate on pushes to `main` and pull requests
- GitHub Actions manual release validation: [`.github/workflows/release-validation.yml`](c:\Users\JBrown\Documents\Project\s3-platform\.github\workflows\release-validation.yml)
  - workflow-dispatch only
  - prepares `.env` from `.env.example`
  - can run release smokes, resilience checks, and S3 regression checks on demand
  - collects a support bundle artifact automatically if the release validation run fails
- GitHub Actions tagged release pipeline: [`.github/workflows/release.yml`](c:\Users\JBrown\Documents\Project\s3-platform\.github\workflows\release.yml)
  - runs the full release validation gate on tags matching `v*`
  - publishes versioned GHCR images for the backend runtime and frontend
  - attaches a deployment bundle with pinned `release-images.env` image references and SHA-256 checksums to the GitHub release
- clean-install smoke: [`scripts/release-clean-install-smoke.ps1`](c:\Users\JBrown\Documents\Project\s3-platform\scripts\release-clean-install-smoke.ps1)
- upgrade smoke with preserved volumes: [`scripts/release-upgrade-smoke.ps1`](c:\Users\JBrown\Documents\Project\s3-platform\scripts\release-upgrade-smoke.ps1)
- worker restart resilience smoke: [`scripts/release-worker-restart-smoke.ps1`](c:\Users\JBrown\Documents\Project\s3-platform\scripts\release-worker-restart-smoke.ps1)
- session revocation regression smoke: [`scripts/release-session-regression-smoke.ps1`](c:\Users\JBrown\Documents\Project\s3-platform\scripts\release-session-regression-smoke.ps1)
- boto3 SDK compatibility smoke: [`scripts/s3-sdk-smoke.ps1`](c:\Users\JBrown\Documents\Project\s3-platform\scripts\s3-sdk-smoke.ps1)
- multipart and versioning edge-case smoke: [`scripts/s3-edge-smoke.ps1`](c:\Users\JBrown\Documents\Project\s3-platform\scripts\s3-edge-smoke.ps1)
- bucket-policy compatibility smoke: [`scripts/s3-policy-smoke.ps1`](c:\Users\JBrown\Documents\Project\s3-platform\scripts\s3-policy-smoke.ps1)
- bucket-policy condition smoke: [`scripts/s3-policy-conditions-smoke.ps1`](c:\Users\JBrown\Documents\Project\s3-platform\scripts\s3-policy-conditions-smoke.ps1)
- operator support bundle: [`scripts/support-bundle.ps1`](c:\Users\JBrown\Documents\Project\s3-platform\scripts\support-bundle.ps1)
  - includes summary views for settings, setup, audit volume, database row counts, and quota state

Tagged releases now produce a deployment bundle that can be started with:

`docker compose --env-file .env --env-file release-images.env up -d`
