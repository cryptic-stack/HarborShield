package multipart

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"

	"harborshield/backend/internal/objects"
	"harborshield/backend/internal/storage"
)

type Service struct {
	db      *pgxpool.Pool
	storage storage.BlobStore
	objects *objects.Service
	tenant  string
}

type Upload struct {
	ID                 string
	BucketID           string
	ObjectKey          string
	ContentType        string
	CacheControl       string
	ContentDisposition string
	ContentEncoding    string
	UserMetadata       map[string]string
}

type Part struct {
	PartNumber  int
	ETag        string
	SizeBytes   int64
	StoragePath string
}

type completeMultipartUpload struct {
	XMLName xml.Name       `xml:"CompleteMultipartUpload"`
	Parts   []completePart `xml:"Part"`
}

type completePart struct {
	PartNumber int    `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
}

type InitiateInput struct {
	ObjectKey          string
	ContentType        string
	CacheControl       string
	ContentDisposition string
	ContentEncoding    string
	UserMetadata       map[string]string
	InitiatedBy        string
}

var (
	ErrInvalidPart      = errors.New("invalid part")
	ErrInvalidPartOrder = errors.New("invalid part order")
)

func New(db *pgxpool.Pool, blobStore storage.BlobStore, objectService *objects.Service, tenant string) *Service {
	return &Service{db: db, storage: blobStore, objects: objectService, tenant: tenant}
}

func UploadPath(tenant, bucketID, uploadID string, partNumber int) string {
	return filepath.ToSlash(filepath.Join("tenants", tenant, "buckets", bucketID, "multipart", uploadID, strconv.Itoa(partNumber)))
}

func (s *Service) Initiate(ctx context.Context, bucketID string, input InitiateInput) (Upload, error) {
	var upload Upload
	userMetadata, err := json.Marshal(objects.NormalizeUserMetadata(input.UserMetadata))
	if err != nil {
		return Upload{}, fmt.Errorf("marshal multipart metadata: %w", err)
	}
	err = s.db.QueryRow(ctx, `
		INSERT INTO multipart_uploads (bucket_id, object_key, initiated_by, content_type, cache_control, content_disposition, content_encoding, user_metadata, expires_at)
		VALUES ($1, $2, NULLIF($3, '')::uuid, $4, $5, $6, $7, $8::jsonb, NOW() + INTERVAL '24 hours')
		RETURNING id::text, bucket_id::text, object_key, content_type, cache_control, content_disposition, content_encoding, user_metadata
	`, bucketID, input.ObjectKey, input.InitiatedBy, defaultContentType(input.ContentType), input.CacheControl, input.ContentDisposition, input.ContentEncoding, userMetadata).
		Scan(&upload.ID, &upload.BucketID, &upload.ObjectKey, &upload.ContentType, &upload.CacheControl, &upload.ContentDisposition, &upload.ContentEncoding, &userMetadata)
	if err != nil {
		return Upload{}, err
	}
	upload.UserMetadata, err = objects.DecodeUserMetadata(userMetadata)
	if err != nil {
		return Upload{}, err
	}
	return upload, nil
}

func (s *Service) PutPart(ctx context.Context, uploadID string, partNumber int, expectedSize int64, contentMD5 string, body io.Reader) (Part, error) {
	upload, err := s.getUpload(ctx, uploadID)
	if err != nil {
		return Part{}, err
	}
	payload, err := io.ReadAll(body)
	if err != nil {
		return Part{}, err
	}
	if err := objects.ValidatePayload(payload, expectedSize, contentMD5); err != nil {
		return Part{}, err
	}
	sum := sha256.Sum256(payload)
	etag := hex.EncodeToString(sum[:])
	location := UploadPath(s.tenant, upload.BucketID, upload.ID, partNumber)
	if err := s.storage.Put(ctx, location, bytes.NewReader(payload)); err != nil {
		return Part{}, err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO multipart_upload_parts (upload_id, part_number, size_bytes, etag, storage_path)
		VALUES ($1::uuid, $2, $3, $4, $5)
		ON CONFLICT (upload_id, part_number)
		DO UPDATE SET size_bytes = EXCLUDED.size_bytes, etag = EXCLUDED.etag, storage_path = EXCLUDED.storage_path, created_at = NOW()
	`, uploadID, partNumber, len(payload), etag, location)
	if err != nil {
		return Part{}, err
	}
	return Part{PartNumber: partNumber, ETag: etag, SizeBytes: int64(len(payload)), StoragePath: location}, nil
}

func (s *Service) Complete(ctx context.Context, uploadID, createdBy string, orderedParts []int) (objects.Metadata, error) {
	upload, err := s.getUpload(ctx, uploadID)
	if err != nil {
		return objects.Metadata{}, err
	}
	if len(orderedParts) == 0 {
		return objects.Metadata{}, ErrInvalidPart
	}
	if !strictAscending(orderedParts) {
		return objects.Metadata{}, ErrInvalidPartOrder
	}
	parts, err := s.listParts(ctx, uploadID)
	if err != nil {
		return objects.Metadata{}, err
	}
	partMap := map[int]Part{}
	for _, part := range parts {
		partMap[part.PartNumber] = part
	}

	var combined bytes.Buffer
	for _, number := range orderedParts {
		part, ok := partMap[number]
		if !ok {
			return objects.Metadata{}, ErrInvalidPart
		}
		reader, err := s.storage.Get(ctx, part.StoragePath)
		if err != nil {
			return objects.Metadata{}, err
		}
		_, copyErr := io.Copy(&combined, reader)
		reader.Close()
		if copyErr != nil {
			return objects.Metadata{}, copyErr
		}
	}

	item, err := s.objects.Put(ctx, upload.BucketID, objects.PutInput{
		Key:                upload.ObjectKey,
		ContentType:        upload.ContentType,
		CacheControl:       upload.CacheControl,
		ContentDisposition: upload.ContentDisposition,
		ContentEncoding:    upload.ContentEncoding,
		UserMetadata:       upload.UserMetadata,
		CreatedBy:          createdBy,
		ExpectedSize:       int64(combined.Len()),
		Body:               bytes.NewReader(combined.Bytes()),
	})
	if err != nil {
		return objects.Metadata{}, err
	}
	if err := s.cleanup(ctx, uploadID, parts); err != nil {
		return objects.Metadata{}, err
	}
	return item, nil
}

func (s *Service) Abort(ctx context.Context, uploadID string) error {
	parts, err := s.listParts(ctx, uploadID)
	if err != nil {
		return err
	}
	return s.cleanup(ctx, uploadID, parts)
}

func (s *Service) CleanupExpired(ctx context.Context) (int, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text
		FROM multipart_uploads
		WHERE expires_at <= NOW()
		ORDER BY created_at
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	uploadIDs := make([]string, 0)
	for rows.Next() {
		var uploadID string
		if err := rows.Scan(&uploadID); err != nil {
			return 0, err
		}
		uploadIDs = append(uploadIDs, uploadID)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	cleaned := 0
	for _, uploadID := range uploadIDs {
		parts, err := s.listParts(ctx, uploadID)
		if err != nil {
			return cleaned, err
		}
		if err := s.cleanup(ctx, uploadID, parts); err != nil {
			return cleaned, err
		}
		cleaned++
	}
	return cleaned, nil
}

func ParseCompleteRequest(body io.Reader) ([]int, error) {
	var payload completeMultipartUpload
	if err := xml.NewDecoder(body).Decode(&payload); err != nil {
		return nil, err
	}
	numbers := make([]int, 0, len(payload.Parts))
	for _, part := range payload.Parts {
		if part.PartNumber <= 0 {
			return nil, ErrInvalidPart
		}
		numbers = append(numbers, part.PartNumber)
	}
	return numbers, nil
}

func (s *Service) getUpload(ctx context.Context, uploadID string) (Upload, error) {
	var upload Upload
	var rawUserMetadata []byte
	err := s.db.QueryRow(ctx, `
		SELECT id::text, bucket_id::text, object_key, content_type, cache_control, content_disposition, content_encoding, user_metadata
		FROM multipart_uploads
		WHERE id = $1::uuid AND expires_at > NOW()
	`, uploadID).Scan(&upload.ID, &upload.BucketID, &upload.ObjectKey, &upload.ContentType, &upload.CacheControl, &upload.ContentDisposition, &upload.ContentEncoding, &rawUserMetadata)
	if err != nil {
		return upload, err
	}
	upload.UserMetadata, err = objects.DecodeUserMetadata(rawUserMetadata)
	return upload, err
}

func (s *Service) listParts(ctx context.Context, uploadID string) ([]Part, error) {
	rows, err := s.db.Query(ctx, `
		SELECT part_number, etag, size_bytes, storage_path
		FROM multipart_upload_parts
		WHERE upload_id = $1::uuid
		ORDER BY part_number
	`, uploadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var parts []Part
	for rows.Next() {
		var part Part
		if err := rows.Scan(&part.PartNumber, &part.ETag, &part.SizeBytes, &part.StoragePath); err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	return parts, rows.Err()
}

func (s *Service) cleanup(ctx context.Context, uploadID string, parts []Part) error {
	for _, part := range parts {
		_ = s.storage.Delete(ctx, part.StoragePath)
	}
	_, err := s.db.Exec(ctx, `DELETE FROM multipart_uploads WHERE id = $1::uuid`, uploadID)
	return err
}

func defaultContentType(contentType string) string {
	if contentType == "" {
		return "application/octet-stream"
	}
	return contentType
}

func SortPartNumbers(numbers []int) []int {
	out := append([]int(nil), numbers...)
	sort.Ints(out)
	return out
}

func strictAscending(numbers []int) bool {
	for i := 1; i < len(numbers); i++ {
		if numbers[i] <= numbers[i-1] {
			return false
		}
	}
	return true
}
