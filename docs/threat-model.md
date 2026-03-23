# Threat Model Summary

Primary concerns:

- stolen admin credentials
- overbroad S3 service account permissions
- path traversal and object locator abuse
- audit log tampering
- malicious uploads
- replay of signed URLs
- bucket deletion mistakes

Initial mitigations:

- bootstrap password rotation flow
- secret hashing at rest
- non-user-derived object storage paths
- structured immutable-style audit records in PostgreSQL
- short presigned URL expiry windows
- safer destructive action prompts in UI
