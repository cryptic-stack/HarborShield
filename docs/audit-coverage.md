# Audit Coverage

This guide describes the audit trail HarborShield is expected to produce for broad-release operations.

## Goals

The audit log should answer four operator questions quickly:

- Who changed the platform?
- What object, bucket, user, policy, or setting was affected?
- Whether the action succeeded or failed
- What changed, without exposing secrets

## Covered Action Areas

The current platform records audit entries for the following high-value actions.

### Authentication

- `auth.login`
- `auth.refresh`
- `auth.logout`
- `auth.change-password`
- `auth.logout-all`

### First-Run And Settings

- `setup.complete`
- `settings.oidc.update`
- `settings.oidc.clear-secret`
- `settings.oidc.test`
- `settings.storage-policy.update`

### Buckets And Objects

- `bucket.create`
- `bucket.policy.put`
- `bucket.policy.delete`
- `bucket.durability.update`
- `object.put`
- `object.copy`
- `object.delete`
- `object.version.delete`
- `object.restore`
- `object.tagging.put`
- `object.legal-hold.put`

### Credentials, Users, And Roles

- `credential.create`
- `credential.role.update`
- `user.role.update`
- `admin-token.create`
- `role.statement.create`
- `role.statement.update`
- `role.statement.delete`
- `role-binding.create`
- `role-binding.delete`

### Quotas

- `quota.bucket.update`
- `quota.user.update`

### Events And Malware

- `event-target.create`
- `malware.scan.completed`
- webhook delivery outcomes are recorded through the events service

### Storage Topology

- `storage.join-token.create`
- `storage.node.register`
- `storage.node.update`
- `storage.node.tls.repin`

## Masking And Safe Detail

Audit detail is sanitized before storage and API output.

- Secrets, tokens, passwords, hashes, and private keys are masked
- OIDC client secret state is represented as a boolean flag
- Event target secret presence is represented as `secretStored`
- Settings changes use `before`, `after`, and `changedFields` where that helps operators understand impact

## Expected Review Points

For release quality, operators should verify these flows during acceptance testing:

1. First login, password change, and setup completion create visible audit entries.
2. OIDC settings edits show masked diffs instead of raw secrets.
3. Bucket, object, and credential actions appear with actor, request ID, and resource.
4. Quota changes record before and after state.
5. Role and binding changes record enough detail to reconstruct who granted what.
6. Storage-node lifecycle changes are visible before and after maintenance operations.

## Known Boundaries

- Read-only actions are intentionally not exhaustively audited.
- Internal recurring worker sweeps are primarily visible through metrics, logs, and event-delivery records rather than verbose audit spam.
- Failed authorization attempts are surfaced through logs and metrics more heavily than through full audit expansion today.

## Related Guides

- [operations.md](./operations.md)
- [troubleshooting.md](./troubleshooting.md)
- [metrics-alerting.md](./metrics-alerting.md)
