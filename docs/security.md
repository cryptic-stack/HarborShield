# Security Model

- deny-by-default authorization
- bcrypt password hashing
- SigV4 request verification for supported S3 operations
- SigV4-style presigned `GET` and `PUT` with expiration enforcement
- constant-time secret comparisons
- structured audit records for login, bucket, and object actions
- private buckets by default
- request size limits at the API layer
- separate admin tokens and S3 credentials
- encrypted object blobs at rest via `STORAGE_MASTER_KEY`
- OIDC authorization-code login with configurable claim-to-role mapping
- future hooks for admin IP allowlists, SSE, KMS, and MFA

Current gaps:

- PostgreSQL and Redis encryption depend on deployment choices outside the app
- MFA and KMS-backed encryption are not implemented yet
- OIDC federation does not yet include richer group sync, temporary S3 session credentials, or provider-specific policy sync
- OIDC role mappings must target built-in HarborShield roles only: `superadmin`, `admin`, `auditor`, `bucket-admin`, or `readonly`
