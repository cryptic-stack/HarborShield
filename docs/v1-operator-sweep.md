# v1 Operator Manageability Sweep

This document records the final operator-manageability sweep for HarborShield `v1.0`.

Decision date:

- 2026-03-24

## Sweep Scope

The sweep covered the main operator-facing areas that could still hide runtime behaviors behind env vars, restarts, or undocumented assumptions:

- setup
- settings
- storage
- quotas
- malware
- auth and OIDC
- recovery guidance

## Conclusion

Result:

- no remaining `v1.0` blockers were found in the reviewed operator surfaces
- the remaining env-scoped settings appear intentional and acceptable as deployment-level controls
- `REL-104` can be treated as closed for the `v1.0` decision path

## Verified Operator-Manageable Areas

### Setup

Verified:

- deployment plan can be saved from the setup wizard
- single-node versus distributed-local intent is visible to the operator
- apply-required guidance is explicit instead of silent

### Settings

Verified:

- storage policy is manageable from the UI
- OIDC issuer, client ID, redirect URL, scopes, role claim, default role, role mapping, and stored secret lifecycle are manageable from the UI
- OIDC connection testing is exposed to operators
- runtime settings summary is visible in the UI

### Storage

Verified:

- distributed node registration is manageable from the UI
- node operator state changes are manageable from the UI
- TLS identity re-pin is manageable from the UI when applicable
- local-to-distributed migration is manageable from the UI
- migration backlog, bytes, history, and drain-complete signals are visible
- the last healthy active node safeguard is surfaced in both UI and API behavior

### Quotas

Verified:

- quota data loads through the admin surface
- bucket and user quota controls are exposed through the quotas workflow rather than hidden config edits

### Malware

Verified:

- malware mode is operator-manageable from the Malware page
- malware state is visible to the operator

### Auth And OIDC

Verified:

- bootstrap password change exists
- logout-all exists
- OIDC readiness is visible from the login flow
- OIDC provider configuration is manageable from Settings

## Accepted Deployment-Scoped Controls

The following still rely on deployment configuration rather than live UI mutation, and that is acceptable for `v1.0`:

- `JWT_SECRET`
- `ADMIN_IP_ALLOWLIST`
- `CORS_ORIGINS`
- `LOG_LEVEL`
- PostgreSQL and Redis connection and secret wiring
- `STORAGE_BACKEND` runtime selection
- blob-node shared-secret and TLS file mounting

These are accepted as deployment-level controls because changing them live would alter core trust boundaries, edge behavior, or runtime topology in ways that are not required for the `single-node` GA promise.

## v1 Interpretation

For `v1.0`, HarborShield should treat the reviewed admin surfaces as operator-complete enough for the signed GA scope.

The remaining env-based controls should be documented as deployment inputs, not product gaps.

## Exit Effect

Publishing this sweep closes the final operator-manageability review for the `v1.0` path.

After this document, the remaining release decision is straightforward:

- decide whether `v0.1.0-rc4` is the final release candidate
- or cut one more prerelease if you want the signed `v1.0` decision docs bundled into a fresh tagged artifact
