INSERT INTO role_policy_statements (role_name, action, resource, effect) VALUES
    ('superadmin', 'settings.read', '*', 'allow'),
    ('admin', 'settings.read', '*', 'allow'),
    ('auditor', 'settings.read', '*', 'allow')
ON CONFLICT DO NOTHING;
