package settings

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"harborshield/backend/internal/config"
)

func TestCompleteDeploymentSetupLocalDistributedKeepsEmptyRemoteEndpoints(t *testing.T) {
	pool := openSettingsTestDB(t)
	ctx := context.Background()
	service := NewService(pool, config.Config{
		AppEnv:           "development",
		StorageBackend:   "local",
		StorageMasterKey: "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
	})

	status, err := service.CompleteDeploymentSetup(ctx, DeploymentSetupInput{
		Mode:            "distributed",
		DistributedMode: "local",
	})
	if err != nil {
		t.Fatalf("CompleteDeploymentSetup returned error: %v", err)
	}
	if status.DesiredStorageBackend != "distributed" {
		t.Fatalf("expected distributed desired backend, got %q", status.DesiredStorageBackend)
	}
	if status.DistributedScope != "local" {
		t.Fatalf("expected local distributed scope, got %q", status.DistributedScope)
	}
	if status.RemoteEndpoints == nil {
		t.Fatal("expected remote endpoints to be an empty slice, got nil")
	}
	if len(status.RemoteEndpoints) != 0 {
		t.Fatalf("expected no remote endpoints, got %#v", status.RemoteEndpoints)
	}

	payload, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	jsonText := string(payload)
	if !strings.Contains(jsonText, `"remoteEndpoints":[]`) {
		t.Fatalf("expected remoteEndpoints to serialize as an empty array, got %s", jsonText)
	}

	roundTrip, err := service.DeploymentSetupStatus(ctx)
	if err != nil {
		t.Fatalf("DeploymentSetupStatus returned error: %v", err)
	}
	if roundTrip.RemoteEndpoints == nil {
		t.Fatal("expected round-trip remote endpoints to be an empty slice, got nil")
	}
	if len(roundTrip.RemoteEndpoints) != 0 {
		t.Fatalf("expected round-trip remote endpoints to stay empty, got %#v", roundTrip.RemoteEndpoints)
	}
}

func openSettingsTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, "postgres://postgres:postgres@127.0.0.1:55432/s3platform_test?sslmode=disable")
	if err != nil {
		t.Skipf("settings integration db unavailable: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("settings integration db unavailable: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
	})

	if _, err := pool.Exec(ctx, `TRUNCATE TABLE
		deployment_setups,
		storage_nodes
		RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate failed: %v", err)
	}

	return pool
}
