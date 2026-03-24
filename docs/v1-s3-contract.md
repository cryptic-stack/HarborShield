# v1 S3 Contract

This document is the signed S3 compatibility contract for HarborShield `v1.0`.

Decision date:

- 2026-03-24

Contract basis:

- [`s3-compatibility.md`](./s3-compatibility.md)

## Contract Statement

For `v1.0`, HarborShield claims:

- a tested, practical S3-compatible surface for APIs marked `Supported` in [`s3-compatibility.md`](./s3-compatibility.md)
- documented caveated behavior for APIs marked `Partial`
- no compatibility promise for APIs marked `Not supported`

This is a practical compatibility contract, not a full Amazon S3 parity claim.

## Supported v1 Promise

The `v1.0` promise includes the following as supported behavior:

- path-style bucket operations:
  - `ListBuckets`
  - `CreateBucket`
  - `DeleteBucket` for empty buckets
  - `GetBucketPolicy`
  - `DeleteBucketPolicy`
  - `ListObjectsV2`
- object operations:
  - `PutObject`
  - `GetObject`
  - `HeadObject`
  - `DeleteObject`
  - `CopyObject`
  - `GetObjectTagging`
  - `PutObjectTagging`
- multipart operations:
  - `CreateMultipartUpload`
  - `UploadPart`
  - `CompleteMultipartUpload`
  - `AbortMultipartUpload`
- presigned URL support:
  - presigned `PUT`
  - presigned `GET`
- authentication:
  - AWS Signature Version 4 header-signed requests
  - SigV4 presigned `GET` and `PUT`
  - anonymous access only when explicitly allowed by bucket policy

## Partial v1 Promise

The following are intentionally promised only as partial behavior:

- `PutBucketPolicy`
- `ListObjectVersions`
- `DeleteObjectVersion`
- bucket policy evaluation
- signed object access decisions by bucket policy
- condition-based anonymous access
- version listing
- version-specific delete
- retention checks
- legal hold checks

For these areas, HarborShield promises practical documented behavior, not full AWS parity.

## Explicit Non-Promise Areas

`v1.0` does not promise compatibility for:

- `GetBucketVersioning`
- `PutBucketVersioning`
- bucket tagging APIs
- `DeleteObjectTagging`
- `RestoreObject`
- `SelectObjectContent`
- `ListMultipartUploads`
- `ListParts`
- presigned `DELETE`
- presigned multipart flows
- full AWS IAM compatibility
- STS temporary credentials
- virtual-host-style S3 addressing

## Interpretation Rules

When communicating `v1.0` compatibility:

- use `Supported` as the compatibility promise
- use `Partial` only with caveats
- do not imply support for `Not supported` APIs
- do not describe HarborShield as “S3-compatible” without the practical-surface qualifier
- do not describe HarborShield as full Amazon S3 parity

## Validation Basis

This contract is backed by the current regression and release-validation path:

- [`scripts/s3-api-smoke.ps1`](../scripts/s3-api-smoke.ps1)
- [`scripts/s3-sdk-smoke.ps1`](../scripts/s3-sdk-smoke.ps1)
- [`scripts/s3-edge-smoke.ps1`](../scripts/s3-edge-smoke.ps1)
- [`scripts/s3-policy-smoke.ps1`](../scripts/s3-policy-smoke.ps1)
- [`scripts/s3-policy-conditions-smoke.ps1`](../scripts/s3-policy-conditions-smoke.ps1)
- GitHub release validation and tagged release workflows

## Exit Effect

Publishing this document closes the S3-contract part of the `v1.0` path.

After this document, the remaining release-decision work is:

- complete the final operator-manageability sweep
- decide whether `v0.1.0-rc4` is the final candidate or whether one more prerelease is needed to carry final sign-off docs
