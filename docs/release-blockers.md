# Release Blockers

This register tracks the remaining blockers between the current HarborShield prerelease line and a broad `v1.0` release decision.

Status rules:

- `open`: unresolved and currently blocks `v1.0`
- `watch`: not a hard blocker yet, but must be reviewed before GA
- `closed`: resolved with evidence linked here

## Active Blockers

| ID | Severity | Status | Area | Summary | Owner | Evidence | Target |
| --- | --- | --- | --- | --- | --- | --- | --- |
| REL-100 | critical | open | install | Run a clean operator acceptance pass from the published `v0.1.0-rc1` release bundle, not from source checkout | maintainer | Release exists at `v0.1.0-rc1`, but external install acceptance is not yet recorded | before `v1.0.0` |
| REL-101 | critical | open | recovery | Validate backup and restore using the released bundle and document the exact restore evidence | maintainer | [`docs/backup-restore.md`](./backup-restore.md) exists, but release-path restore evidence is not yet captured | before `v1.0.0` |
| REL-102 | high | open | scope | Publish explicit GA scope and known issues for `single-node`, while keeping `distributed` clearly beta | maintainer | Scope is implied across docs, but not yet captured in a single signed-off release decision note | before `v1.0.0` |
| REL-103 | high | open | compatibility | Sign off the S3 compatibility contract as the release promise for GA | maintainer | [`docs/s3-compatibility.md`](./s3-compatibility.md) exists, but no release sign-off record yet | before `v1.0.0` |
| REL-104 | high | watch | operations | Confirm no remaining runtime behaviors are visible but not operator-manageable where needed | maintainer | Malware mode is now manageable from the UI; remaining operational settings need one final sweep | before `v1.0.0` |

## Closed Items

| ID | Closed On | Summary | Evidence |
| --- | --- | --- | --- |
| REL-090 | 2026-03-23 | Fast CI and deep release validation both pass on GitHub-hosted Linux runners | [`.github/workflows/ci.yml`](../.github/workflows/ci.yml), [`.github/workflows/release-validation.yml`](../.github/workflows/release-validation.yml) |
| REL-091 | 2026-03-23 | Tagged prerelease publishing works end to end, including GHCR images and GitHub release assets | [`.github/workflows/release.yml`](../.github/workflows/release.yml), [`docs/release-notes/v0.1.0-rc1.md`](./release-notes/v0.1.0-rc1.md) |
| REL-092 | 2026-03-23 | Malware scan mode is operator-manageable from the Malware section and persisted as a cluster setting | [`frontend/src/pages/MalwarePage.tsx`](../frontend/src/pages/MalwarePage.tsx), [`backend/internal/settings/service.go`](../backend/internal/settings/service.go) |

## Exit Rule For v1.0

Do not cut `v1.0.0` until every `critical` blocker is closed and every `high` item is either closed or explicitly accepted in signed release notes.
