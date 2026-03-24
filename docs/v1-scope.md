# v1 Scope Decision

This document is the release-scope decision for HarborShield `v1.0`.

Decision date:

- 2026-03-24

Release intent:

- HarborShield `v1.0` targets a broad-release-quality `single-node` deployment
- `distributed` remains a documented and validated `beta` capability, not part of the `v1.0` GA promise

## GA Scope

The `v1.0` GA promise covers:

- single-host Docker Compose deployment
- first-run bootstrap, password rotation, and setup flow
- local encrypted object storage
- core admin workflows:
  - users and credentials
  - buckets and objects
  - quotas
  - malware scan mode
  - audit visibility
  - backup and restore for the documented single-node path
- the S3-compatible surface documented in [`s3-compatibility.md`](./s3-compatibility.md) as `Supported`
- the release bundle workflow:
  - published images
  - published deployment bundle
  - published checksums

## Beta Scope

The following are intentionally not part of the `v1.0` GA promise:

- distributed blob storage
- live node admission and local-to-distributed migration
- distributed recovery beyond operator-managed beta guidance
- deeper distributed repair and rebalance automation
- full AWS IAM or S3 parity beyond the documented compatibility contract
- future storage backends
- advanced federation or post-`v1.0` clustering work

These may ship in the same repository and release line, but they must remain labeled `beta`, `partial`, or `not supported` where appropriate.

## Support-Level Rules

For `v1.0` communication:

- `single-node` may be described as `GA`
- `distributed` must be described as `beta`
- unsupported S3 APIs must not be described as part of the `v1.0` contract
- partial S3 behaviors must retain caveats and must not be described as full Amazon S3 parity

## Known v1 Boundaries

`v1.0` does not claim:

- virtual-host-style S3 addressing
- full AWS IAM compatibility
- STS temporary credentials
- bucket tagging APIs
- Glacier-style restore APIs
- multipart listing APIs
- full object-lock API parity
- distributed GA support

## Evidence Basis

This scope decision is grounded in the current release evidence:

- published-bundle acceptance for `v0.1.0-rc4`
- published-bundle backup and restore evidence
- hosted CI and release validation
- hosted distributed beta migration validation

References:

- [`release-acceptance/v0.1.0-rc4.md`](./release-acceptance/v0.1.0-rc4.md)
- [`release-notes/v0.1.0-rc4.md`](./release-notes/v0.1.0-rc4.md)
- [`release-blockers.md`](./release-blockers.md)
- [`s3-compatibility.md`](./s3-compatibility.md)

## Exit Effect

Publishing this scope note closes the scope-definition part of the `v1.0` path.

After this document, the remaining release-decision work is:

- sign off the S3 compatibility contract
- complete the final operator-manageability sweep
- decide whether `rc4` is the final candidate or whether one more prerelease is needed to carry final sign-off docs
