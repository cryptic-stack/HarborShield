INSERT INTO role_policy_statements (role_name, action, resource, effect)
VALUES
    ('superadmin', 'event.*', '*', 'allow'),
    ('admin', 'event.read', '*', 'allow'),
    ('admin', 'event.manage', '*', 'allow'),
    ('auditor', 'event.read', '*', 'allow')
ON CONFLICT DO NOTHING;
