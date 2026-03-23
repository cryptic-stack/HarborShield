package buckets

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"harborshield/backend/internal/config"
	"harborshield/backend/internal/settings"
)

var ErrBucketNotEmpty = errors.New("bucket is not empty")
var ErrBucketNotFound = errors.New("bucket not found")

type Bucket struct {
	ID                     string `json:"id"`
	Name                   string `json:"name"`
	Tenant                 string `json:"tenant"`
	StorageClass           string `json:"storageClass"`
	ReplicaTarget          int    `json:"replicaTarget"`
	EffectiveStorageClass  string `json:"effectiveStorageClass"`
	EffectiveReplicaTarget int    `json:"effectiveReplicaTarget"`
	CreatedAt              string `json:"createdAt"`
}

type Service struct {
	db                  *pgxpool.Pool
	defaultStorageClass string
	clusterReplicas     int
	settings            *settings.Service
}

func New(db *pgxpool.Pool, defaultStorageClass string, clusterReplicas int, settingsSvc *settings.Service) *Service {
	return &Service{
		db:                  db,
		defaultStorageClass: normalizeStorageClass(defaultStorageClass),
		clusterReplicas:     clusterReplicas,
		settings:            settingsSvc,
	}
}

func (s *Service) List(ctx context.Context) ([]Bucket, error) {
	rows, err := s.db.Query(ctx, `SELECT id::text, name, tenant, storage_class, COALESCE(replica_target, 0), created_at::text FROM buckets ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Bucket, 0)
	for rows.Next() {
		var bucket Bucket
		if err := rows.Scan(&bucket.ID, &bucket.Name, &bucket.Tenant, &bucket.StorageClass, &bucket.ReplicaTarget, &bucket.CreatedAt); err != nil {
			return nil, err
		}
		s.applyEffectiveDurability(ctx, &bucket)
		out = append(out, bucket)
	}
	return out, rows.Err()
}

func (s *Service) Create(ctx context.Context, name, tenant, createdBy, storageClass string, replicaTarget int) (Bucket, error) {
	storageClass = normalizeStorageClass(storageClass)
	if storageClass == "" {
		storageClass = "inherit"
	}
	if storageClass != "inherit" && !config.IsValidStorageClass(storageClass) {
		return Bucket{}, fmt.Errorf("storage class must be inherit, standard, reduced-redundancy, or archive-ready")
	}
	if replicaTarget < 0 {
		return Bucket{}, fmt.Errorf("replica target must be zero or greater")
	}
	var bucket Bucket
	err := s.db.QueryRow(ctx, `
		INSERT INTO buckets (name, tenant, storage_class, replica_target, created_by)
		VALUES ($1, $2, $3, NULLIF($4, 0), NULLIF($5, '')::uuid)
		RETURNING id::text, name, tenant, storage_class, COALESCE(replica_target, 0), created_at::text
	`, name, tenant, storageClass, replicaTarget, createdBy).Scan(&bucket.ID, &bucket.Name, &bucket.Tenant, &bucket.StorageClass, &bucket.ReplicaTarget, &bucket.CreatedAt)
	if err != nil {
		return Bucket{}, fmt.Errorf("create bucket: %w", err)
	}
	s.applyEffectiveDurability(ctx, &bucket)
	return bucket, nil
}

func (s *Service) Delete(ctx context.Context, name string) error {
	var bucketID string
	if err := s.db.QueryRow(ctx, `SELECT id::text FROM buckets WHERE name = $1`, name).Scan(&bucketID); err != nil {
		return ErrBucketNotFound
	}
	var objectCount int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(1) FROM objects WHERE bucket_id = $1 AND deleted_at IS NULL`, bucketID).Scan(&objectCount); err != nil {
		return err
	}
	if objectCount > 0 {
		return ErrBucketNotEmpty
	}
	commandTag, err := s.db.Exec(ctx, `DELETE FROM buckets WHERE id = $1::uuid`, bucketID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return ErrBucketNotFound
	}
	return nil
}

func (s *Service) GetByName(ctx context.Context, name string) (Bucket, error) {
	var bucket Bucket
	err := s.db.QueryRow(ctx, `
		SELECT id::text, name, tenant, storage_class, COALESCE(replica_target, 0), created_at::text
		FROM buckets WHERE name = $1
	`, name).Scan(&bucket.ID, &bucket.Name, &bucket.Tenant, &bucket.StorageClass, &bucket.ReplicaTarget, &bucket.CreatedAt)
	if err == nil {
		s.applyEffectiveDurability(ctx, &bucket)
	}
	return bucket, err
}

func (s *Service) GetByID(ctx context.Context, bucketID string) (Bucket, error) {
	var bucket Bucket
	err := s.db.QueryRow(ctx, `
		SELECT id::text, name, tenant, storage_class, COALESCE(replica_target, 0), created_at::text
		FROM buckets WHERE id = $1::uuid
	`, bucketID).Scan(&bucket.ID, &bucket.Name, &bucket.Tenant, &bucket.StorageClass, &bucket.ReplicaTarget, &bucket.CreatedAt)
	if err != nil {
		return Bucket{}, ErrBucketNotFound
	}
	s.applyEffectiveDurability(ctx, &bucket)
	return bucket, nil
}

func (s *Service) UpdateDurability(ctx context.Context, bucketID, storageClass string, replicaTarget int) (Bucket, error) {
	storageClass = normalizeStorageClass(storageClass)
	if storageClass == "" {
		storageClass = "inherit"
	}
	if storageClass != "inherit" && !config.IsValidStorageClass(storageClass) {
		return Bucket{}, fmt.Errorf("storage class must be inherit, standard, reduced-redundancy, or archive-ready")
	}
	if replicaTarget < 0 {
		return Bucket{}, fmt.Errorf("replica target must be zero or greater")
	}
	var bucket Bucket
	err := s.db.QueryRow(ctx, `
		UPDATE buckets
		SET storage_class = $2,
		    replica_target = NULLIF($3, 0),
		    created_at = created_at
		WHERE id = $1::uuid
		RETURNING id::text, name, tenant, storage_class, COALESCE(replica_target, 0), created_at::text
	`, bucketID, storageClass, replicaTarget).Scan(&bucket.ID, &bucket.Name, &bucket.Tenant, &bucket.StorageClass, &bucket.ReplicaTarget, &bucket.CreatedAt)
	if err != nil {
		return Bucket{}, ErrBucketNotFound
	}
	s.applyEffectiveDurability(ctx, &bucket)
	return bucket, nil
}

func normalizeStorageClass(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (s *Service) applyEffectiveDurability(ctx context.Context, bucket *Bucket) {
	rawClass := normalizeStorageClass(bucket.StorageClass)
	if rawClass == "" {
		rawClass = "inherit"
	}
	bucket.StorageClass = rawClass
	effectiveClass := rawClass
	if effectiveClass == "inherit" {
		if s.settings != nil {
			if policy, err := s.settings.ResolveStoragePolicy(ctx); err == nil && policy.DefaultClass != "" {
				effectiveClass = policy.DefaultClass
			}
		}
		if effectiveClass == "inherit" || effectiveClass == "" {
			effectiveClass = s.defaultStorageClass
			if effectiveClass == "" {
				effectiveClass = "standard"
			}
		}
	}
	bucket.EffectiveStorageClass = effectiveClass
	if bucket.ReplicaTarget > 0 {
		bucket.EffectiveReplicaTarget = bucket.ReplicaTarget
		return
	}
	if s.settings != nil {
		if policy, err := s.settings.ResolveStoragePolicy(ctx); err == nil {
			for _, item := range policy.Policies {
				if item.Name == effectiveClass && item.DefaultReplicas > 0 {
					bucket.EffectiveReplicaTarget = item.DefaultReplicas
					return
				}
			}
		}
	}
	bucket.EffectiveReplicaTarget = config.EffectiveReplicaTarget(effectiveClass, s.clusterReplicas)
}
