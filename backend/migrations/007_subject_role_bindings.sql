CREATE TABLE IF NOT EXISTS subject_role_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_type TEXT NOT NULL,
    subject_id TEXT NOT NULL,
    role_name TEXT NOT NULL REFERENCES roles(name) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (subject_type, subject_id)
);
