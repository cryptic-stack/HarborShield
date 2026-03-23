# Session Validation

This guide covers the current release validation for HarborShield session and refresh-token lifecycle behavior.

## Goal

Prove that HarborShield handles concurrent sessions safely enough for broad-release operation.

## Automated Smoke

Run:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\release-session-regression-smoke.ps1
```

Or:

```powershell
make release-session-regression-smoke
```

## What The Smoke Validates

The smoke performs this flow:

1. reset to a clean stack
2. bootstrap and rotate the admin password
3. create two concurrent admin sessions
4. refresh one session and verify refresh-token rotation
5. verify the old refresh token is rejected after rotation
6. log out the second session and verify its refresh token is rejected
7. create a third session
8. call `logout-all`
9. verify the remaining refresh tokens are rejected
10. verify a brand-new login still succeeds after `logout-all`

At the end, the script resets the environment back to first-run baseline.

## Current Scope

This validation currently proves revocation behavior for refresh tokens.

It does not yet prove immediate invalidation of already-issued access tokens, because HarborShield currently treats access tokens as short-lived bearer tokens that expire naturally rather than consulting a live revocation store on every request.

## Why This Matters

For release quality, operators need confidence that:

- refresh tokens rotate correctly
- refresh-token reuse is rejected
- logging out one session does not accidentally preserve it
- `logout-all` actually clears the user's outstanding refresh-token sessions

## Related Guides

- [worker-restart-validation.md](./worker-restart-validation.md)
- [upgrade-validation.md](./upgrade-validation.md)
- [operations.md](./operations.md)
