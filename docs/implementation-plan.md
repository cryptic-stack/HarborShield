# Implementation Plan

This document turns the project brief into a phased delivery program for HarborShield.

## Milestone 0: Foundation Hardening

Goals:

- clean repository hygiene
- deterministic local bootstrap
- fail-fast startup validation
- repeatable smoke verification

Backlog:

- add `.gitignore` and remove generated artifacts from the working tree
- validate required secrets and reject placeholder defaults outside development
- validate storage path and core port settings on startup
- add a smoke-test script covering login, bucket create/list, credential create, object upload/download, and audit fetch
- document smoke-test and bootstrap flow in `README.md`

Exit criteria:

- clean workspace after build
- startup fails clearly on invalid config
- one-command smoke verification exists

## Milestone 1: Correct S3 Core

Goals:

- production-correct behavior for currently supported object APIs
- stronger request validation and safer defaults

Backlog:

- replace custom S3 auth with SigV4 verification for supported operations
- normalize S3 XML errors and status codes
- enforce request size limits and content-length rules
- block non-empty bucket deletion
- improve `HeadObject` and metadata header handling
- complete multipart upload create/upload/complete/abort flows
- add more S3 compatibility tests and unsupported-feature documentation

Exit criteria:

- supported bucket/object flows use real request signing and predictable semantics

## Milestone 2: Authorization Control Plane

Goals:

- real roles, policies, and deny-by-default enforcement

Backlog:

- add DB tables and services for roles, policy statements, and subject bindings
- enforce policy evaluation on admin API and S3 API routes
- implement built-in roles: `superadmin`, `admin`, `auditor`, `bucket-admin`, `readonly`
- add admin API tokens distinct from UI session JWTs
- expose roles and policies via admin API

Exit criteria:

- every privileged action is policy-gated

## Milestone 3: Complete Admin API

Goals:

- admin automation surface aligned to the original brief

Backlog:

- add endpoints for roles, policies, quotas, event targets, malware status, settings, and admin API tokens
- add pagination and filtering for audit logs
- scaffold OpenAPI output for the admin API
- implement password-change flow for bootstrap admin

Exit criteria:

- admin automation does not depend on undocumented routes

## Milestone 4: Worker Execution Layer

Goals:

- real idempotent background jobs

Backlog:

- job polling and leasing model
- multipart cleanup
- soft-delete purge
- lifecycle expiration
- quota recalculation
- retention checks
- event delivery retries and dead-letter tracking

Exit criteria:

- worker processes durable job records with metrics and retry safety

## Milestone 5: Governance And Observability

Goals:

- better-than-lightweight auditability and quota visibility

Backlog:

- enforce per-bucket and per-user quotas
- add bytes stored and object count accounting
- searchable audit trail with filters and pagination
- richer structured logs with actor, bucket, object key, action, and latency
- add queue depth, multipart count, bucket count, and worker throughput metrics

Exit criteria:

- operators can answer who did what, what is consuming capacity, and whether the system is healthy

## Milestone 6: Admin UX Completion

Goals:

- UI becomes a real operations surface

Backlog:

- bucket detail page
- upload manager
- credentials page
- roles and policies page
- quotas page
- events/webhooks page
- system health page
- settings page
- safer destructive prompts and activity summaries

Exit criteria:

- common operational tasks can be performed entirely from the UI

## Milestone 7: Advanced Data Controls

Goals:

- versioning, retention, and malware pipeline integration

Backlog:

- object versioning
- restore workflow
- retention and immutability hooks
- ClamAV scanning modes
- scan state surfacing in audit and UI
- copy object and object tagging

Exit criteria:

- platform supports richer object governance and upload pipeline controls

## Recommended Order

1. Milestone 0
2. Milestone 1
3. Milestone 2
4. Milestone 4
5. Milestone 3
6. Milestone 5
7. Milestone 6
8. Milestone 7

## Immediate Next Slice

Begin with Milestone 0:

- repository hygiene
- startup validation
- smoke script
- README updates
