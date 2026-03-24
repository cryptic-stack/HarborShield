# Roadmap

## Phase 1

- documentation and OpenAPI contract completion
- stale-session recovery and bootstrap password-change flow
- encrypted deployment guidance for PostgreSQL and Redis
- OIDC platform login
- deeper S3 and IAM compatibility work

## Phase 2

- stronger object-lock or legal-hold style governance
- webhook retries and dead-letter operator tooling
- richer worker and queue observability
- storage backend expansion and replication design work

## Release Quality Track

- broad-release execution backlog: [`release-backlog.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\release-backlog.md)
- end-goal release decision path: [`v1-release-plan.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\v1-release-plan.md)
- target `single-node` for `v1.0 GA`
- keep `distributed` as beta until distributed reliability and operations gates are passed

## Optional distributed storage

- add an opt-in distributed blob backend selected with `STORAGE_BACKEND=distributed`
- keep local encrypted filesystem storage as the default
- model storage nodes and object placement in PostgreSQL
- add worker-driven repair and rebalance flows
- add a separate distributed-lab Compose path instead of forcing clustering on every install
