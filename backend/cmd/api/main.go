package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"harborshield/backend/internal/admin"
	"harborshield/backend/internal/adminapi"
	"harborshield/backend/internal/audit"
	"harborshield/backend/internal/auth"
	"harborshield/backend/internal/authz"
	"harborshield/backend/internal/buckets"
	"harborshield/backend/internal/config"
	"harborshield/backend/internal/credentials"
	"harborshield/backend/internal/dashboard"
	"harborshield/backend/internal/db"
	"harborshield/backend/internal/events"
	"harborshield/backend/internal/malware"
	metricspkg "harborshield/backend/internal/metrics"
	mw "harborshield/backend/internal/middleware"
	"harborshield/backend/internal/multipart"
	"harborshield/backend/internal/objects"
	"harborshield/backend/internal/oidc"
	"harborshield/backend/internal/policies"
	"harborshield/backend/internal/quotas"
	"harborshield/backend/internal/s3"
	"harborshield/backend/internal/settings"
	"harborshield/backend/internal/storage"
	"harborshield/backend/internal/users"
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

	authService := auth.New(pool, cfg.JWTSecret, cfg.JWTIssuer, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	if err := authService.EnsureBootstrapAdmin(ctx, cfg.AdminBootstrapEmail, cfg.AdminBootstrapPassword); err != nil {
		log.Fatal(err)
	}

	metrics := metricspkg.New()
	metrics.MustRegister()

	settingsSvc := settings.NewService(pool, cfg)
	bucketSvc := buckets.New(pool, cfg.StorageDefaultClass, cfg.StorageDistributedReplicas, settingsSvc)
	dashboardSvc := dashboard.New(pool, cfg.StorageDistributedReplicas, settingsSvc)
	quotaSvc := quotas.NewService(pool)
	localStore, err := storage.NewLocal(cfg.StorageRoot, cfg.StorageMasterKey)
	if err != nil {
		log.Fatal(err)
	}
	var distributedStore *storage.DistributedStore
	if cfg.StorageBackend == "distributed" || len(cfg.StorageDistributedEndpoints) > 0 {
		distributedStore = storage.NewDistributed(cfg.StorageDistributedEndpoints, cfg.StorageDistributedReplicas, cfg.StorageNodeSharedSecret, blobNodeTLSConfig)
	}
	var activeStore storage.BlobStore = localStore
	if cfg.StorageBackend == "distributed" && distributedStore != nil {
		activeStore = distributedStore
	}
	objectSvc := objects.New(pool, localStore, distributedStore, cfg.S3DefaultTenant, quotaSvc, cfg.StorageBackend, cfg.StorageDistributedEndpoints, cfg.StorageDistributedReplicas, cfg.StorageDefaultClass, settingsSvc)
	multipartSvc := multipart.New(pool, activeStore, objectSvc, cfg.S3DefaultTenant)
	credSvc, err := credentials.New(pool, cfg.StorageMasterKey)
	if err != nil {
		log.Fatal(err)
	}
	userSvc := users.New(pool)
	auditSvc := audit.New(pool)
	eventSvc, err := events.NewService(pool, cfg.StorageMasterKey, auditSvc)
	if err != nil {
		log.Fatal(err)
	}
	malwareSvc := malware.NewService(pool, localStore, distributedStore, malware.NewScanner(cfg.EnableClamAV, cfg.ClamAVHost, cfg.ClamAVPort), cfg.MalwareScanMode, auditSvc, eventSvc, settingsSvc)
	adminTokenSvc := adminapi.New(pool)
	policySvc := policies.NewService(pool)
	oidcSvc := oidc.New(cfg, authService, settingsSvc)
	storageTopologySvc := storage.NewTopologyService(pool, distributedStore, cfg.StorageDistributedReplicas, cfg.StorageMasterKey)
	if cfg.StorageBackend == "distributed" {
		if err := storageTopologySvc.SyncConfiguredNodes(ctx, cfg.StorageDistributedEndpoints); err != nil {
			log.Fatal(err)
		}
		if _, err := storageTopologySvc.RefreshConfiguredNodes(ctx); err != nil {
			log.Fatal(err)
		}
	}
	authorizer := authz.New(policySvc)

	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID)
	router.Use(chimiddleware.RealIP)
	router.Use(chimiddleware.Recoverer)
	router.Use(mw.RequestLogger(logger))
	router.Use(mw.Metrics(metrics))
	router.Use(chimiddleware.Timeout(60 * time.Second))

	router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	router.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		if err := redisClient.Ping(r.Context()).Err(); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})
	router.Handle("/metrics", promhttp.Handler())

	admin.Mount(router, admin.RouterDeps{
		Auth:             authService,
		Authorizer:       authorizer,
		AdminTokens:      adminTokenSvc,
		Buckets:          bucketSvc,
		Dashboard:        dashboardSvc,
		Objects:          objectSvc,
		OIDC:             oidcSvc,
		Credentials:      credSvc,
		Users:            userSvc,
		Audit:            auditSvc,
		Events:           eventSvc,
		Malware:          malwareSvc,
		Policies:         policySvc,
		Quotas:           quotaSvc,
		Settings:         settingsSvc,
		StorageTopology:  storageTopologySvc,
		Tokens:           authService.Tokens(),
		AdminIPAllowlist: cfg.AdminIPAllowlist,
	})
	s3.Mount(router, s3.RouterDeps{
		Buckets:            bucketSvc,
		Objects:            objectSvc,
		Multipart:          multipartSvc,
		Credentials:        credSvc,
		Audit:              auditSvc,
		Events:             eventSvc,
		Authorizer:         authorizer,
		Policies:           policySvc,
		Quotas:             quotaSvc,
		Metrics:            metrics,
		PresignTTL:         cfg.S3PresignTTL,
		MaxUploadSizeBytes: cfg.MaxUploadSizeBytes,
		Region:             cfg.S3Region,
	})

	server := &http.Server{
		Addr:              ":" + cfg.APIPort,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("api_listening", slog.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}
