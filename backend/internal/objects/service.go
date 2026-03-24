package objects

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"harborshield/backend/internal/config"
	"harborshield/backend/internal/quotas"
	"harborshield/backend/internal/settings"
	"harborshield/backend/internal/storage"
)

type Metadata struct {
	ID                 string            `json:"id"`
	BucketID           string            `json:"bucketId"`
	Key                string            `json:"key"`
	VersionID          string            `json:"versionId"`
	SizeBytes          int64             `json:"sizeBytes"`
	ETag               string            `json:"etag"`
	ContentType        string            `json:"contentType"`
	CacheControl       string            `json:"cacheControl"`
	ContentDisposition string            `json:"contentDisposition"`
	ContentEncoding    string            `json:"contentEncoding"`
	UserMetadata       map[string]string `json:"userMetadata"`
	Tags               map[string]string `json:"tags"`
	StoragePath        string            `json:"storagePath"`
	ScanStatus         string            `json:"scanStatus"`
	IsLatest           bool              `json:"isLatest"`
	IsDeleteMarker     bool              `json:"isDeleteMarker"`
	LegalHold          bool              `json:"legalHold"`
	RetentionUntil     string            `json:"retentionUntil"`
	LifecycleDeleteAt  string            `json:"lifecycleDeleteAt"`
	CreatedAt          string            `json:"createdAt"`
}

type PutInput struct {
	Key                string
	ContentType        string
	CacheControl       string
	ContentDisposition string
	ContentEncoding    string
	UserMetadata       map[string]string
	CreatedBy          string
	RetentionUntil     *time.Time
	LifecycleDeleteAt  *time.Time
	Tags               map[string]string
	ExpectedSize       int64
	ContentMD5         string
	Body               io.Reader
}

type Service struct {
	db                  *pgxpool.Pool
	localStorage        storage.BlobStore
	distributedStorage  *storage.DistributedStore
	tenant              string
	quotas              *quotas.Service
	storageBackend      string
	placementEndpoints  []string
	placementReplicas   int
	defaultStorageClass string
	settings            *settings.Service
}

type MigrationStatus struct {
	PendingLocalObjects int64 `json:"pendingLocalObjects"`
	PendingLocalBytes   int64 `json:"pendingLocalBytes"`
	DistributedObjects  int64 `json:"distributedObjects"`
	DistributedBytes    int64 `json:"distributedBytes"`
}

func New(db *pgxpool.Pool, localStore storage.BlobStore, distributedStore *storage.DistributedStore, tenant string, quotaService *quotas.Service, storageBackend string, placementEndpoints []string, placementReplicas int, defaultStorageClass string, settingsSvc *settings.Service) *Service {
	return &Service{
		db:                  db,
		localStorage:        localStore,
		distributedStorage:  distributedStore,
		tenant:              tenant,
		quotas:              quotaService,
		storageBackend:      storageBackend,
		placementEndpoints:  append([]string(nil), placementEndpoints...),
		placementReplicas:   placementReplicas,
		defaultStorageClass: config.NormalizeStorageClass(defaultStorageClass),
		settings:            settingsSvc,
	}
}

func PhysicalPath(tenant, bucketID, objectID string) string {
	return filepath.ToSlash(filepath.Join("tenants", tenant, "buckets", bucketID, "objects", objectID))
}

var (
	ErrIncompleteBody      = errors.New("incomplete body")
	ErrInvalidDigest       = errors.New("invalid digest")
	ErrBadDigest           = errors.New("bad digest")
	ErrRetentionActive     = errors.New("retention is still active")
	ErrLegalHoldActive     = errors.New("legal hold is active")
	ErrNoRestorableVersion = errors.New("no restorable version found")
	ErrVersionNotFound     = errors.New("object version not found")
)

func (s *Service) Put(ctx context.Context, bucketID string, input PutInput) (Metadata, error) {
	payload, err := io.ReadAll(input.Body)
	if err != nil {
		return Metadata{}, fmt.Errorf("read request body: %w", err)
	}
	if err := ValidatePayload(payload, input.ExpectedSize, input.ContentMD5); err != nil {
		return Metadata{}, err
	}
	objectID := uuid.NewString()
	location := PhysicalPath(s.tenant, bucketID, objectID)
	sum := sha256.Sum256(payload)
	etag := hex.EncodeToString(sum[:])
	userMetadata, err := json.Marshal(NormalizeUserMetadata(input.UserMetadata))
	if err != nil {
		return Metadata{}, fmt.Errorf("marshal user metadata: %w", err)
	}
	objectTags, err := json.Marshal(NormalizeTags(input.Tags))
	if err != nil {
		return Metadata{}, fmt.Errorf("marshal object tags: %w", err)
	}

	existingSize, exists, err := s.lookupExistingObject(ctx, bucketID, input.Key)
	if err != nil {
		return Metadata{}, err
	}
	if s.quotas != nil {
		if err := s.quotas.CheckObjectWrite(ctx, bucketID, input.CreatedBy, existingSize, int64(len(payload)), exists); err != nil {
			return Metadata{}, err
		}
	}

	selectedEndpoints, err := s.resolvePlacementEndpoints(ctx, bucketID)
	if err != nil {
		return Metadata{}, err
	}
	writeBackend := s.resolveWriteBackend(selectedEndpoints)
	if writeBackend == "distributed" {
		if s.distributedStorage == nil {
			return Metadata{}, fmt.Errorf("distributed storage backend is unavailable")
		}
		if err := s.distributedStorage.PutToEndpoints(ctx, location, bytes.NewReader(payload), selectedEndpoints); err != nil {
			return Metadata{}, err
		}
	} else if err := s.localStorage.Put(ctx, location, bytes.NewReader(payload)); err != nil {
		return Metadata{}, err
	}

	var item Metadata
	var rawUserMetadata []byte
	var rawTags []byte
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Metadata{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		UPDATE objects
		SET is_latest = FALSE, updated_at = NOW()
		WHERE bucket_id = $1 AND object_key = $2 AND is_latest = TRUE
	`, bucketID, input.Key); err != nil {
		return Metadata{}, err
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO objects (bucket_id, object_key, version_id, is_latest, is_delete_marker, size_bytes, checksum_sha256, etag, content_type, cache_control, content_disposition, content_encoding, user_metadata, object_tags, storage_path, storage_backend, created_by, retention_until, lifecycle_delete_at, scan_status)
		VALUES ($1, $2, gen_random_uuid(), TRUE, FALSE, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11::jsonb, $12, $13, NULLIF($14, '')::uuid, $15, $16, 'pending-scan')
		RETURNING id::text, bucket_id::text, object_key, version_id::text, size_bytes, etag, content_type, cache_control, content_disposition, content_encoding, user_metadata, object_tags, storage_path, scan_status, is_latest, is_delete_marker, legal_hold, COALESCE(retention_until::text, ''), COALESCE(lifecycle_delete_at::text, ''), created_at::text
	`, bucketID, input.Key, len(payload), etag, etag, defaultContentType(input.ContentType), input.CacheControl, input.ContentDisposition, input.ContentEncoding, userMetadata, objectTags, location, writeBackend, input.CreatedBy, input.RetentionUntil, input.LifecycleDeleteAt).
		Scan(&item.ID, &item.BucketID, &item.Key, &item.VersionID, &item.SizeBytes, &item.ETag, &item.ContentType, &item.CacheControl, &item.ContentDisposition, &item.ContentEncoding, &rawUserMetadata, &rawTags, &item.StoragePath, &item.ScanStatus, &item.IsLatest, &item.IsDeleteMarker, &item.LegalHold, &item.RetentionUntil, &item.LifecycleDeleteAt, &item.CreatedAt)
	if err != nil {
		return Metadata{}, err
	}
	if err := s.recordPlacements(ctx, tx, item.ID, location, selectedEndpoints); err != nil {
		return Metadata{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Metadata{}, err
	}
	item.UserMetadata, err = DecodeUserMetadata(rawUserMetadata)
	if err != nil {
		return Metadata{}, err
	}
	item.Tags, err = DecodeTags(rawTags)
	if err != nil {
		return Metadata{}, err
	}

	return item, nil
}

func (s *Service) recordPlacements(ctx context.Context, tx pgx.Tx, objectID, locator string, placementEndpoints []string) error {
	if s.storageBackend != "distributed" || len(placementEndpoints) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `DELETE FROM object_placements WHERE object_id = $1::uuid`, objectID); err != nil {
		return err
	}
	for index, endpoint := range placementEndpoints {
		endpoint = strings.TrimSpace(endpoint)
		if endpoint == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO object_placements (object_id, replica_index, chunk_ordinal, storage_node_id, locator, state, metadata, checksum_sha256, updated_at)
			VALUES (
				$1::uuid,
				$2,
				0,
				(SELECT id FROM storage_nodes WHERE endpoint = $3 LIMIT 1),
				$4,
				'stored',
				jsonb_build_object('endpoint', $3),
				'',
				NOW()
			)
		`, objectID, index, endpoint, locator); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) resolvePlacementEndpoints(ctx context.Context, bucketID string) ([]string, error) {
	if s.storageBackend != "distributed" {
		return nil, nil
	}
	availableEndpoints, err := s.activePlacementEndpoints(ctx)
	if err != nil {
		return nil, err
	}
	if len(availableEndpoints) == 0 {
		return nil, nil
	}
	replicaTarget := s.placementReplicas
	var bucketReplicaTarget int
	var storageClass string
	if err := s.db.QueryRow(ctx, `SELECT COALESCE(replica_target, 0), storage_class FROM buckets WHERE id = $1::uuid`, bucketID).Scan(&bucketReplicaTarget, &storageClass); err == nil {
		if bucketReplicaTarget > 0 {
			replicaTarget = bucketReplicaTarget
		} else {
			effectiveClass := config.NormalizeStorageClass(storageClass)
			if effectiveClass == "" || effectiveClass == "inherit" {
				if s.settings != nil {
					if policy, err := s.settings.ResolveStoragePolicy(ctx); err == nil && policy.DefaultClass != "" {
						effectiveClass = policy.DefaultClass
					}
				}
				if effectiveClass == "" || effectiveClass == "inherit" {
					effectiveClass = s.defaultStorageClass
				}
			}
			if s.settings != nil {
				if policy, err := s.settings.ResolveStoragePolicy(ctx); err == nil {
					for _, item := range policy.Policies {
						if item.Name == effectiveClass && item.DefaultReplicas > 0 {
							replicaTarget = item.DefaultReplicas
							break
						}
					}
				}
			}
			if replicaTarget <= 0 {
				replicaTarget = config.EffectiveReplicaTarget(effectiveClass, replicaTarget)
			}
		}
	}
	if replicaTarget <= 0 || replicaTarget > len(availableEndpoints) {
		replicaTarget = len(availableEndpoints)
	}
	return append([]string(nil), availableEndpoints[:replicaTarget]...), nil
}

func (s *Service) activePlacementEndpoints(ctx context.Context) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT endpoint
		FROM storage_nodes
		WHERE operator_state = 'active'
		ORDER BY name ASC, endpoint ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	endpoints := make([]string, 0)
	for rows.Next() {
		var endpoint string
		if err := rows.Scan(&endpoint); err != nil {
			return nil, err
		}
		endpoints = append(endpoints, strings.TrimSpace(endpoint))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(endpoints) > 0 {
		return endpoints, nil
	}
	var totalNodes int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM storage_nodes`).Scan(&totalNodes); err != nil {
		return nil, err
	}
	if totalNodes > 0 {
		return []string{}, nil
	}
	return append([]string(nil), s.placementEndpoints...), nil
}

func (s *Service) lookupExistingObject(ctx context.Context, bucketID, key string) (int64, bool, error) {
	var existingSize int64
	err := s.db.QueryRow(ctx, `
		SELECT size_bytes
		FROM objects
		WHERE bucket_id = $1 AND object_key = $2 AND deleted_at IS NULL AND is_latest = TRUE AND is_delete_marker = FALSE
	`, bucketID, key).Scan(&existingSize)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return existingSize, true, nil
}

func (s *Service) List(ctx context.Context, bucketID, prefix string) ([]Metadata, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, bucket_id::text, object_key, version_id::text, size_bytes, etag, content_type, cache_control, content_disposition, content_encoding, user_metadata, object_tags, storage_path, scan_status, is_latest, is_delete_marker, legal_hold, COALESCE(retention_until::text, ''), COALESCE(lifecycle_delete_at::text, ''), created_at::text
		FROM objects
		WHERE bucket_id = $1 AND deleted_at IS NULL AND is_latest = TRUE AND is_delete_marker = FALSE AND object_key LIKE $2
		ORDER BY object_key
	`, bucketID, prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]Metadata, 0)
	for rows.Next() {
		var item Metadata
		var rawUserMetadata []byte
		var rawTags []byte
		if err := rows.Scan(&item.ID, &item.BucketID, &item.Key, &item.VersionID, &item.SizeBytes, &item.ETag, &item.ContentType, &item.CacheControl, &item.ContentDisposition, &item.ContentEncoding, &rawUserMetadata, &rawTags, &item.StoragePath, &item.ScanStatus, &item.IsLatest, &item.IsDeleteMarker, &item.LegalHold, &item.RetentionUntil, &item.LifecycleDeleteAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.UserMetadata, err = DecodeUserMetadata(rawUserMetadata)
		if err != nil {
			return nil, err
		}
		item.Tags, err = DecodeTags(rawTags)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Service) ListVersions(ctx context.Context, bucketID, key string) ([]Metadata, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, bucket_id::text, object_key, version_id::text, size_bytes, etag, content_type, cache_control, content_disposition, content_encoding, user_metadata, object_tags, storage_path, scan_status, is_latest, is_delete_marker, legal_hold, COALESCE(retention_until::text, ''), COALESCE(lifecycle_delete_at::text, ''), created_at::text
		FROM objects
		WHERE bucket_id = $1 AND object_key = $2
		ORDER BY created_at DESC
	`, bucketID, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]Metadata, 0)
	for rows.Next() {
		var item Metadata
		var rawUserMetadata []byte
		var rawTags []byte
		if err := rows.Scan(&item.ID, &item.BucketID, &item.Key, &item.VersionID, &item.SizeBytes, &item.ETag, &item.ContentType, &item.CacheControl, &item.ContentDisposition, &item.ContentEncoding, &rawUserMetadata, &rawTags, &item.StoragePath, &item.ScanStatus, &item.IsLatest, &item.IsDeleteMarker, &item.LegalHold, &item.RetentionUntil, &item.LifecycleDeleteAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.UserMetadata, err = DecodeUserMetadata(rawUserMetadata)
		if err != nil {
			return nil, err
		}
		item.Tags, err = DecodeTags(rawTags)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Service) ListAllVersions(ctx context.Context, bucketID, prefix string) ([]Metadata, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, bucket_id::text, object_key, version_id::text, size_bytes, etag, content_type, cache_control, content_disposition, content_encoding, user_metadata, object_tags, storage_path, scan_status, is_latest, is_delete_marker, legal_hold, COALESCE(retention_until::text, ''), COALESCE(lifecycle_delete_at::text, ''), created_at::text
		FROM objects
		WHERE bucket_id = $1 AND deleted_at IS NULL AND object_key LIKE $2
		ORDER BY object_key, created_at DESC
	`, bucketID, prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]Metadata, 0)
	for rows.Next() {
		var item Metadata
		var rawUserMetadata []byte
		var rawTags []byte
		if err := rows.Scan(&item.ID, &item.BucketID, &item.Key, &item.VersionID, &item.SizeBytes, &item.ETag, &item.ContentType, &item.CacheControl, &item.ContentDisposition, &item.ContentEncoding, &rawUserMetadata, &rawTags, &item.StoragePath, &item.ScanStatus, &item.IsLatest, &item.IsDeleteMarker, &item.LegalHold, &item.RetentionUntil, &item.LifecycleDeleteAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.UserMetadata, err = DecodeUserMetadata(rawUserMetadata)
		if err != nil {
			return nil, err
		}
		item.Tags, err = DecodeTags(rawTags)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Service) GetByKey(ctx context.Context, bucketID, key string) (Metadata, io.ReadCloser, error) {
	return s.getVersion(ctx, bucketID, key, "")
}

func (s *Service) GetByVersion(ctx context.Context, bucketID, key, versionID string) (Metadata, io.ReadCloser, error) {
	return s.getVersion(ctx, bucketID, key, versionID)
}

func (s *Service) getVersion(ctx context.Context, bucketID, key, versionID string) (Metadata, io.ReadCloser, error) {
	var item Metadata
	var rawUserMetadata []byte
	query := `
		SELECT id::text, bucket_id::text, object_key, version_id::text, size_bytes, etag, content_type, cache_control, content_disposition, content_encoding, user_metadata, object_tags, storage_path, scan_status, is_latest, is_delete_marker, legal_hold, COALESCE(retention_until::text, ''), COALESCE(lifecycle_delete_at::text, ''), created_at::text
		FROM objects
		WHERE bucket_id = $1 AND object_key = $2 AND deleted_at IS NULL AND is_delete_marker = FALSE
	`
	args := []any{bucketID, key}
	var rawTags []byte
	if versionID == "" {
		query += ` AND is_latest = TRUE`
	} else {
		query += ` AND version_id = $3::uuid`
		args = append(args, versionID)
	}
	err := s.db.QueryRow(ctx, query, args...).Scan(&item.ID, &item.BucketID, &item.Key, &item.VersionID, &item.SizeBytes, &item.ETag, &item.ContentType, &item.CacheControl, &item.ContentDisposition, &item.ContentEncoding, &rawUserMetadata, &rawTags, &item.StoragePath, &item.ScanStatus, &item.IsLatest, &item.IsDeleteMarker, &item.LegalHold, &item.RetentionUntil, &item.LifecycleDeleteAt, &item.CreatedAt)
	if err != nil {
		return Metadata{}, nil, err
	}
	item.UserMetadata, err = DecodeUserMetadata(rawUserMetadata)
	if err != nil {
		return Metadata{}, nil, err
	}
	item.Tags, err = DecodeTags(rawTags)
	if err != nil {
		return Metadata{}, nil, err
	}
	var storageBackend string
	err = s.db.QueryRow(ctx, `SELECT COALESCE(storage_backend, 'filesystem') FROM objects WHERE id = $1::uuid`, item.ID).Scan(&storageBackend)
	if err != nil {
		return Metadata{}, nil, err
	}
	reader, err := s.storeForBackend(storageBackend).Get(ctx, item.StoragePath)
	if err != nil {
		return Metadata{}, nil, err
	}
	return item, reader, nil
}

func (s *Service) Delete(ctx context.Context, bucketID, key string) error {
	var retentionUntil *time.Time
	var legalHold bool
	if err := s.db.QueryRow(ctx, `SELECT retention_until, legal_hold FROM objects WHERE bucket_id = $1 AND object_key = $2 AND is_latest = TRUE`, bucketID, key).Scan(&retentionUntil, &legalHold); err != nil {
		return err
	}
	if legalHold {
		return ErrLegalHoldActive
	}
	if retentionUntil != nil && retentionUntil.After(time.Now().UTC()) {
		return ErrRetentionActive
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		UPDATE objects SET is_latest = FALSE, updated_at = NOW()
		WHERE bucket_id = $1 AND object_key = $2 AND is_latest = TRUE
	`, bucketID, key); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO objects (bucket_id, object_key, version_id, is_latest, is_delete_marker, size_bytes, checksum_sha256, etag, content_type, object_tags, storage_path, scan_status)
		VALUES ($1, $2, gen_random_uuid(), TRUE, TRUE, 0, '', '', 'application/octet-stream', '{}'::jsonb, '', 'clean')
	`, bucketID, key); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Service) DeleteVersion(ctx context.Context, bucketID, key, versionID string) error {
	if _, err := uuid.Parse(versionID); err != nil {
		return ErrVersionNotFound
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var objectID, storagePath, storageBackend string
	var retentionUntil *time.Time
	var legalHold, isLatest bool
	if err := tx.QueryRow(ctx, `
		SELECT id::text, COALESCE(storage_path, ''), COALESCE(storage_backend, 'filesystem'), retention_until, legal_hold, is_latest
		FROM objects
		WHERE bucket_id = $1 AND object_key = $2 AND version_id = $3::uuid AND deleted_at IS NULL
	`, bucketID, key, versionID).Scan(&objectID, &storagePath, &storageBackend, &retentionUntil, &legalHold, &isLatest); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrVersionNotFound
		}
		return err
	}
	if legalHold {
		return ErrLegalHoldActive
	}
	if retentionUntil != nil && retentionUntil.After(time.Now().UTC()) {
		return ErrRetentionActive
	}

	if _, err := tx.Exec(ctx, `DELETE FROM objects WHERE id = $1::uuid`, objectID); err != nil {
		return err
	}
	if isLatest {
		if _, err := tx.Exec(ctx, `
			UPDATE objects
			SET is_latest = FALSE, updated_at = NOW()
			WHERE bucket_id = $1 AND object_key = $2
		`, bucketID, key); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE objects
			SET is_latest = TRUE, updated_at = NOW()
			WHERE id = (
				SELECT id
				FROM objects
				WHERE bucket_id = $1 AND object_key = $2 AND deleted_at IS NULL
				ORDER BY created_at DESC
				LIMIT 1
			)
		`, bucketID, key); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if storagePath != "" {
		if err := s.storeForBackend(storageBackend).Delete(ctx, storagePath); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) RestoreLatest(ctx context.Context, bucketID, key string) (Metadata, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Metadata{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		UPDATE objects
		SET is_latest = FALSE, updated_at = NOW()
		WHERE bucket_id = $1 AND object_key = $2 AND is_latest = TRUE
	`, bucketID, key); err != nil {
		return Metadata{}, err
	}

	var item Metadata
	var rawUserMetadata []byte
	var rawTags []byte
	err = tx.QueryRow(ctx, `
		UPDATE objects
		SET is_latest = TRUE, updated_at = NOW()
		WHERE id = (
			SELECT id
			FROM objects
			WHERE bucket_id = $1
			  AND object_key = $2
			  AND deleted_at IS NULL
			  AND is_delete_marker = FALSE
			ORDER BY created_at DESC
			LIMIT 1
		)
		RETURNING id::text, bucket_id::text, object_key, version_id::text, size_bytes, etag, content_type, cache_control, content_disposition, content_encoding, user_metadata, object_tags, storage_path, scan_status, is_latest, is_delete_marker, legal_hold, COALESCE(retention_until::text, ''), COALESCE(lifecycle_delete_at::text, ''), created_at::text
	`, bucketID, key).Scan(&item.ID, &item.BucketID, &item.Key, &item.VersionID, &item.SizeBytes, &item.ETag, &item.ContentType, &item.CacheControl, &item.ContentDisposition, &item.ContentEncoding, &rawUserMetadata, &rawTags, &item.StoragePath, &item.ScanStatus, &item.IsLatest, &item.IsDeleteMarker, &item.LegalHold, &item.RetentionUntil, &item.LifecycleDeleteAt, &item.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Metadata{}, ErrNoRestorableVersion
		}
		return Metadata{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Metadata{}, err
	}
	item.UserMetadata, err = DecodeUserMetadata(rawUserMetadata)
	if err != nil {
		return Metadata{}, err
	}
	item.Tags, err = DecodeTags(rawTags)
	if err != nil {
		return Metadata{}, err
	}
	return item, nil
}

func (s *Service) ApplyLifecycleExpirations(ctx context.Context, now time.Time) (int, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE objects
		SET deleted_at = $1, updated_at = NOW()
		WHERE deleted_at IS NULL
		  AND lifecycle_delete_at IS NOT NULL
		  AND lifecycle_delete_at <= $1
		  AND legal_hold = FALSE
		  AND (retention_until IS NULL OR retention_until <= $1)
	`, now.UTC())
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *Service) PurgeDeletedBefore(ctx context.Context, cutoff time.Time) (int, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, storage_path
		FROM objects
		WHERE deleted_at IS NOT NULL AND deleted_at <= $1
	`, cutoff)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type candidate struct {
		id   string
		path string
	}
	candidates := make([]candidate, 0)
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.path); err != nil {
			return 0, err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	purged := 0
	for _, item := range candidates {
		backend := s.lookupStoredBackend(ctx, item.id)
		if err := s.storeForBackend(backend).Delete(ctx, item.path); err != nil {
			return purged, err
		}
		tag, err := s.db.Exec(ctx, `DELETE FROM objects WHERE id = $1::uuid AND deleted_at IS NOT NULL AND deleted_at <= $2`, item.id, cutoff)
		if err != nil {
			return purged, err
		}
		if tag.RowsAffected() > 0 {
			purged++
		}
	}
	return purged, nil
}

func (s *Service) MigrationStatus(ctx context.Context) (MigrationStatus, error) {
	var status MigrationStatus
	err := s.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE deleted_at IS NULL AND is_delete_marker = FALSE AND storage_path <> '' AND COALESCE(storage_backend, 'filesystem') <> 'distributed'),
			COALESCE(SUM(size_bytes) FILTER (WHERE deleted_at IS NULL AND is_delete_marker = FALSE AND storage_path <> '' AND COALESCE(storage_backend, 'filesystem') <> 'distributed'), 0),
			COUNT(*) FILTER (WHERE deleted_at IS NULL AND is_delete_marker = FALSE AND storage_path <> '' AND COALESCE(storage_backend, 'filesystem') = 'distributed'),
			COALESCE(SUM(size_bytes) FILTER (WHERE deleted_at IS NULL AND is_delete_marker = FALSE AND storage_path <> '' AND COALESCE(storage_backend, 'filesystem') = 'distributed'), 0)
		FROM objects
	`).Scan(&status.PendingLocalObjects, &status.PendingLocalBytes, &status.DistributedObjects, &status.DistributedBytes)
	return status, err
}

func (s *Service) MigrateLocalObjectsToDistributed(ctx context.Context, limit int) (int64, error) {
	if s.distributedStorage == nil {
		return 0, fmt.Errorf("distributed storage backend is unavailable")
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, bucket_id::text, storage_path
		FROM objects
		WHERE deleted_at IS NULL
		  AND is_delete_marker = FALSE
		  AND storage_path <> ''
		  AND COALESCE(storage_backend, 'filesystem') <> 'distributed'
		ORDER BY created_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type candidate struct {
		id       string
		bucketID string
		path     string
	}
	candidates := make([]candidate, 0)
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.bucketID, &item.path); err != nil {
			return 0, err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	var migrated int64
	for _, item := range candidates {
		selectedEndpoints, err := s.resolvePlacementEndpoints(ctx, item.bucketID)
		if err != nil {
			return migrated, err
		}
		if len(selectedEndpoints) == 0 {
			return migrated, fmt.Errorf("no active distributed storage nodes are available for migration")
		}
		reader, err := s.localStorage.Get(ctx, item.path)
		if err != nil {
			return migrated, err
		}
		payload, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			return migrated, err
		}
		if err := s.distributedStorage.PutToEndpoints(ctx, item.path, bytes.NewReader(payload), selectedEndpoints); err != nil {
			return migrated, err
		}

		tx, err := s.db.Begin(ctx)
		if err != nil {
			return migrated, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE objects
			SET storage_backend = 'distributed',
			    updated_at = NOW()
			WHERE id = $1::uuid
		`, item.id); err != nil {
			_ = tx.Rollback(ctx)
			return migrated, err
		}
		if err := s.recordPlacements(ctx, tx, item.id, item.path, selectedEndpoints); err != nil {
			_ = tx.Rollback(ctx)
			return migrated, err
		}
		if err := tx.Commit(ctx); err != nil {
			return migrated, err
		}
		_ = s.localStorage.Delete(ctx, item.path)
		migrated++
	}
	return migrated, nil
}

func (s *Service) resolveWriteBackend(selectedEndpoints []string) string {
	if s.storageBackend == "distributed" && s.distributedStorage != nil && len(selectedEndpoints) > 0 {
		return "distributed"
	}
	return "local"
}

func (s *Service) storeForBackend(backend string) storage.BlobStore {
	if normalizeStoredBackend(backend) == "distributed" && s.distributedStorage != nil {
		return s.distributedStorage
	}
	return s.localStorage
}

func (s *Service) lookupStoredBackend(ctx context.Context, objectID string) string {
	var backend string
	if err := s.db.QueryRow(ctx, `SELECT COALESCE(storage_backend, 'filesystem') FROM objects WHERE id = $1::uuid`, objectID).Scan(&backend); err != nil {
		return "local"
	}
	return backend
}

func normalizeStoredBackend(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "distributed":
		return "distributed"
	default:
		return "local"
	}
}

func (s *Service) Head(ctx context.Context, bucketID, key string) (Metadata, error) {
	var item Metadata
	var rawUserMetadata []byte
	var rawTags []byte
	err := s.db.QueryRow(ctx, `
		SELECT id::text, bucket_id::text, object_key, version_id::text, size_bytes, etag, content_type, cache_control, content_disposition, content_encoding, user_metadata, object_tags, storage_path, scan_status, is_latest, is_delete_marker, legal_hold, COALESCE(retention_until::text, ''), COALESCE(lifecycle_delete_at::text, ''), created_at::text
		FROM objects
		WHERE bucket_id = $1 AND object_key = $2 AND deleted_at IS NULL AND is_latest = TRUE AND is_delete_marker = FALSE
	`, bucketID, key).Scan(&item.ID, &item.BucketID, &item.Key, &item.VersionID, &item.SizeBytes, &item.ETag, &item.ContentType, &item.CacheControl, &item.ContentDisposition, &item.ContentEncoding, &rawUserMetadata, &rawTags, &item.StoragePath, &item.ScanStatus, &item.IsLatest, &item.IsDeleteMarker, &item.LegalHold, &item.RetentionUntil, &item.LifecycleDeleteAt, &item.CreatedAt)
	if err != nil {
		return Metadata{}, fmt.Errorf("head object: %w", err)
	}
	item.UserMetadata, err = DecodeUserMetadata(rawUserMetadata)
	if err != nil {
		return Metadata{}, err
	}
	item.Tags, err = DecodeTags(rawTags)
	if err != nil {
		return Metadata{}, err
	}
	return item, nil
}

func (s *Service) GetTags(ctx context.Context, bucketID, key, versionID string) (map[string]string, error) {
	item, reader, err := s.getVersion(ctx, bucketID, key, versionID)
	if reader != nil {
		_ = reader.Close()
	}
	if err != nil {
		return nil, err
	}
	return item.Tags, nil
}

func (s *Service) SetLegalHold(ctx context.Context, bucketID, key, versionID string, legalHold bool) (Metadata, error) {
	var item Metadata
	var rawUserMetadata []byte
	var rawTags []byte
	query := `
		UPDATE objects
		SET legal_hold = $3, updated_at = NOW()
		WHERE bucket_id = $1 AND object_key = $2 AND deleted_at IS NULL
	`
	args := []any{bucketID, key, legalHold}
	if versionID == "" {
		query += ` AND is_latest = TRUE`
	} else {
		query += ` AND version_id = $4::uuid`
		args = append(args, versionID)
	}
	query += `
		RETURNING id::text, bucket_id::text, object_key, version_id::text, size_bytes, etag, content_type, cache_control, content_disposition, content_encoding, user_metadata, object_tags, storage_path, scan_status, is_latest, is_delete_marker, legal_hold, COALESCE(retention_until::text, ''), COALESCE(lifecycle_delete_at::text, ''), created_at::text
	`
	if err := s.db.QueryRow(ctx, query, args...).Scan(&item.ID, &item.BucketID, &item.Key, &item.VersionID, &item.SizeBytes, &item.ETag, &item.ContentType, &item.CacheControl, &item.ContentDisposition, &item.ContentEncoding, &rawUserMetadata, &rawTags, &item.StoragePath, &item.ScanStatus, &item.IsLatest, &item.IsDeleteMarker, &item.LegalHold, &item.RetentionUntil, &item.LifecycleDeleteAt, &item.CreatedAt); err != nil {
		return Metadata{}, err
	}
	var err error
	item.UserMetadata, err = DecodeUserMetadata(rawUserMetadata)
	if err != nil {
		return Metadata{}, err
	}
	item.Tags, err = DecodeTags(rawTags)
	if err != nil {
		return Metadata{}, err
	}
	return item, nil
}

func (s *Service) PutTags(ctx context.Context, bucketID, key, versionID string, tags map[string]string) (Metadata, error) {
	rawTags, err := json.Marshal(NormalizeTags(tags))
	if err != nil {
		return Metadata{}, fmt.Errorf("marshal object tags: %w", err)
	}
	var item Metadata
	var rawUserMetadata []byte
	var storedTags []byte
	query := `
		UPDATE objects
		SET object_tags = $3::jsonb, updated_at = NOW()
		WHERE bucket_id = $1 AND object_key = $2 AND deleted_at IS NULL AND is_delete_marker = FALSE
	`
	args := []any{bucketID, key, rawTags}
	if versionID == "" {
		query += ` AND is_latest = TRUE`
	} else {
		query += ` AND version_id = $4::uuid`
		args = append(args, versionID)
	}
	query += `
		RETURNING id::text, bucket_id::text, object_key, version_id::text, size_bytes, etag, content_type, cache_control, content_disposition, content_encoding, user_metadata, object_tags, storage_path, scan_status, is_latest, is_delete_marker, legal_hold, COALESCE(retention_until::text, ''), COALESCE(lifecycle_delete_at::text, ''), created_at::text
	`
	if err := s.db.QueryRow(ctx, query, args...).Scan(&item.ID, &item.BucketID, &item.Key, &item.VersionID, &item.SizeBytes, &item.ETag, &item.ContentType, &item.CacheControl, &item.ContentDisposition, &item.ContentEncoding, &rawUserMetadata, &storedTags, &item.StoragePath, &item.ScanStatus, &item.IsLatest, &item.IsDeleteMarker, &item.LegalHold, &item.RetentionUntil, &item.LifecycleDeleteAt, &item.CreatedAt); err != nil {
		return Metadata{}, err
	}
	item.UserMetadata, err = DecodeUserMetadata(rawUserMetadata)
	if err != nil {
		return Metadata{}, err
	}
	item.Tags, err = DecodeTags(storedTags)
	if err != nil {
		return Metadata{}, err
	}
	return item, nil
}

func (s *Service) Copy(ctx context.Context, sourceBucketID, sourceKey, sourceVersionID, destinationBucketID string, input PutInput) (Metadata, error) {
	source, reader, err := s.getVersion(ctx, sourceBucketID, sourceKey, sourceVersionID)
	if err != nil {
		return Metadata{}, err
	}
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil {
		return Metadata{}, fmt.Errorf("read source object: %w", err)
	}
	if input.ContentType == "" {
		input.ContentType = source.ContentType
	}
	if input.CacheControl == "" {
		input.CacheControl = source.CacheControl
	}
	if input.ContentDisposition == "" {
		input.ContentDisposition = source.ContentDisposition
	}
	if input.ContentEncoding == "" {
		input.ContentEncoding = source.ContentEncoding
	}
	if input.UserMetadata == nil {
		input.UserMetadata = source.UserMetadata
	}
	if input.Tags == nil {
		input.Tags = source.Tags
	}
	input.ExpectedSize = int64(len(body))
	input.Body = bytes.NewReader(body)
	return s.Put(ctx, destinationBucketID, input)
}

func ValidatePayload(payload []byte, expectedSize int64, contentMD5 string) error {
	if expectedSize >= 0 && int64(len(payload)) != expectedSize {
		return ErrIncompleteBody
	}
	if contentMD5 == "" {
		return nil
	}
	expectedDigest, err := base64.StdEncoding.DecodeString(contentMD5)
	if err != nil {
		return ErrInvalidDigest
	}
	sum := md5.Sum(payload)
	if !bytes.Equal(expectedDigest, sum[:]) {
		return ErrBadDigest
	}
	return nil
}

func NormalizeUserMetadata(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func NormalizeTags(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		if key == "" {
			continue
		}
		output[key] = value
	}
	return output
}

func DecodeUserMetadata(raw []byte) (map[string]string, error) {
	if len(raw) == 0 {
		return map[string]string{}, nil
	}
	var values map[string]string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("decode user metadata: %w", err)
	}
	return NormalizeUserMetadata(values), nil
}

func DecodeTags(raw []byte) (map[string]string, error) {
	if len(raw) == 0 {
		return map[string]string{}, nil
	}
	var values map[string]string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("decode object tags: %w", err)
	}
	return NormalizeTags(values), nil
}

func ParseTags(encoded string) (map[string]string, error) {
	if strings.TrimSpace(encoded) == "" {
		return map[string]string{}, nil
	}
	values, err := url.ParseQuery(encoded)
	if err != nil {
		return nil, fmt.Errorf("parse object tags: %w", err)
	}
	output := make(map[string]string, len(values))
	for key, items := range values {
		if len(items) == 0 {
			output[key] = ""
			continue
		}
		output[key] = items[0]
	}
	return NormalizeTags(output), nil
}

func defaultContentType(contentType string) string {
	if contentType == "" {
		return "application/octet-stream"
	}
	return contentType
}
