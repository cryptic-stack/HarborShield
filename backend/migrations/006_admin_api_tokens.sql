CREATE TABLE IF NOT EXISTS admin_api_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    role TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    last_used_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO role_policy_statements (role_name, action, resource, effect) VALUES
    ('superadmin', 'admin-token.*', '*', 'allow'),
    ('superadmin', 'role.manage', '*', 'allow'),
    ('admin', 'admin-token.create', '*', 'allow'),
    ('admin', 'admin-token.read', '*', 'allow')
ON CONFLICT DO NOTHING;
