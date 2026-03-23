ALTER TABLE jobs
    ADD COLUMN IF NOT EXISTS last_error TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS jobs_job_type_unique ON jobs (job_type);
