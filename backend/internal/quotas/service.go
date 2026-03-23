package quotas

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	db *pgxpool.Pool
}

type BucketQuota struct {
	BucketID                string `json:"bucketId"`
	BucketName              string `json:"bucketName"`
	CurrentBytes            int64  `json:"currentBytes"`
	CurrentObjects          int64  `json:"currentObjects"`
	MaxBytes                *int64 `json:"maxBytes"`
	MaxObjects              *int64 `json:"maxObjects"`
	WarningThresholdPercent *int32 `json:"warningThresholdPercent"`
	RecalculatedAt          string `json:"recalculatedAt"`
}

type UserQuota struct {
	UserID                  string `json:"userId"`
	Email                   string `json:"email"`
	CurrentBytes            int64  `json:"currentBytes"`
	MaxBytes                *int64 `json:"maxBytes"`
	WarningThresholdPercent *int32 `json:"warningThresholdPercent"`
	RecalculatedAt          string `json:"recalculatedAt"`
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) ListBucketQuotas(ctx context.Context) ([]BucketQuota, error) {
	rows, err := s.db.Query(ctx, `
		SELECT
			b.id::text,
			b.name,
			COALESCE(q.current_bytes, 0),
			COALESCE(q.current_objects, 0),
			q.max_bytes,
			q.max_objects,
			q.warning_threshold_percent,
			COALESCE(q.recalculated_at::text, '')
		FROM buckets b
		LEFT JOIN bucket_quotas q ON q.bucket_id = b.id
		ORDER BY b.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []BucketQuota
	for rows.Next() {
		var item BucketQuota
		if err := rows.Scan(
			&item.BucketID,
			&item.BucketName,
			&item.CurrentBytes,
			&item.CurrentObjects,
			&item.MaxBytes,
			&item.MaxObjects,
			&item.WarningThresholdPercent,
			&item.RecalculatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) UpdateBucketQuota(ctx context.Context, bucketID string, maxBytes, maxObjects *int64, warningThresholdPercent *int32) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO bucket_quotas (bucket_id, max_bytes, max_objects, warning_threshold_percent)
		VALUES ($1::uuid, $2, $3, $4)
		ON CONFLICT (bucket_id)
		DO UPDATE SET
			max_bytes = EXCLUDED.max_bytes,
			max_objects = EXCLUDED.max_objects,
			warning_threshold_percent = EXCLUDED.warning_threshold_percent
	`, bucketID, maxBytes, maxObjects, warningThresholdPercent)
	return err
}

func (s *Service) ListUserQuotas(ctx context.Context) ([]UserQuota, error) {
	rows, err := s.db.Query(ctx, `
		SELECT
			u.id::text,
			u.email,
			COALESCE(q.current_bytes, 0),
			q.max_bytes,
			q.warning_threshold_percent,
			COALESCE(q.recalculated_at::text, '')
		FROM users u
		LEFT JOIN user_quotas q ON q.user_id = u.id
		ORDER BY u.email
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []UserQuota
	for rows.Next() {
		var item UserQuota
		if err := rows.Scan(
			&item.UserID,
			&item.Email,
			&item.CurrentBytes,
			&item.MaxBytes,
			&item.WarningThresholdPercent,
			&item.RecalculatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) UpdateUserQuota(ctx context.Context, userID string, maxBytes *int64, warningThresholdPercent *int32) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO user_quotas (user_id, max_bytes, warning_threshold_percent)
		VALUES ($1::uuid, $2, $3)
		ON CONFLICT (user_id)
		DO UPDATE SET
			max_bytes = EXCLUDED.max_bytes,
			warning_threshold_percent = EXCLUDED.warning_threshold_percent
	`, userID, maxBytes, warningThresholdPercent)
	return err
}

func (s *Service) CheckObjectWrite(ctx context.Context, bucketID, userID string, existingSize, nextSize int64, exists bool) error {
	bucketSnapshot, bucketWarningThreshold, err := s.bucketSnapshot(ctx, bucketID)
	if err != nil {
		return err
	}

	nextBucketBytes := nextSize - existingSize
	nextBucketObjects := int64(0)
	if !exists {
		nextBucketObjects = 1
	}
	if !Allows(nextBucketBytes, nextBucketObjects, bucketSnapshot) {
		return ErrQuotaExceeded
	}

	if userID == "" {
		return nil
	}

	userSnapshot, _, err := s.userSnapshot(ctx, userID)
	if err != nil {
		return err
	}
	if !Allows(nextSize-existingSize, 0, userSnapshot) {
		return ErrQuotaExceeded
	}

	_ = bucketWarningThreshold
	return nil
}

func (s *Service) CurrentWarnings(ctx context.Context, bucketID, userID string) (WarningState, error) {
	state := WarningState{}

	bucketSnapshot, bucketWarningThreshold, err := s.bucketSnapshot(ctx, bucketID)
	if err != nil {
		return state, err
	}
	state.BucketBytes = warningReached(bucketSnapshot.CurrentBytes, bucketSnapshot.MaxBytes, bucketWarningThreshold)
	state.BucketCount = warningReached(bucketSnapshot.CurrentObjects, bucketSnapshot.MaxObjects, bucketWarningThreshold)

	if userID != "" {
		userSnapshot, userWarningThreshold, err := s.userSnapshot(ctx, userID)
		if err != nil {
			return state, err
		}
		state.UserBytes = warningReached(userSnapshot.CurrentBytes, userSnapshot.MaxBytes, userWarningThreshold)
	}

	return state, nil
}

func (s *Service) RecalculateBucketUsage(ctx context.Context) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		INSERT INTO bucket_quotas (bucket_id, current_bytes, current_objects, recalculated_at)
		SELECT
			b.id,
			COALESCE(SUM(o.size_bytes) FILTER (WHERE o.deleted_at IS NULL), 0),
			COALESCE(COUNT(o.id) FILTER (WHERE o.deleted_at IS NULL), 0),
			NOW()
		FROM buckets b
		LEFT JOIN objects o ON o.bucket_id = b.id
		GROUP BY b.id
		ON CONFLICT (bucket_id)
		DO UPDATE SET
			current_bytes = EXCLUDED.current_bytes,
			current_objects = EXCLUDED.current_objects,
			recalculated_at = EXCLUDED.recalculated_at
	`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Service) RecalculateUserUsage(ctx context.Context) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		INSERT INTO user_quotas (user_id, current_bytes, recalculated_at)
		SELECT
			u.id,
			COALESCE(SUM(o.size_bytes) FILTER (WHERE o.deleted_at IS NULL), 0),
			NOW()
		FROM users u
		LEFT JOIN objects o ON o.created_by = u.id
		GROUP BY u.id
		ON CONFLICT (user_id)
		DO UPDATE SET
			current_bytes = EXCLUDED.current_bytes,
			recalculated_at = EXCLUDED.recalculated_at
	`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Service) bucketSnapshot(ctx context.Context, bucketID string) (Snapshot, *int32, error) {
	var snapshot Snapshot
	var warningThreshold *int32
	err := s.db.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(o.size_bytes) FILTER (WHERE o.deleted_at IS NULL), 0),
			COALESCE(COUNT(o.id) FILTER (WHERE o.deleted_at IS NULL), 0),
			q.max_bytes,
			q.max_objects,
			q.warning_threshold_percent
		FROM buckets b
		LEFT JOIN bucket_quotas q ON q.bucket_id = b.id
		LEFT JOIN objects o ON o.bucket_id = b.id
		WHERE b.id = $1::uuid
		GROUP BY q.max_bytes, q.max_objects, q.warning_threshold_percent
	`, bucketID).Scan(&snapshot.CurrentBytes, &snapshot.CurrentObjects, &snapshot.MaxBytes, &snapshot.MaxObjects, &warningThreshold)
	if err != nil {
		return Snapshot{}, nil, err
	}
	return snapshot, warningThreshold, nil
}

func (s *Service) userSnapshot(ctx context.Context, userID string) (Snapshot, *int32, error) {
	var snapshot Snapshot
	var warningThreshold *int32
	err := s.db.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(o.size_bytes) FILTER (WHERE o.deleted_at IS NULL), 0),
			0,
			q.max_bytes,
			NULL,
			q.warning_threshold_percent
		FROM users u
		LEFT JOIN user_quotas q ON q.user_id = u.id
		LEFT JOIN objects o ON o.created_by = u.id
		WHERE u.id = $1::uuid
		GROUP BY q.max_bytes, q.warning_threshold_percent
	`, userID).Scan(&snapshot.CurrentBytes, &snapshot.CurrentObjects, &snapshot.MaxBytes, &snapshot.MaxObjects, &warningThreshold)
	if err != nil {
		return Snapshot{}, nil, err
	}
	return snapshot, warningThreshold, nil
}

func warningReached(current int64, max *int64, threshold *int32) bool {
	if max == nil || threshold == nil || *max <= 0 {
		return false
	}
	return current*100 >= *max*int64(*threshold)
}

func IsQuotaExceeded(err error) bool {
	return errors.Is(err, ErrQuotaExceeded)
}
