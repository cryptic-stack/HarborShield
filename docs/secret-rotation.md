# Secret Rotation

This runbook covers the primary HarborShield secrets that operators should know how to rotate before broad release.

## Covered secrets

- `JWT_SECRET`
- `OIDC_CLIENT_SECRET`
- `STORAGE_NODE_SHARED_SECRET`
- admin API tokens

Related but separate:

- `STORAGE_MASTER_KEY`
- `POSTGRES_PASSWORD`
- `REDIS_PASSWORD`
- `ADMIN_BOOTSTRAP_PASSWORD`

`STORAGE_MASTER_KEY` is intentionally not treated as a routine rotation target yet because it protects encrypted object blobs at rest and would require a controlled re-encryption workflow.

## General rotation principles

For every secret rotation:

1. make a backup first
2. know whether the change is live or restart-required
3. revoke or replace the old secret deliberately
4. verify the result
5. record the change in `Audit`

## JWT secret rotation

What it affects:

- admin access tokens
- refresh tokens
- OIDC callback state signing

Current expectation:

- rotation is deployment-driven through `.env`
- existing tokens should be considered invalid after rotation

Suggested workflow:

1. generate a new strong `JWT_SECRET`
2. update `.env`
3. restart the stack:

```powershell
docker compose --env-file .env up --build -d
```

4. force operators to sign in again
5. verify:
   - login works
   - refresh works for newly issued sessions
   - stale sessions are rejected cleanly

## OIDC client secret rotation

What it affects:

- provider-backed admin login

Current expectation:

- rotation can be done from `Settings`
- the secret is stored encrypted at rest

Workflow:

1. open `Settings`
2. enter the new `Client Secret`
3. save OIDC settings
4. click `Test Connection`
5. verify the audit trail

If you want to remove the current provider secret first:

1. open `Settings`
2. click `Clear Stored Secret`
3. verify the client secret status changed
4. save the replacement secret
5. test the provider connection again

## Storage node shared secret rotation

What it affects:

- distributed blob-node RPC authentication when using the shared-secret path

Current expectation:

- this is deployment-driven through `.env`
- distributed nodes and the control plane must agree on the new value

Suggested workflow:

1. generate a new `STORAGE_NODE_SHARED_SECRET`
2. update the control plane environment
3. update blob-node environments
4. restart control-plane and blob-node services in a coordinated window
5. verify:
   - node health returns to normal
   - storage operations succeed
   - no RPC auth failures remain

If you rely primarily on issued per-node RPC tokens and mTLS, still keep the shared secret rotated and controlled because it remains part of the broader trust model.

## Admin API token rotation

What it affects:

- automation using admin-plane tokens

Current expectation:

- tokens are created in the admin plane
- they can be replaced by issuing new ones and removing old ones from use

Suggested workflow:

1. create a replacement admin token
2. update automation to use the new token
3. validate the automation path
4. retire the old token from the workflow

Before broad release, HarborShield should add a stronger explicit token revocation lifecycle in the UI and docs if any gaps remain.

## Bootstrap admin password rotation

What it affects:

- initial admin access

Current expectation:

- the first-run flow forces password change on first login

For emergency reset:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\reset-bootstrap-admin.ps1
```

That should be treated as a recovery operation, not normal rotation.

## Verification checklist after any rotation

At minimum verify:

- relevant login or API path succeeds with the new secret
- the old secret no longer works where appropriate
- `Audit` shows the change
- `Health` remains normal

## Release-readiness note

Before broad release, these runbooks should be backed by:

- scripted verification where practical
- screenshots for admin-plane rotation flows
- a support note on blast radius and rollback for each secret type
