# Deployment

The default deployment path is Docker Compose with Caddy as the only public listener.

1. Copy `.env.example` to `.env`
2. Rotate all secrets
3. Run `docker compose --env-file .env up --build`
4. Persist named volumes for PostgreSQL, Redis, Caddy, and object data

Notes:

- `api`, `worker`, `postgres`, and `redis` live on an internal network
- `frontend` and `api` are reachable by `caddy` on the edge network
- optional profiles: `clamav`, `prometheus`
- long-running services now default to `restart: unless-stopped`, `no-new-privileges`, and bounded Docker log rotation
- image references are env-configurable through `POSTGRES_IMAGE`, `REDIS_IMAGE`, `CADDY_IMAGE`, `CLAMAV_IMAGE`, `PROMETHEUS_IMAGE`, and `WEBHOOK_RECEIVER_IMAGE`
- log retention defaults can be tuned with `COMPOSE_LOG_MAX_SIZE` and `COMPOSE_LOG_MAX_FILES`
- object blobs are encrypted by the application, but PostgreSQL and Redis still require host or platform encryption if you need encrypted persistent stores end to end
- admin API contract work starts in [`docs/openapi-admin.yaml`](c:\Users\JBrown\Documents\Project\s3-platform\docs\openapi-admin.yaml)
- secret-bearing settings also support Docker-style `_FILE` variants, including `POSTGRES_PASSWORD_FILE`, `REDIS_PASSWORD_FILE`, `JWT_SECRET_FILE`, `ADMIN_BOOTSTRAP_PASSWORD_FILE`, `STORAGE_MASTER_KEY_FILE`, and `STORAGE_NODE_SHARED_SECRET_FILE`
- OIDC login supports `OIDC_ENABLED`, `OIDC_ISSUER_URL`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET` or `OIDC_CLIENT_SECRET_FILE`, `OIDC_REDIRECT_URL`, `OIDC_SCOPES`, optional `OIDC_ROLE_CLAIM`, optional `OIDC_ROLE_MAP`, and `OIDC_DEFAULT_ROLE`
- Example OIDC role mapping: `OIDC_ROLE_CLAIM=groups` and `OIDC_ROLE_MAP=storage-admin=admin,auditors=auditor,readers=readonly`
- `OIDC_DEFAULT_ROLE` and every mapped role target must be one of HarborShield's built-in roles: `superadmin`, `admin`, `auditor`, `bucket-admin`, or `readonly`
- distributed mode should use a dedicated `STORAGE_NODE_SHARED_SECRET` for blob-node RPCs instead of reusing `STORAGE_MASTER_KEY`
- optional node enrollment can now be driven by `POST /api/v1/storage/join-tokens` and `POST /api/v1/internal/storage/join`

Release-readiness deployment recommendation:

- copy `.env.example` and pin any image tags you intend to support in production instead of relying on floating defaults
- keep optional profiles such as `clamav` and `prometheus` on explicit image versions during release validation
- size Docker log rotation deliberately for your host so support bundles and disk usage stay predictable

Recommended reading order:

1. [`first-run.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\first-run.md)
2. this deployment guide
3. [`operations.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\operations.md)
4. [`backup-restore.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\backup-restore.md)
5. [`secret-rotation.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\secret-rotation.md)
