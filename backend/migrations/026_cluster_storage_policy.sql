CREATE TABLE IF NOT EXISTS cluster_storage_policies (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    default_storage_class TEXT NOT NULL,
    standard_replicas INTEGER NOT NULL,
    reduced_redundancy_replicas INTEGER NOT NULL,
    archive_ready_replicas INTEGER NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO role_policy_statements (role_name, action, resource, effect) VALUES
    ('superadmin', 'settings.manage', '*', 'allow'),
    ('admin', 'settings.manage', '*', 'allow')
ON CONFLICT DO NOTHING;
