# S3 Compatibility Matrix

This document defines HarborShield's current S3 compatibility contract for release-quality validation.

Base path:

- `/s3`

Auth modes currently supported:

- AWS Signature Version 4 header-signed requests
- SigV4 presigned `GET` and `PUT`
- anonymous access only when explicitly allowed by bucket policy

Support levels:

- `Supported`: implemented and covered by the current regression smoke
- `Partial`: implemented with practical behavior, but not full Amazon S3 parity
- `Not supported`: intentionally not part of the current release contract

Validation source:

- [`scripts/s3-api-smoke.ps1`](c:\Users\JBrown\Documents\Project\s3-platform\scripts\s3-api-smoke.ps1)
- [`scripts/s3-sdk-smoke.ps1`](c:\Users\JBrown\Documents\Project\s3-platform\scripts\s3-sdk-smoke.ps1)
- [`scripts/s3-edge-smoke.ps1`](c:\Users\JBrown\Documents\Project\s3-platform\scripts\s3-edge-smoke.ps1)
- [`scripts/s3-policy-smoke.ps1`](c:\Users\JBrown\Documents\Project\s3-platform\scripts\s3-policy-smoke.ps1)
- [`scripts/s3-policy-conditions-smoke.ps1`](c:\Users\JBrown\Documents\Project\s3-platform\scripts\s3-policy-conditions-smoke.ps1)

## Bucket APIs

| API | Status | Notes |
| --- | --- | --- |
| `ListBuckets` | Supported | Path-style root listing through `GET /s3` |
| `CreateBucket` | Supported | Path-style bucket create through `PUT /s3/{bucket}` |
| `DeleteBucket` | Supported | Empty buckets only; non-empty buckets return conflict |
| `GetBucketPolicy` | Supported | `GET /s3/{bucket}?policy` |
| `PutBucketPolicy` | Partial | Practical AWS-style bucket policy support; not full IAM parity |
| `DeleteBucketPolicy` | Supported | `DELETE /s3/{bucket}?policy` |
| `ListObjectsV2` | Supported | Includes `max-keys`, continuation token, `StartAfter`, and `Prefix` |
| `ListObjectVersions` | Partial | Practical version listing through `GET /s3/{bucket}?versions`; not full XML parity |
| `GetBucketVersioning` | Not supported | No separate bucket versioning configuration API yet |
| `PutBucketVersioning` | Not supported | Versioning behavior is implemented in-platform, but bucket versioning API parity is not complete |
| `GetBucketTagging` | Not supported | Bucket tagging is not implemented |
| `PutBucketTagging` | Not supported | Bucket tagging is not implemented |
| `DeleteBucketTagging` | Not supported | Bucket tagging is not implemented |

## Object APIs

| API | Status | Notes |
| --- | --- | --- |
| `PutObject` | Supported | SigV4 header-signed and presigned `PUT`; metadata headers supported |
| `GetObject` | Supported | Supports policy-gated and presigned access |
| `HeadObject` | Supported | Returns `ETag`, `Last-Modified`, and persisted metadata headers |
| `DeleteObject` | Supported | Creates delete markers on versioned objects; retention and legal-hold checks apply |
| `DeleteObjectVersion` | Partial | Practical version-aware delete through `versionId`; not full Amazon parity on all edge cases |
| `CopyObject` | Supported | Returns `CopyObjectResult` XML and version headers |
| `GetObjectTagging` | Supported | `GET ?tagging` |
| `PutObjectTagging` | Supported | `PUT ?tagging` |
| `DeleteObjectTagging` | Not supported | Explicit delete-tagging API is not implemented |
| `RestoreObject` | Not supported | Glacier-style restore is not part of the current scope |
| `SelectObjectContent` | Not supported | Not part of the current scope |

## Multipart APIs

| API | Status | Notes |
| --- | --- | --- |
| `CreateMultipartUpload` | Supported | Returns `UploadId` |
| `UploadPart` | Supported | Content-MD5 validation is enforced |
| `CompleteMultipartUpload` | Supported | Validates part order; returns final object metadata |
| `AbortMultipartUpload` | Supported | Returns success on abort and worker cleans up expired sessions |
| `ListMultipartUploads` | Not supported | No public multipart listing API yet |
| `ListParts` | Not supported | No public part-listing API yet |

## Presigned URLs

| API | Status | Notes |
| --- | --- | --- |
| Presigned `PUT` | Supported | SigV4 query auth |
| Presigned `GET` | Supported | SigV4 query auth |
| Presigned `DELETE` | Not supported | Not part of the current release contract |
| Presigned multipart flows | Not supported | Not part of the current release contract |

## Policy And Access Control

| Area | Status | Notes |
| --- | --- | --- |
| SigV4 header auth | Supported | Current default for authenticated S3 calls |
| Bucket policy evaluation | Partial | Supports common `Principal`, `NotPrincipal`, `Allow`, `Deny`, and practical `Condition` operators |
| Signed object access decisions by bucket policy | Partial | Practical signed-access policy enforcement exists, including explicit deny behavior |
| Condition-based anonymous access | Partial | Release regression covers prefix-constrained listing and public-object allow rules |
| Full AWS IAM compatibility | Not supported | HarborShield is practical, not parity-complete |
| STS temporary credentials | Not supported | Not implemented |

## Versioning And Governance

| Area | Status | Notes |
| --- | --- | --- |
| Object version creation | Supported | Overwrites create new versions |
| Delete markers | Supported | Current object can be hidden by delete marker |
| Version listing | Partial | Practical version-listing support exists |
| Version-specific delete | Partial | Supported for cleanup workflows; not full AWS parity on every edge case |
| Retention checks | Partial | Enforced in-platform; not full object-lock API parity |
| Legal hold checks | Partial | Enforced in-platform; not full S3 object-lock API surface |

## Known Compatibility Boundaries

- HarborShield currently uses path-style routing. Virtual-host-style S3 addressing is not part of the release contract.
- Bucket policy support is intentionally practical rather than parity-complete. Amazon-style `Condition` and `Principal` coverage is incomplete.
- Multipart failure semantics are tested for `BadDigest`, `InvalidPart`, and `InvalidPartOrder`, but public multipart listing APIs remain out of scope.
- Version cleanup semantics are tested for delete markers, version-specific delete, and `NoSuchVersion`, but full Amazon versioning parity is still not claimed.
- Bucket policy regression covers policy round-trip, `NotPrincipal` deny behavior for signed access, and prefix-based anonymous access rules, but full AWS condition-key coverage and IAM interaction breadth are still incomplete.
- Unsupported APIs should be treated as not promised for `v1.0`.
- Distributed storage does not change the object-plane contract; it changes the blob placement backend only.

## Release Positioning

For broad release quality, HarborShield should claim:

- a tested, practical S3-compatible surface for the APIs listed as `Supported`
- partial support with documented caveats for APIs listed as `Partial`
- no compatibility claim for APIs listed as `Not supported`
