ALTER TABLE subject_role_bindings
    ADD COLUMN IF NOT EXISTS resource TEXT NOT NULL DEFAULT '*';

ALTER TABLE subject_role_bindings
    DROP CONSTRAINT IF EXISTS subject_role_bindings_subject_type_subject_id_key;

CREATE UNIQUE INDEX IF NOT EXISTS subject_role_bindings_unique_scope
    ON subject_role_bindings (subject_type, subject_id, resource);
