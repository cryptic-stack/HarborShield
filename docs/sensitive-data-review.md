# Sensitive Data Review

This document summarizes the current HarborShield review for sensitive-data exposure across logs, audit records, support bundles, and UI responses.

## Reviewed Areas

- API request logging
- audit-log sanitization
- support-bundle collection
- settings and auth responses returned to the admin UI
- credential and OIDC workflows

## Current Safe Defaults

### Logs

- request logs record method, path, request ID, outcome, and latency
- request bodies and auth headers are not logged by the request logger

### Audit

- audit payloads are sanitized before they are returned through the API
- keys containing:
  - `secret`
  - `password`
  - `token`
  - `authorization`
  - `cookie`
  - `session`
  - `ciphertext`
  - `privatekey`
- are redacted
- `*Configured` booleans remain visible so operators can still reason about state without seeing the secret itself

### Settings And OIDC

- OIDC client secret is stored encrypted at rest
- OIDC client secret is never returned to the browser after save
- the UI only receives `clientSecretConfigured`

### Credentials

- S3 secret keys are shown only at creation time
- credential listings do not return stored secret values
- audit entries for credential creation record the access key and metadata, not the secret key

### Support Bundle

- the bundle captures authenticated admin snapshots, but not bearer tokens themselves
- settings summaries include posture fields, not secret values
- audit summaries are derived from sanitized audit API output

## Remaining Boundary

The main remaining boundary is browser-side session storage:

- the admin UI currently stores the access token and refresh token in `localStorage`

That is workable for the current product stage, but it is still the biggest remaining sensitive-data exposure surface in the admin experience. A future hardening pass should evaluate:

- moving refresh tokens out of browser-readable storage
- using secure HTTP-only cookies for session continuation
- reducing token lifetime further if browser storage remains in use

## Review Outcome

Current status:

- no obvious secret leakage path was found in normal logs, audit output, or support bundles
- audit masking is now test-backed
- the remaining notable risk is browser-side token storage, not server-side redaction

## Related Guides

- [audit-coverage.md](./audit-coverage.md)
- [operations.md](./operations.md)
- [security.md](./security.md)
