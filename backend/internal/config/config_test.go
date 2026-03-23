package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateRejectsMissingJWTSecret(t *testing.T) {
	cfg := Config{
		AppEnv:                  "development",
		AdminBootstrapEmail:     "admin@example.com",
		AdminBootstrapPassword:  "change_me_now",
		StorageRoot:             "/data",
		StorageMasterKey:        "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		StorageNodeSharedSecret: "node-secret",
		MaxUploadSizeBytes:      1024,
		SoftDeleteRetention:     24,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing JWT secret to fail validation")
	}
}

func TestValidateRejectsPlaceholderSecretsOutsideDevelopment(t *testing.T) {
	cfg := Config{
		AppEnv:                  "production",
		JWTSecret:               "change_me_long_random_value",
		AdminBootstrapEmail:     "admin@example.com",
		AdminBootstrapPassword:  "change_me_now",
		StorageRoot:             "/data",
		PostgresDSN:             "postgres://user:change_me@postgres:5432/s3platform?sslmode=disable",
		RedisPassword:           "change_me",
		StorageMasterKey:        "change_me",
		StorageNodeSharedSecret: "change_me",
		MaxUploadSizeBytes:      1024,
		SoftDeleteRetention:     24,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected placeholder secrets to fail validation")
	}
}

func TestValidateAllowsDevelopmentPlaceholders(t *testing.T) {
	cfg := Config{
		AppEnv:                  "development",
		JWTSecret:               "change_me_long_random_value",
		AdminBootstrapEmail:     "admin@example.com",
		AdminBootstrapPassword:  "change_me_now",
		StorageRoot:             "/data",
		PostgresDSN:             "postgres://user:change_me@postgres:5432/s3platform?sslmode=disable",
		RedisPassword:           "change_me",
		StorageMasterKey:        "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		StorageNodeSharedSecret: "node-secret",
		MaxUploadSizeBytes:      1024,
		SoftDeleteRetention:     24,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected development config to pass, got %v", err)
	}
}

func TestValidateRejectsNonPositiveSoftDeleteRetention(t *testing.T) {
	cfg := Config{
		AppEnv:                  "development",
		JWTSecret:               "change_me_long_random_value",
		AdminBootstrapEmail:     "admin@example.com",
		AdminBootstrapPassword:  "change_me_now",
		StorageRoot:             "/data",
		StorageMasterKey:        "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		StorageNodeSharedSecret: "node-secret",
		MaxUploadSizeBytes:      1024,
		SoftDeleteRetention:     0,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected non-positive soft delete retention to fail validation")
	}
}

func TestGetSecretEnvReadsFileOverride(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	tempDir := t.TempDir()
	secretPath := filepath.Join(tempDir, "jwt-secret.txt")
	if err := os.WriteFile(secretPath, []byte("from-file\n"), 0o600); err != nil {
		t.Fatalf("write secret file: %v", err)
	}
	t.Setenv("JWT_SECRET_FILE", secretPath)

	value, err := getSecretEnv("JWT_SECRET", "fallback")
	if err != nil {
		t.Fatalf("getSecretEnv returned error: %v", err)
	}
	if value != "from-file" {
		t.Fatalf("expected trimmed file value, got %q", value)
	}
}

func TestGetSecretEnvPrefersDirectEnv(t *testing.T) {
	t.Setenv("JWT_SECRET", "direct-value")
	t.Setenv("JWT_SECRET_FILE", "C:\\ignored.txt")

	value, err := getSecretEnv("JWT_SECRET", "fallback")
	if err != nil {
		t.Fatalf("getSecretEnv returned error: %v", err)
	}
	if value != "direct-value" {
		t.Fatalf("expected direct env value, got %q", value)
	}
}

func TestValidateRequiresOIDCFieldsWhenEnabled(t *testing.T) {
	cfg := Config{
		AppEnv:                  "development",
		JWTSecret:               "dev-secret",
		AdminBootstrapEmail:     "admin@example.com",
		AdminBootstrapPassword:  "change_me_now",
		StorageRoot:             "/data",
		StorageMasterKey:        "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		StorageNodeSharedSecret: "node-secret",
		MaxUploadSizeBytes:      1024,
		SoftDeleteRetention:     24,
		OIDCEnabled:             true,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected oidc-enabled config without required fields to fail validation")
	}
}

func TestValidateRejectsInvalidOIDCDefaultRole(t *testing.T) {
	cfg := Config{
		AppEnv:                  "development",
		JWTSecret:               "dev-secret",
		AdminBootstrapEmail:     "admin@example.com",
		AdminBootstrapPassword:  "change_me_now",
		StorageRoot:             "/data",
		StorageMasterKey:        "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		StorageNodeSharedSecret: "node-secret",
		MaxUploadSizeBytes:      1024,
		SoftDeleteRetention:     24,
		OIDCEnabled:             true,
		OIDCIssuerURL:           "https://issuer.example.com",
		OIDCClientID:            "client-id",
		OIDCRedirectURL:         "http://localhost/api/v1/auth/oidc/callback",
		OIDCDefaultRole:         "power-user",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid OIDC default role to fail validation")
	}
}

func TestValidateRejectsInvalidOIDCRoleMapTarget(t *testing.T) {
	cfg := Config{
		AppEnv:                  "development",
		JWTSecret:               "dev-secret",
		AdminBootstrapEmail:     "admin@example.com",
		AdminBootstrapPassword:  "change_me_now",
		StorageRoot:             "/data",
		StorageMasterKey:        "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		StorageNodeSharedSecret: "node-secret",
		MaxUploadSizeBytes:      1024,
		SoftDeleteRetention:     24,
		OIDCEnabled:             true,
		OIDCIssuerURL:           "https://issuer.example.com",
		OIDCClientID:            "client-id",
		OIDCRedirectURL:         "http://localhost/api/v1/auth/oidc/callback",
		OIDCDefaultRole:         "admin",
		OIDCRoleMap: map[string]string{
			"storage-admin": "power-user",
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid OIDC role map target to fail validation")
	}
}

func TestParseRoleMap(t *testing.T) {
	got := parseRoleMap("storage-admin=admin, auditors=auditor ,invalid")
	if got["storage-admin"] != "admin" {
		t.Fatalf("expected storage-admin mapping, got %#v", got)
	}
	if got["auditors"] != "auditor" {
		t.Fatalf("expected auditors mapping, got %#v", got)
	}
	if _, exists := got["invalid"]; exists {
		t.Fatalf("expected invalid entry to be ignored, got %#v", got)
	}
}

func TestValidateRejectsInvalidStorageBackend(t *testing.T) {
	cfg := Config{
		AppEnv:                  "development",
		JWTSecret:               "dev-secret",
		AdminBootstrapEmail:     "admin@example.com",
		AdminBootstrapPassword:  "change_me_now",
		StorageBackend:          "mystery",
		StorageRoot:             "/data",
		StorageMasterKey:        "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		StorageNodeSharedSecret: "node-secret",
		MaxUploadSizeBytes:      1024,
		SoftDeleteRetention:     24,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid storage backend to fail validation")
	}
}

func TestValidateRejectsTooManyDistributedReplicas(t *testing.T) {
	cfg := Config{
		AppEnv:                      "development",
		JWTSecret:                   "dev-secret",
		AdminBootstrapEmail:         "admin@example.com",
		AdminBootstrapPassword:      "change_me_now",
		StorageBackend:              "distributed",
		StorageRoot:                 "/data",
		StorageDistributedEndpoints: []string{"http://node-a:9100", "http://node-b:9100"},
		StorageDistributedReplicas:  3,
		StorageMasterKey:            "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		StorageNodeSharedSecret:     "node-secret",
		MaxUploadSizeBytes:          1024,
		SoftDeleteRetention:         24,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected too many distributed replicas to fail validation")
	}
}

func TestValidateAllowsDistributedReplicasWithinConfiguredEndpoints(t *testing.T) {
	cfg := Config{
		AppEnv:                      "development",
		JWTSecret:                   "dev-secret",
		AdminBootstrapEmail:         "admin@example.com",
		AdminBootstrapPassword:      "change_me_now",
		StorageBackend:              "distributed",
		StorageRoot:                 "/data",
		StorageDistributedEndpoints: []string{"http://node-a:9100", "http://node-b:9100", "http://node-c:9100"},
		StorageDistributedReplicas:  2,
		StorageMasterKey:            "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		StorageNodeSharedSecret:     "node-secret",
		MaxUploadSizeBytes:          1024,
		SoftDeleteRetention:         24,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected distributed replica config to pass, got %v", err)
	}
}

func TestValidateRejectsInvalidStorageDefaultClass(t *testing.T) {
	cfg := Config{
		AppEnv:                  "development",
		JWTSecret:               "dev-secret",
		AdminBootstrapEmail:     "admin@example.com",
		AdminBootstrapPassword:  "change_me_now",
		StorageRoot:             "/data",
		StorageMasterKey:        "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		StorageNodeSharedSecret: "node-secret",
		StorageDefaultClass:     "frozen",
		MaxUploadSizeBytes:      1024,
		SoftDeleteRetention:     24,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid storage default class to fail validation")
	}
}

func TestEffectiveReplicaTargetByStorageClass(t *testing.T) {
	if got := EffectiveReplicaTarget("standard", 3); got != 3 {
		t.Fatalf("expected standard target 3, got %d", got)
	}
	if got := EffectiveReplicaTarget("reduced-redundancy", 3); got != 2 {
		t.Fatalf("expected reduced redundancy target 2, got %d", got)
	}
	if got := EffectiveReplicaTarget("archive-ready", 3); got != 1 {
		t.Fatalf("expected archive-ready target 1, got %d", got)
	}
}
