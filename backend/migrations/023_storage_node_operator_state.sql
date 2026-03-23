ALTER TABLE storage_nodes
    ADD COLUMN IF NOT EXISTS operator_state TEXT NOT NULL DEFAULT 'active';

CREATE INDEX IF NOT EXISTS storage_nodes_operator_state_idx
    ON storage_nodes (operator_state, status);

INSERT INTO role_policy_statements (role_name, action, resource, effect) VALUES
    ('superadmin', 'storage.manage', '*', 'allow'),
    ('admin', 'storage.manage', '*', 'allow')
ON CONFLICT DO NOTHING;
