INSERT INTO role_policy_statements (role_name, action, resource, effect) VALUES
    ('superadmin', 'quota.*', '*', 'allow'),
    ('admin', 'quota.read', '*', 'allow'),
    ('admin', 'quota.manage', '*', 'allow'),
    ('auditor', 'quota.read', '*', 'allow')
ON CONFLICT DO NOTHING;
