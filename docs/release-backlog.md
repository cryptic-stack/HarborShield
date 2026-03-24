# Release Backlog

This document turns HarborShield's broad-release plan into an execution backlog with phases, epics, actions, and go or no-go gates.

Current recommendation:

- `single-node`: target for broad release quality and `v1.0 GA`
- `distributed`: keep as `beta` until its own operational gate is fully passed
- track current go or no-go blockers in [`release-blockers.md`](./release-blockers.md)
- track prerelease evidence and operator-facing notes in [`release-notes/v0.1.0-rc4.md`](./release-notes/v0.1.0-rc4.md)

## Release Objectives

Broad release means:

- first install succeeds cleanly
- first-run setup is intuitive and documented
- upgrades are repeatable and safe
- backup and restore are documented and verified
- core admin workflows are stable
- core S3 workflows are predictable and documented
- security defaults are sane and reviewable
- observability and troubleshooting are usable by operators
- no open critical blockers

## Phase 1: Release Scope Freeze

Goal:

- define what `v1.0` includes and what it explicitly does not include

Epics:

- release scope definition
- support-level classification
- blocker tracking

Actions:

- lock `single-node` as GA candidate
- classify `distributed` as beta unless all distributed gates pass
- classify advanced federation, deeper IAM parity, and future storage backends as post-`v1.0`
- define `must-have`, `should-have`, and `deferred` features
- create a release blocker register with owner, severity, evidence, and target fix milestone

Exit criteria:

- one written `v1.0` scope
- all non-`v1.0` work labeled clearly
- release blockers tracked in one place

## Phase 2: Install, Upgrade, And Recovery Reliability

Goal:

- prove the product can be installed, restarted, upgraded, and recovered predictably

Epics:

- clean install validation
- upgrade validation
- backup and restore validation
- worker and migration resilience

Actions:

- add a clean-install smoke that starts from empty Docker volumes
- add an upgrade smoke from previous schema/data state to current code
- add explicit migration version checks and failure guidance
- document and validate Postgres backup
- document and validate object-data backup
- document restore order for metadata and blobs
- test worker behavior after abrupt restart during active jobs
- test API, worker, and frontend restart behavior independently

Exit criteria:

- clean install test passes
- upgrade test passes
- restore test passes
- migration failure path is documented

Suggested tickets:

- `REL-001` clean install smoke suite
- `REL-002` upgrade validation script
- `REL-003` backup and restore runbook
- `REL-004` worker restart resilience test

## Phase 3: Security Hardening And Review

Goal:

- close the remaining security gaps that block broad operator trust

Epics:

- secret lifecycle
- session lifecycle
- authn and authz review
- deployment hardening

Actions:

- document and test rotation workflow for:
  - `JWT_SECRET`
  - `OIDC_CLIENT_SECRET`
  - `STORAGE_NODE_SHARED_SECRET`
  - admin tokens
- review refresh-token and logout-all behavior under concurrent sessions
- verify audit coverage for all sensitive settings and auth actions
- review admin IP allowlist behavior and document safe usage
- document secure deployment expectations for Postgres and Redis encryption
- verify secret masking in logs, audit, and UI
- add release security checklist

Exit criteria:

- secret rotation is documented for core secrets
- auth failure and revocation flows are predictable
- sensitive values do not leak in UI, logs, or audit

Suggested tickets:

- `SEC-001` secret rotation runbooks
- `SEC-002` session revocation regression suite
- `SEC-003` sensitive-data exposure review
- `SEC-004` deployment encryption guidance

## Phase 4: Product And Operator UX Completion

Goal:

- make the platform feel intentional, consistent, and trustworthy for real operators

Epics:

- first-run polish
- settings and recovery polish
- error-message standardization
- admin workflow cleanup

Actions:

- run a full first-run walkthrough and close all UX friction points
- standardize wording and capitalization across the admin UI
- tighten empty states and destructive-action prompts
- improve error states for:
  - OIDC misconfiguration
  - storage degradation
  - webhook delivery failure
  - quota denial
- add success feedback for all major admin actions
- add visual guidance for GA vs beta features

Exit criteria:

- first-run flow is smooth
- common admin workflows require no guesswork
- major failure states are understandable

Suggested tickets:

- `UX-001` first-run walkthrough fix list
- `UX-002` admin copy and terminology sweep
- `UX-003` destructive action confirmation review
- `UX-004` feature support labeling

## Phase 5: S3 Compatibility Contract And Validation

Goal:

- define and validate the exact S3 compatibility promise for release

Epics:

- compatibility contract
- regression coverage
- client validation

Actions:

- publish the supported S3 surface as a release contract
- document unsupported or partial behaviors clearly
- add automated validation against:
  - AWS SDK flows you intend to support
  - common path-style clients
  - presigned `GET` and `PUT`
  - multipart flows
  - versioning and delete-marker flows
- verify exact behavior for:
  - non-empty bucket delete
  - version-aware cleanup
  - tagging
  - copy object
  - policy evaluation on object access

Exit criteria:

- supported S3 APIs are documented and regression-tested
- no hidden compatibility claims remain

Suggested tickets:

- `S3-001` published compatibility matrix
- `S3-002` SDK regression suite
- `S3-003` multipart and versioning edge-case suite
- `S3-004` policy compatibility sweep

## Phase 6: Operations And Observability

Goal:

- give operators enough visibility to run and troubleshoot the platform confidently

Epics:

- health and metrics coverage
- audit usability
- support bundle and diagnostics

Actions:

- expand metrics documentation and operator guidance
- add support bundle collection for:
  - health endpoints
  - Compose status
  - API and worker logs
  - migration state
  - selected settings summary
- review audit coverage and volume for noisy or missing events
- add release troubleshooting guide for:
  - login issues
  - OIDC issues
  - storage issues
  - quota issues
  - webhook issues

Exit criteria:

- operators can collect useful diagnostics without digging through the code
- major operational problems have documented first-response steps

Suggested tickets:

- `OPS-001` support bundle script
- `OPS-002` troubleshooting guide
- `OPS-003` metrics and alerting reference
- `OPS-004` audit coverage review

## Phase 7: Documentation Completion

Goal:

- make docs sufficient for an external operator to succeed without handholding

Epics:

- install and first-run docs
- day-2 operations docs
- security docs
- compatibility docs

Actions:

- keep `README.md`, `docs/first-run.md`, `docs/operations.md`, `docs/deployment.md`, `docs/security.md`, and `docs/api.md` aligned
- add screenshots for:
  - first-run wizard
  - settings
  - storage
  - audit
  - uploads
- add backup and restore walkthroughs
- add upgrade notes template
- finalize known-issues and support-level notes

Exit criteria:

- docs are coherent and operator-complete
- GA and beta feature status is obvious

Suggested tickets:

- `DOC-001` screenshot pass
- `DOC-002` backup and restore walkthrough
- `DOC-003` upgrade notes template
- `DOC-004` known issues and support-level matrix

## Phase 8: Distributed Beta Gate

Goal:

- decide whether distributed mode remains beta or is promoted later

Epics:

- distributed operations proof
- distributed recovery proof
- distributed operator docs

Actions:

- validate node admission, drain, repair, rebalance, and TLS identity workflows repeatedly
- document distributed topology recommendations
- document distributed failure and recovery actions
- verify operator state transitions under degraded node conditions
- verify storage policy changes under active distributed load
- keep a public regression helper for live migration from local objects into the active distributed node set

Exit criteria:

- distributed mode has its own reliability evidence
- distributed operator docs are complete
- support level is explicit

Suggested tickets:

- `DST-001` distributed recovery regression suite
- `DST-002` distributed operator runbook
- `DST-003` distributed topology recommendations
- `DST-004` beta promotion criteria
- public live-migration regression helper: [`scripts/distributed-migration-smoke.sh`](c:\Users\JBrown\Documents\Project\s3-platform\scripts\distributed-migration-smoke.sh)

## Go Or No-Go Checklist For v1.0

Release only if all are true:

- clean install passes
- first-run guide matches actual behavior
- backup and restore were validated on a clean environment
- upgrade test passes
- core security checklist passes
- key admin workflows pass manual and automated validation
- S3 compatibility contract is published and regression-tested
- no critical blockers remain
- distributed support level is labeled clearly

## Recommended Execution Order

1. Phase 1
2. Phase 2
3. Phase 3
4. Phase 4
5. Phase 5
6. Phase 6
7. Phase 7
8. Phase 8

## Immediate Starting Tranche

Start here:

1. `REL-001` clean install smoke suite
2. `REL-003` backup and restore runbook
3. `SEC-001` secret rotation runbooks
4. `S3-001` published compatibility matrix
5. `UX-001` first-run walkthrough fix list

These are the highest-leverage items for turning HarborShield from a strong project into a broad-release-quality platform.

## Current Execution Entry Point

The repo now includes a consolidated release gate runner:

- [`scripts/release-readiness.ps1`](c:\Users\JBrown\Documents\Project\s3-platform\scripts\release-readiness.ps1)

Suggested usage:

- default run for fast local validation:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\release-readiness.ps1
```

- fuller pre-release run:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\release-readiness.ps1 `
  -IncludeReleaseSmokes `
  -IncludeResilience `
  -IncludeS3Regression
```

This does not replace the written go or no-go checklist above, but it gives operators and maintainers one command that reflects the current automated gate.

GitHub workflow split:

- fast CI workflow: [`.github/workflows/ci.yml`](c:\Users\JBrown\Documents\Project\s3-platform\.github\workflows\ci.yml)
  - push and pull-request gate
  - validates Compose config, backend tests, and frontend production build
- deeper release workflow: [`.github/workflows/release-validation.yml`](c:\Users\JBrown\Documents\Project\s3-platform\.github\workflows\release-validation.yml)
  - manual `workflow_dispatch` run
  - prepares `.env` from `.env.example`
  - can execute release smokes, resilience checks, S3 regression suites, and the distributed live-migration beta smoke before a release decision
- tagged publish workflow: [`.github/workflows/release.yml`](c:\Users\JBrown\Documents\Project\s3-platform\.github\workflows\release.yml)
  - runs the full release gate on tags matching `v*`
  - runs the distributed live-migration beta smoke before publishing
  - publishes versioned GHCR images for backend and frontend runtimes
  - attaches a deployment bundle with pinned image refs and SHA-256 checksums to the GitHub release
