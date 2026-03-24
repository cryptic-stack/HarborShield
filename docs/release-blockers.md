# Release Blockers

This register tracks the remaining blockers between the current HarborShield prerelease line and a broad `v1.0` release decision.

Current planning reference:

- [`v1-release-plan.md`](./v1-release-plan.md)

Status rules:

- `open`: unresolved and currently blocks `v1.0`
- `watch`: not a hard blocker yet, but must be reviewed before GA
- `closed`: resolved with evidence linked here

## Active Blockers

| ID | Severity | Status | Area | Summary | Owner | Evidence | Target |
| --- | --- | --- | --- | --- | --- | --- | --- |
| REL-102 | high | closed | scope | Publish explicit GA scope and known issues for `single-node`, while keeping `distributed` clearly beta | maintainer | Signed scope note published in [`v1-scope.md`](./v1-scope.md) | before `v1.0.0` |
| REL-103 | high | closed | compatibility | Sign off the S3 compatibility contract as the release promise for GA | maintainer | Signed contract published in [`v1-s3-contract.md`](./v1-s3-contract.md) | before `v1.0.0` |
| REL-104 | high | watch | operations | Confirm no remaining runtime behaviors are visible but not operator-manageable where needed | maintainer | Malware mode is now manageable from the UI; remaining operational settings need one final sweep | before `v1.0.0` |

## Closed Items

| ID | Closed On | Summary | Evidence |
| --- | --- | --- | --- |
| REL-100 | 2026-03-24 | Published-bundle operator acceptance passed for `v0.1.0-rc4`, including install, first-run, core S3, distributed beta workflow, backup, and restore evidence | [`docs/release-acceptance/v0.1.0-rc4.md`](./release-acceptance/v0.1.0-rc4.md) |
| REL-101 | 2026-03-24 | Published-bundle backup and restore evidence was captured and verified against `v0.1.0-rc4` | [`docs/release-acceptance/v0.1.0-rc4.md`](./release-acceptance/v0.1.0-rc4.md), [`docs/backup-restore.md`](./backup-restore.md) |
| REL-102 | 2026-03-24 | `v1.0` GA scope was signed off with `single-node` as GA and `distributed` remaining beta | [`docs/v1-scope.md`](./v1-scope.md) |
| REL-103 | 2026-03-24 | `v1.0` S3 compatibility promise was signed off as a practical, non-parity contract | [`docs/v1-s3-contract.md`](./v1-s3-contract.md), [`docs/s3-compatibility.md`](./s3-compatibility.md) |
| REL-090 | 2026-03-23 | Fast CI and deep release validation both pass on GitHub-hosted Linux runners | [`.github/workflows/ci.yml`](../.github/workflows/ci.yml), [`.github/workflows/release-validation.yml`](../.github/workflows/release-validation.yml) |
| REL-091 | 2026-03-23 | Tagged prerelease publishing works end to end, including GHCR images and GitHub release assets | [`.github/workflows/release.yml`](../.github/workflows/release.yml), [`docs/release-notes/v0.1.0-rc1.md`](./release-notes/v0.1.0-rc1.md) |
| REL-092 | 2026-03-23 | Malware scan mode is operator-manageable from the Malware section and persisted as a cluster setting | [`frontend/src/pages/MalwarePage.tsx`](../frontend/src/pages/MalwarePage.tsx), [`backend/internal/settings/service.go`](../backend/internal/settings/service.go) |

## Exit Rule For v1.0

Do not cut `v1.0.0` until every `critical` blocker is closed and every `high` item is either closed or explicitly accepted in signed release notes.
