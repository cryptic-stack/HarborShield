# API Overview

## Admin API

Base path: `/api/v1`

Implemented domains:

- `/auth/login`
- `/auth/refresh`
- `/auth/oidc`
- `/auth/oidc/start`
- `/auth/oidc/callback`
- `/auth/me`
- `/auth/change-password`
- `/buckets`
- `/buckets/{bucketID}/objects`
- `/buckets/{bucketID}/objects/versions`
- `/buckets/{bucketID}/objects/restore`
- `/buckets/{bucketID}/objects/tags`
- `/buckets/{bucketID}/objects/copy`
- `/buckets/{bucketID}/objects/upload`
- `/buckets/{bucketID}/objects/download`
- `/users`
- `/credentials`
- `/dashboard`
- `/audit`
- `/event-targets`
- `/event-deliveries`
- `/malware-status`
- `/settings`
- `/storage/nodes`
- `/storage/placements`
- `/quotas`
- `/roles`
- `/roles/{roleName}/statements`
- `/role-bindings`
- `/policy-evaluate`
- `/admin-tokens`
- `/health`

Checked-in contract:

- [`docs/openapi-admin.yaml`](c:\Users\JBrown\Documents\Project\s3-platform\docs\openapi-admin.yaml)

The OpenAPI file is now the canonical route inventory for the current admin API.
Some response schemas remain intentionally flexible where the payload is still
evolving, but the request shapes and implemented paths should match the live
service.

## Object API

Base path: `/s3`

Release compatibility contract:

- [`docs/s3-compatibility.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\s3-compatibility.md)

Current path-style operations:

- `GET /s3`
- `PUT /s3/{bucket}`
- `DELETE /s3/{bucket}`
- `GET /s3/{bucket}?list-type=2`
- `PUT /s3/{bucket}/{key}`
- `GET /s3/{bucket}/{key}`
- `HEAD /s3/{bucket}/{key}`
- `DELETE /s3/{bucket}/{key}`

Additional implemented object features:

- multipart initiate, upload part, complete, abort
- presigned `GET` and `PUT`
- `CopyObject`
- object tagging `GET` and `PUT`
- bucket policy `GET`, `PUT`, and `DELETE` through `?policy`
- version-aware object retrieval for the admin plane

Current AWS-style bucket policy support:

- policy version `2012-10-17`
- `Principal: "*"` and `Principal: { "AWS": ... }`
- `NotPrincipal`
- single or array `Action`
- single or array `Resource`
- practical `Condition` support for:
  - `StringEquals`
  - `StringLike`
  - `StringNotEquals`
  - `StringNotLike`
  - `IpAddress`
  - `NotIpAddress`
- explicit `Deny` overrides `Allow`
- currently supported S3 actions:
  - `s3:ListBucket`
  - `s3:GetObject`
  - `s3:PutObject`
  - `s3:DeleteObject`
  - `s3:GetObjectTagging`
  - `s3:PutObjectTagging`
  - `s3:AbortMultipartUpload`

Not yet complete:

- full AWS IAM compatibility
- full AWS bucket policy condition/principal parity
- OIDC or STS-style federated identity on the object plane

Storage topology endpoints are currently read-only scaffolding for the future optional distributed backend. On the default local backend they will typically return empty lists.
