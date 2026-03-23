package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/redis/go-redis/v9"

	"harborshield/backend/internal/audit"
	"harborshield/backend/internal/config"
	"harborshield/backend/internal/db"
	"harborshield/backend/internal/events"
	"harborshield/backend/internal/jobs"
	"harborshield/backend/internal/malware"
	"harborshield/backend/internal/multipart"
	"harborshield/backend/internal/objects"
	"harborshield/backend/internal/quotas"
	"harborshield/backend/internal/settings"
	"harborshield/backend/internal/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	blobNodeTLSConfig, err := storage.LoadBlobNodeClientTLSConfig(cfg.StorageNodeTLSCAFile, cfg.StorageNodeTLSClientCertFile, cfg.StorageNodeTLSClientKeyFile)
	if err != nil {
		log.Fatal(err)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx := context.Background()

	pool, err := db.Open(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool, filepath.Join("migrations")); err != nil {
		log.Fatal(err)
	}

	redisClient := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword})
	defer redisClient.Close()

	blobStore, err := storage.NewStore(cfg.StorageBackend, cfg.StorageRoot, cfg.StorageMasterKey, cfg.StorageDistributedEndpoints, cfg.StorageDistributedReplicas, cfg.StorageNodeSharedSecret, blobNodeTLSConfig)
	if err != nil {
		log.Fatal(err)
	}
	var distributedStore *storage.DistributedStore
	if typedStore, ok := blobStore.(*storage.DistributedStore); ok {
		distributedStore = typedStore
	}
	auditSvc := audit.New(pool)
	storageTopologySvc := storage.NewTopologyService(pool, distributedStore, cfg.StorageDistributedReplicas, cfg.StorageMasterKey)
	quotaSvc := quotas.NewService(pool)
	settingsSvc := settings.NewService(pool, cfg)
	objectSvc := objects.New(pool, blobStore, cfg.S3DefaultTenant, quotaSvc, cfg.StorageBackend, cfg.StorageDistributedEndpoints, cfg.StorageDistributedReplicas, cfg.StorageDefaultClass, settingsSvc)
	multipartSvc := multipart.New(pool, blobStore, objectSvc, cfg.S3DefaultTenant)
	eventSvc, err := events.NewService(pool, cfg.StorageMasterKey, auditSvc)
	if err != nil {
		log.Fatal(err)
	}
	malwareSvc := malware.NewService(pool, blobStore, malware.NewScanner(cfg.EnableClamAV, cfg.ClamAVHost, cfg.ClamAVPort), cfg.MalwareScanMode, auditSvc, eventSvc)
	jobSvc := jobs.NewService(pool)

	recurringJobs := []struct {
		name     string
		interval time.Duration
		run      func(context.Context) (int64, error)
	}{
		{name: "multipart_cleanup", interval: 30 * time.Second, run: func(ctx context.Context) (int64, error) {
			count, err := multipartSvc.CleanupExpired(ctx)
			return int64(count), err
		}},
		{name: "soft_delete_purge", interval: 30 * time.Second, run: func(ctx context.Context) (int64, error) {
			count, err := objectSvc.PurgeDeletedBefore(ctx, time.Now().UTC().Add(-cfg.SoftDeleteRetention))
			return int64(count), err
		}},
		{name: "lifecycle_expiration", interval: 30 * time.Second, run: func(ctx context.Context) (int64, error) {
			count, err := objectSvc.ApplyLifecycleExpirations(ctx, time.Now().UTC())
			return int64(count), err
		}},
		{name: "bucket_quota_recalc", interval: 30 * time.Second, run: quotaSvc.RecalculateBucketUsage},
		{name: "user_quota_recalc", interval: 30 * time.Second, run: quotaSvc.RecalculateUserUsage},
		{name: "event_delivery_retry", interval: 15 * time.Second, run: func(ctx context.Context) (int64, error) {
			return eventSvc.DeliverPending(ctx, 10)
		}},
		{name: "malware_scan", interval: 15 * time.Second, run: func(ctx context.Context) (int64, error) {
			return malwareSvc.ProcessPending(ctx, 10)
		}},
	}
	if cfg.StorageBackend == "distributed" {
		if err := storageTopologySvc.SyncConfiguredNodes(ctx, cfg.StorageDistributedEndpoints); err != nil {
			log.Fatal(err)
		}
		if _, err := storageTopologySvc.RefreshConfiguredNodes(ctx); err != nil {
			log.Fatal(err)
		}
		recurringJobs = append(recurringJobs, struct {
			name     string
			interval time.Duration
			run      func(context.Context) (int64, error)
		}{
			name:     "storage_topology_refresh",
			interval: 15 * time.Second,
			run:      storageTopologySvc.RefreshConfiguredNodes,
		}, struct {
			name     string
			interval time.Duration
			run      func(context.Context) (int64, error)
		}{
			name:     "storage_placement_health",
			interval: 20 * time.Second,
			run:      storageTopologySvc.RefreshPlacementHealth,
		}, struct {
			name     string
			interval time.Duration
			run      func(context.Context) (int64, error)
		}{
			name:     "storage_repair",
			interval: 20 * time.Second,
			run:      storageTopologySvc.RepairDegradedPlacements,
		}, struct {
			name     string
			interval time.Duration
			run      func(context.Context) (int64, error)
		}{
			name:     "storage_rebalance",
			interval: 30 * time.Second,
			run:      storageTopologySvc.RebalancePlacements,
		})
	}

	for _, job := range recurringJobs {
		if err := jobSvc.EnsureRecurring(ctx, job.name, job.interval, map[string]any{"interval": job.interval.String()}); err != nil {
			log.Fatal(err)
		}
	}

	type recurringHandler struct {
		interval time.Duration
		run      func(context.Context) (int64, error)
	}
	jobHandlers := map[string]recurringHandler{}
	for _, job := range recurringJobs {
		jobHandlers[job.name] = recurringHandler{interval: job.interval, run: job.run}
	}

	runDueJobs := func() {
		if err := jobSvc.ResetStaleRunning(ctx, 10*time.Second); err != nil {
			logger.Error("job_stale_recovery_failed", slog.String("service", "worker"), slog.String("error", err.Error()))
			return
		}

		for {
			job, err := jobSvc.ClaimDue(ctx)
			if err != nil {
				if errors.Is(err, jobs.ErrNoDueJobs) {
					return
				}
				logger.Error("job_claim_failed", slog.String("service", "worker"), slog.String("error", err.Error()))
				return
			}

			handler, ok := jobHandlers[job.JobType]
			if !ok {
				logger.Error("job_handler_missing", slog.String("service", "worker"), slog.String("job_type", job.JobType))
				_ = jobSvc.Fail(ctx, job.ID, time.Now().UTC().Add(30*time.Second), "missing handler")
				continue
			}

			count, runErr := handler.run(ctx)
			if runErr != nil {
				logger.Error("job_execution_failed", slog.String("service", "worker"), slog.String("job_type", job.JobType), slog.String("error", runErr.Error()))
				_ = jobSvc.Fail(ctx, job.ID, time.Now().UTC().Add(15*time.Second), runErr.Error())
				continue
			}

			logger.Info("job_execution_completed", slog.String("service", "worker"), slog.String("job_type", job.JobType), slog.Int64("affected", count))
			_ = jobSvc.Complete(ctx, job.ID, time.Now().UTC().Add(handler.interval))
		}
	}

	runDueJobs()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		runDueJobs()
	}
}
