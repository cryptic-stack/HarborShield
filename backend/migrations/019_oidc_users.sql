ALTER TABLE users
    ADD COLUMN IF NOT EXISTS auth_provider TEXT NOT NULL DEFAULT 'local',
    ADD COLUMN IF NOT EXISTS external_subject TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS users_auth_provider_external_subject_unique
    ON users (auth_provider, external_subject)
    WHERE external_subject <> '';
