package dashboard

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"harborshield/backend/internal/settings"
)

type Service struct {
	db           *pgxpool.Pool
	replicaCount int
	settings     *settings.Service
}

type Summary struct {
	BucketCount         int64  `json:"bucketCount"`
	LiveObjectCount     int64  `json:"liveObjectCount"`
	TotalStoredBytes    int64  `json:"totalStoredBytes"`
	PendingScanCount    int64  `json:"pendingScanCount"`
	DeadLetterCount     int64  `json:"deadLetterCount"`
	StorageNodeCount    int64  `json:"storageNodeCount"`
	ReplicaTarget       int64  `json:"replicaTarget"`
	OfflineStorageNodes int64  `json:"offlineStorageNodes"`
	DegradedPlacements  int64  `json:"degradedPlacements"`
	RebalanceGapCount   int64  `json:"rebalanceGapCount"`
	RecentAuditCount24h int64  `json:"recentAuditCount24h"`
	LatestAuditAt       string `json:"latestAuditAt"`
}

func New(db *pgxpool.Pool, replicaCount int, settingsSvc *settings.Service) *Service {
	return &Service{db: db, replicaCount: replicaCount, settings: settingsSvc}
}

func (s *Service) Summary(ctx context.Context) (Summary, error) {
	var summary Summary
	replicaCount := s.replicaCount
	if s.settings != nil {
		if policy, err := s.settings.ResolveStoragePolicy(ctx); err == nil {
			for _, item := range policy.Policies {
				if item.Name == policy.DefaultClass && item.DefaultReplicas > 0 {
					replicaCount = item.DefaultReplicas
					break
				}
			}
		}
	}
	replicaTargetExpr := `GREATEST(LEAST($1::bigint, (SELECT COUNT(*) FROM storage_nodes WHERE operator_state = 'active')), 0)`
	err := s.db.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM buckets),
			(SELECT COUNT(*) FROM objects WHERE deleted_at IS NULL AND is_delete_marker = FALSE AND is_latest = TRUE),
			(SELECT COALESCE(SUM(size_bytes), 0) FROM objects WHERE deleted_at IS NULL AND is_delete_marker = FALSE AND is_latest = TRUE),
			(SELECT COUNT(*) FROM objects WHERE deleted_at IS NULL AND scan_status = 'pending-scan'),
			(SELECT COUNT(*) FROM event_deliveries WHERE status = 'dead_letter'),
			(SELECT COUNT(*) FROM storage_nodes),
			`+replicaTargetExpr+`,
			(SELECT COUNT(*) FROM storage_nodes WHERE status <> 'healthy'),
			(SELECT COUNT(*) FROM object_placements WHERE state = 'degraded'),
			(SELECT COUNT(*) FROM (
				SELECT p.object_id
				FROM object_placements p
				GROUP BY p.object_id
				HAVING COUNT(*) < `+replicaTargetExpr+`
			) AS rebalance_gap_objects),
			(SELECT COUNT(*) FROM audit_logs WHERE created_at >= NOW() - INTERVAL '24 hours'),
			COALESCE((SELECT MAX(created_at)::text FROM audit_logs), '')
	`, replicaCount).Scan(
		&summary.BucketCount,
		&summary.LiveObjectCount,
		&summary.TotalStoredBytes,
		&summary.PendingScanCount,
		&summary.DeadLetterCount,
		&summary.StorageNodeCount,
		&summary.ReplicaTarget,
		&summary.OfflineStorageNodes,
		&summary.DegradedPlacements,
		&summary.RebalanceGapCount,
		&summary.RecentAuditCount24h,
		&summary.LatestAuditAt,
	)
	return summary, err
}
