package quotas

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestListAndUpdateQuotas(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	service := NewService(pool)

	var userID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM users WHERE email = 'admin@example.com' LIMIT 1`).Scan(&userID); err != nil {
		t.Fatalf("seed admin lookup failed: %v", err)
	}

	var bucketID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO buckets (tenant_id, name, created_by)
		VALUES ('default', 'quota-test-bucket', $1::uuid)
		RETURNING id::text
	`, userID).Scan(&bucketID); err != nil {
		t.Fatalf("bucket insert failed: %v", err)
	}

	maxBucketBytes := int64(2048)
	maxBucketObjects := int64(25)
	bucketWarning := int32(80)
	if err := service.UpdateBucketQuota(ctx, bucketID, &maxBucketBytes, &maxBucketObjects, &bucketWarning); err != nil {
		t.Fatalf("UpdateBucketQuota failed: %v", err)
	}

	maxUserBytes := int64(8192)
	userWarning := int32(70)
	if err := service.UpdateUserQuota(ctx, userID, &maxUserBytes, &userWarning); err != nil {
		t.Fatalf("UpdateUserQuota failed: %v", err)
	}

	bucketItems, err := service.ListBucketQuotas(ctx)
	if err != nil {
		t.Fatalf("ListBucketQuotas failed: %v", err)
	}

	var foundBucket *BucketQuota
	for i := range bucketItems {
		if bucketItems[i].BucketID == bucketID {
			foundBucket = &bucketItems[i]
			break
		}
	}
	if foundBucket == nil {
		t.Fatalf("expected bucket quota for bucket %s", bucketID)
	}
	if foundBucket.MaxBytes == nil || *foundBucket.MaxBytes != maxBucketBytes {
		t.Fatalf("unexpected bucket max bytes: %#v", foundBucket.MaxBytes)
	}
	if foundBucket.MaxObjects == nil || *foundBucket.MaxObjects != maxBucketObjects {
		t.Fatalf("unexpected bucket max objects: %#v", foundBucket.MaxObjects)
	}
	if foundBucket.WarningThresholdPercent == nil || *foundBucket.WarningThresholdPercent != bucketWarning {
		t.Fatalf("unexpected bucket warning threshold: %#v", foundBucket.WarningThresholdPercent)
	}

	userItems, err := service.ListUserQuotas(ctx)
	if err != nil {
		t.Fatalf("ListUserQuotas failed: %v", err)
	}

	var foundUser *UserQuota
	for i := range userItems {
		if userItems[i].UserID == userID {
			foundUser = &userItems[i]
			break
		}
	}
	if foundUser == nil {
		t.Fatalf("expected user quota for user %s", userID)
	}
	if foundUser.MaxBytes == nil || *foundUser.MaxBytes != maxUserBytes {
		t.Fatalf("unexpected user max bytes: %#v", foundUser.MaxBytes)
	}
	if foundUser.WarningThresholdPercent == nil || *foundUser.WarningThresholdPercent != userWarning {
		t.Fatalf("unexpected user warning threshold: %#v", foundUser.WarningThresholdPercent)
	}
}

func openTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, "postgres://postgres:postgres@127.0.0.1:55432/s3platform_test?sslmode=disable")
	if err != nil {
		t.Skipf("quota integration db unavailable: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("quota integration db unavailable: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
	})

	if _, err := pool.Exec(ctx, `TRUNCATE TABLE
		audit_logs,
		role_bindings,
		role_policy_statements,
		admin_api_tokens,
		s3_credentials,
		refresh_tokens,
		objects,
		multipart_upload_parts,
		multipart_uploads,
		bucket_quotas,
		user_quotas,
		buckets,
		users
		RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate failed: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO users (email, password_hash, role)
		VALUES (
			'admin@example.com',
			'$2a$10$xQYmkkW0Y.A8QpmdjPcU6Oy9xg5E6tLAH0D9bJP4f4PjWCSVaV4fK',
			'superadmin'
		)
	`); err != nil {
		t.Fatalf("seed admin insert failed: %v", err)
	}

	return pool
}
