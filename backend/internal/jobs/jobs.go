package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNoDueJobs = errors.New("no due jobs")

type Job struct {
	ID        string
	JobType   string
	Payload   map[string]any
	Attempts  int
	NextRunAt time.Time
}

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) ResetStaleRunning(ctx context.Context, staleAfter time.Duration) error {
	_, err := s.db.Exec(ctx, `
		UPDATE jobs
		SET status = 'pending', updated_at = NOW(), last_error = 'recovered_after_worker_restart'
		WHERE status = 'running' AND updated_at <= NOW() - $1::interval
	`, staleAfter.String())
	return err
}

func (s *Service) EnsureRecurring(ctx context.Context, jobType string, interval time.Duration, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO jobs (job_type, payload, status, next_run_at)
		VALUES ($1, $2::jsonb, 'pending', NOW() + $3::interval)
		ON CONFLICT (job_type)
		DO UPDATE SET payload = EXCLUDED.payload
	`, jobType, body, interval.String())
	return err
}

func (s *Service) ClaimDue(ctx context.Context) (Job, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var (
		job       Job
		payload   []byte
		nextRunAt time.Time
	)
	err = tx.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id
			FROM jobs
			WHERE status = 'pending' AND next_run_at <= NOW()
			ORDER BY next_run_at, created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE jobs
		SET status = 'running', attempts = attempts + 1, updated_at = NOW(), last_error = ''
		WHERE id IN (SELECT id FROM candidate)
		RETURNING id::text, job_type, payload, attempts, next_run_at
	`).Scan(&job.ID, &job.JobType, &payload, &job.Attempts, &nextRunAt)
	if err != nil {
		return Job{}, normalizeNoRows(err)
	}
	if err := json.Unmarshal(payload, &job.Payload); err != nil {
		return Job{}, err
	}
	job.NextRunAt = nextRunAt
	if err := tx.Commit(ctx); err != nil {
		return Job{}, err
	}
	return job, nil
}

func (s *Service) Complete(ctx context.Context, jobID string, nextRunAt time.Time) error {
	_, err := s.db.Exec(ctx, `
		UPDATE jobs
		SET status = 'pending', next_run_at = $2, updated_at = NOW(), last_error = ''
		WHERE id = $1::uuid
	`, jobID, nextRunAt.UTC())
	return err
}

func (s *Service) Fail(ctx context.Context, jobID string, nextRunAt time.Time, errMessage string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE jobs
		SET status = 'pending', next_run_at = $2, updated_at = NOW(), last_error = $3
		WHERE id = $1::uuid
	`, jobID, nextRunAt.UTC(), errMessage)
	return err
}

func normalizeNoRows(err error) error {
	if err == nil {
		return nil
	}
	if err.Error() == "no rows in result set" {
		return ErrNoDueJobs
	}
	return err
}
