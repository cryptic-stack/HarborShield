# First Run Guide

This guide walks through the first operator experience from a clean HarborShield deployment through initial platform setup.

Release walkthrough checklist:

- [`docs/first-run-checklist.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\first-run-checklist.md)

## Before you start

1. Copy `.env.example` to `.env`.
2. Rotate all required secrets:
   - `POSTGRES_PASSWORD`
   - `REDIS_PASSWORD`
   - `JWT_SECRET`
   - `ADMIN_BOOTSTRAP_PASSWORD`
   - `STORAGE_MASTER_KEY`
   - `STORAGE_NODE_SHARED_SECRET` if you plan to use distributed storage
3. Start the stack:

```powershell
docker compose --env-file .env up --build -d
```

4. Confirm the stack is healthy:

```powershell
docker compose ps
curl http://localhost/healthz
```

Expected result:
- `api`, `worker`, `frontend`, `postgres`, `redis`, and `caddy` are healthy
- `/healthz` returns `{"status":"ok"}`

## First login

1. Open `http://localhost`.
2. Sign in with the bootstrap admin from `.env`:
   - email: `ADMIN_BOOTSTRAP_EMAIL`
   - password: `ADMIN_BOOTSTRAP_PASSWORD`
3. On first login, HarborShield forces a password change.

That flow is intentional and should be treated as required bootstrap hardening, not optional setup.

## Deployment setup wizard

After the password change, HarborShield opens the first-run deployment wizard.

The wizard asks:

1. `Single Node` or `Distributed`
2. If `Distributed`, `Local Nodes` or `Remote Nodes`
3. If `Remote Nodes`, one storage endpoint per line

### Choosing Single Node

Choose this when:
- you want the default homelab or SMB deployment
- all blobs should stay on the local encrypted filesystem backend
- you do not need multiple blob nodes yet

Behavior:
- `STORAGE_BACKEND` remains `local`
- object data stays on the main host under `STORAGE_ROOT`
- no additional blob-node profile is needed

### Choosing Distributed: Local Nodes

Choose this when:
- you want to test multi-node placement on one Docker host
- you plan to run HarborShield with the Compose distributed profile

Behavior:
- the wizard records that you want distributed mode
- HarborShield will keep you on the setup screen and show the exact apply command until the runtime matches
- use the recommended profile command from the setup screen

Typical follow-up:

```powershell
docker compose --profile distributed --env-file .env up --build -d
```

### Choosing Distributed: Remote Nodes

Choose this when:
- the blob nodes will not run inside the same local Compose stack
- you want the control plane and storage nodes separated

You will be asked for remote node endpoints, for example:

```text
https://blobnode-a.example.internal:9100
https://blobnode-b.example.internal:9100
https://blobnode-c.example.internal:9100
```

Behavior:
- the wizard stores the intended remote topology in the control plane
- HarborShield will validate every endpoint as a full HTTP or HTTPS URL and keep the apply instructions visible until the runtime configuration matches

## What to configure next

After first run, the most important admin tasks are:

1. Review `Settings`
   - confirm region, tenant, upload limits, encryption, and storage mode
2. Review `Storage`
   - if using distributed mode, confirm node health, placement policy, and operator state
3. Review `Roles` and `Policy Lab`
   - verify who can administer the platform
4. Review `Quotas`
   - set bucket and user limits before broad onboarding
5. Review `Audit`
   - verify login and settings changes are recorded

## Optional OIDC setup after first login

OIDC no longer has to be configured only in `.env`.

You can now configure it from `Settings` after first login:
- enable or disable OIDC
- set issuer URL
- set client ID
- set redirect URL
- set scopes
- set role-claim mapping
- store or replace the client secret securely
- test provider discovery
- clear the stored client secret

Recommended flow:

1. Save the OIDC settings
2. Use `Test Connection`
3. Confirm the expected issuer metadata
4. Only then enable it for daily use

## First S3 validation

After admin setup, validate the object plane:

1. Create a bucket from the UI or admin API
2. Create an S3 credential
3. Upload and download an object
4. Confirm the action appears in `Audit`

You can also run the smoke scripts under [`scripts/`](c:\Users\JBrown\Documents\Project\s3-platform\scripts).

## First distributed validation

If you chose distributed mode, validate these next:

1. `Storage` page shows the expected nodes
2. Nodes are `healthy`
3. Placements are being created for uploaded objects
4. Dashboard shows no offline nodes or degraded placements

If you are using blob-node enrollment:

1. Create a join token from the admin API
2. Start the blob node with its join token
3. Verify the node appears in `Storage`
4. If mTLS is enabled, verify TLS identity status

## Troubleshooting first run

### Login works, but the page looks blank

- hard refresh the browser once
- confirm `frontend` and `caddy` are healthy in `docker compose ps`

### First-run wizard says apply is required

That means the saved desired storage mode does not match the currently running stack yet. Follow the recommended runtime command shown by the setup screen.

### OIDC test fails

Check:
- issuer URL
- redirect URL
- client secret
- outbound network access to the provider discovery endpoint

### Distributed nodes do not appear

Check:
- storage endpoints
- node join token usage
- `STORAGE_NODE_SHARED_SECRET`
- optional mTLS configuration and certificates

## After first run

Once first-run setup is complete, use [`operations.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\operations.md) as the day-2 operator guide.
