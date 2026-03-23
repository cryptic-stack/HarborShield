# Gap Remediation Plan

This document turns the current gap analysis into a concrete follow-on backlog for HarborShield.

## Baselines

- Original project brief: production-minded, Docker-first S3 platform with stronger governance and admin UX than lightweight object stores
- Competitive baseline: MinIO's current documented capabilities around identity, governance, eventing, and operations

## Priority Themes

1. Contract clarity and documentation accuracy
2. Identity and access maturity
3. Security hardening
4. S3 and IAM compatibility depth
5. Governance and data controls
6. Eventing and observability depth
7. Storage platform expansion

## Theme 1: Contract Clarity And Documentation Accuracy

Why it matters:

- The codebase has outgrown its original MVP docs.
- Operators and future contributors need an accurate contract before deeper feature work lands cleanly.

Backlog:

- reconcile `README.md`, `docs/api.md`, `docs/security.md`, `docs/deployment.md`, and `docs/roadmap.md` with the current implemented platform
- keep an explicit list of supported and unsupported S3 APIs
- publish a versioned admin API contract in OpenAPI form
- add a simple generated-doc serving plan for local development and CI export

Exit criteria:

- documentation matches the live admin and S3 surfaces
- OpenAPI is checked in and updated with API changes

## Theme 2: Identity And Access Maturity

Why it matters:

- This is the largest functional gap against both the original brief and MinIO-class deployments.

Backlog:

- implement OIDC login for platform auth
- add provider configuration and callback handling
- support role mapping from OIDC claims
- add short-lived admin sessions and clearer token rotation or revocation semantics
- design temporary S3 credentials or session credentials for future federation

Exit criteria:

- admins can authenticate through OIDC without replacing the local bootstrap path

## Theme 3: Security Hardening

Why it matters:

- The platform is security-minded, but some defaults and recovery behaviors still need polishing.

Backlog:

- auto-logout and redirect on invalid admin tokens
- bootstrap password change flow on first login
- support secret-file loading for Docker deployments
- tighten session parsing and stale-session handling across the UI
- document encrypted deployment patterns for PostgreSQL and Redis
- add optional admin IP allowlist enforcement
- clean up stale security documentation around presigning and token behavior

Exit criteria:

- the common failure paths are recoverable and secure by default

## Theme 4: S3 And IAM Compatibility Depth

Why it matters:

- HarborShield is already practical, but compatibility depth is still meaningfully behind MinIO.

Backlog:

- document supported SigV4 scope and unsupported cases clearly
- improve S3 error and header parity further
- add bucket policy API compatibility where practical
- deepen IAM-style semantics and conditions
- improve multipart edge handling and compatibility coverage

Exit criteria:

- supported APIs behave predictably with explicit compatibility guarantees

## Theme 5: Governance And Data Controls

Why it matters:

- This is HarborShield's best path to differentiation.

Backlog:

- strengthen retention into object-lock style semantics
- add legal-hold style controls
- improve version restore UX and APIs
- expand quota reporting and policy conflict visibility
- provide immutable-style audit export options

Exit criteria:

- governance workflows feel safer and more complete than lightweight object stores

## Theme 6: Eventing And Observability Depth

Why it matters:

- Operators need more than basic health and recent delivery visibility.

Backlog:

- add event delivery retry controls and dead-letter inspection to the UI
- expose richer worker and queue metrics
- add more structured logging context for policy decisions and worker outcomes
- consider OpenTelemetry hooks for tracing

Exit criteria:

- operators can troubleshoot delivery failures, worker backlog, and policy denials quickly

## Theme 7: Storage Platform Expansion

Why it matters:

- The current filesystem backend is good for the first target users, but future growth needs a cleaner expansion path.

Backlog:

- formalize the pluggable backend contract
- add a second backend implementation after the interface is tightened
- design replication and storage tiering without overcommitting to distributed complexity

Exit criteria:

- storage backends can evolve without rewriting metadata and policy layers

## Recommended Execution Order

1. Theme 1
2. Theme 3
3. Theme 2
4. Theme 4
5. Theme 5
6. Theme 6
7. Theme 7

## Immediate Next Slice

- finish the OpenAPI scaffold and keep it in sync with route changes
- implement automatic stale-session recovery in the UI
- add the bootstrap password-change flow
