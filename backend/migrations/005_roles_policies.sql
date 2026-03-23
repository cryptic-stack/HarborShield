CREATE TABLE IF NOT EXISTS roles (
    name TEXT PRIMARY KEY,
    description TEXT NOT NULL DEFAULT '',
    builtin BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS role_policy_statements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    role_name TEXT NOT NULL REFERENCES roles(name) ON DELETE CASCADE,
    action TEXT NOT NULL,
    resource TEXT NOT NULL,
    effect TEXT NOT NULL,
    conditions JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS role_policy_statements_unique_stmt
    ON role_policy_statements (role_name, action, resource, effect);

INSERT INTO roles (name, description, builtin) VALUES
    ('superadmin', 'Full platform control', TRUE),
    ('admin', 'Administrative access to buckets, objects, users, and credentials', TRUE),
    ('auditor', 'Read-only access to audit and health surfaces', TRUE),
    ('bucket-admin', 'Bucket and object administration without user management', TRUE),
    ('readonly', 'Read-only bucket and object access', TRUE)
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_policy_statements (role_name, action, resource, effect) VALUES
    ('superadmin', '*', '*', 'allow'),
    ('admin', 'bucket.*', '*', 'allow'),
    ('admin', 'object.*', '*', 'allow'),
    ('admin', 'credential.create', '*', 'allow'),
    ('admin', 'user.manage', '*', 'allow'),
    ('admin', 'audit.read', '*', 'allow'),
    ('admin', 'health.read', '*', 'allow'),
    ('admin', 'role.read', '*', 'allow'),
    ('auditor', 'audit.read', '*', 'allow'),
    ('auditor', 'health.read', '*', 'allow'),
    ('auditor', 'bucket.list', '*', 'allow'),
    ('auditor', 'object.list', '*', 'allow'),
    ('auditor', 'object.get', '*', 'allow'),
    ('auditor', 'role.read', '*', 'allow'),
    ('bucket-admin', 'bucket.create', '*', 'allow'),
    ('bucket-admin', 'bucket.list', '*', 'allow'),
    ('bucket-admin', 'bucket.delete', 'bucket:*', 'allow'),
    ('bucket-admin', 'object.*', '*', 'allow'),
    ('bucket-admin', 'health.read', '*', 'allow'),
    ('readonly', 'bucket.list', '*', 'allow'),
    ('readonly', 'object.list', '*', 'allow'),
    ('readonly', 'object.get', '*', 'allow'),
    ('readonly', 'health.read', '*', 'allow')
ON CONFLICT DO NOTHING;
