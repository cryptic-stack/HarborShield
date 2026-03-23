package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

var validPlatformRoles = map[string]struct{}{
	"superadmin":   {},
	"admin":        {},
	"auditor":      {},
	"bucket-admin": {},
	"readonly":     {},
}

var supportedStorageClasses = []string{"standard", "reduced-redundancy", "archive-ready"}

type StorageClassPolicy struct {
	Name            string `json:"name"`
	Label           string `json:"label"`
	Description     string `json:"description"`
	DefaultReplicas int    `json:"defaultReplicas"`
}

type Config struct {
	AppEnv                       string
	APIPort                      string
	PostgresDSN                  string
	RedisAddr                    string
	RedisPassword                string
	JWTSecret                    string
	JWTIssuer                    string
	AccessTokenTTL               time.Duration
	RefreshTokenTTL              time.Duration
	AdminBootstrapEmail          string
	AdminBootstrapPassword       string
	StorageBackend               string
	StorageRoot                  string
	StorageDistributedEndpoints  []string
	StorageDistributedReplicas   int
	StorageNodeSharedSecret      string
	StorageNodeTLSCAFile         string
	StorageNodeTLSClientCertFile string
	StorageNodeTLSClientKeyFile  string
	BlobNodeTLSCertFile          string
	BlobNodeTLSKeyFile           string
	BlobNodeTLSClientCAFile      string
	StorageDefaultClass          string
	S3Region                     string
	S3DefaultTenant              string
	S3PresignTTL                 time.Duration
	MaxUploadSizeBytes           int64
	CORSOrigins                  []string
	LogLevel                     string
	EnableClamAV                 bool
	ClamAVHost                   string
	ClamAVPort                   int
	MalwareScanMode              string
	AdminIPAllowlist             []string
	StorageMasterKey             string
	SoftDeleteRetention          time.Duration
	OIDCEnabled                  bool
	OIDCIssuerURL                string
	OIDCClientID                 string
	OIDCClientSecret             string
	OIDCRedirectURL              string
	OIDCScopes                   []string
	OIDCRoleClaim                string
	OIDCRoleMap                  map[string]string
	OIDCDefaultRole              string
}

func Load() (Config, error) {
	accessTTL, err := time.ParseDuration(getEnv("ACCESS_TOKEN_TTL", "15m"))
	if err != nil {
		return Config{}, fmt.Errorf("parse ACCESS_TOKEN_TTL: %w", err)
	}
	refreshTTL, err := time.ParseDuration(getEnv("REFRESH_TOKEN_TTL", "168h"))
	if err != nil {
		return Config{}, fmt.Errorf("parse REFRESH_TOKEN_TTL: %w", err)
	}
	presignTTL, err := time.ParseDuration(getEnv("S3_PRESIGN_TTL", "15m"))
	if err != nil {
		return Config{}, fmt.Errorf("parse S3_PRESIGN_TTL: %w", err)
	}
	clamPort, err := strconv.Atoi(getEnv("CLAMAV_PORT", "3310"))
	if err != nil {
		return Config{}, fmt.Errorf("parse CLAMAV_PORT: %w", err)
	}
	maxUploadSizeBytes, err := strconv.ParseInt(getEnv("MAX_UPLOAD_SIZE_BYTES", "104857600"), 10, 64)
	if err != nil {
		return Config{}, fmt.Errorf("parse MAX_UPLOAD_SIZE_BYTES: %w", err)
	}
	distributedReplicas, err := strconv.Atoi(getEnv("STORAGE_DISTRIBUTED_REPLICAS", "0"))
	if err != nil {
		return Config{}, fmt.Errorf("parse STORAGE_DISTRIBUTED_REPLICAS: %w", err)
	}
	softDeleteRetention, err := time.ParseDuration(getEnv("SOFT_DELETE_RETENTION", "24h"))
	if err != nil {
		return Config{}, fmt.Errorf("parse SOFT_DELETE_RETENTION: %w", err)
	}

	cfg := Config{
		AppEnv:                       getEnv("APP_ENV", "development"),
		APIPort:                      getEnv("API_PORT", "8080"),
		RedisAddr:                    fmt.Sprintf("%s:%s", getEnv("REDIS_HOST", "redis"), getEnv("REDIS_PORT", "6379")),
		JWTIssuer:                    getEnv("JWT_ISSUER", "harborshield"),
		AccessTokenTTL:               accessTTL,
		RefreshTokenTTL:              refreshTTL,
		AdminBootstrapEmail:          getEnv("ADMIN_BOOTSTRAP_EMAIL", "admin@example.com"),
		StorageBackend:               strings.ToLower(getEnv("STORAGE_BACKEND", "local")),
		StorageRoot:                  getEnv("STORAGE_ROOT", "/data"),
		StorageDistributedEndpoints:  splitCSV(getEnv("STORAGE_DISTRIBUTED_ENDPOINTS", "")),
		StorageDistributedReplicas:   distributedReplicas,
		StorageNodeTLSCAFile:         getEnv("STORAGE_NODE_TLS_CA_FILE", ""),
		StorageNodeTLSClientCertFile: getEnv("STORAGE_NODE_TLS_CLIENT_CERT_FILE", ""),
		StorageNodeTLSClientKeyFile:  getEnv("STORAGE_NODE_TLS_CLIENT_KEY_FILE", ""),
		BlobNodeTLSCertFile:          getEnv("BLOBNODE_TLS_CERT_FILE", ""),
		BlobNodeTLSKeyFile:           getEnv("BLOBNODE_TLS_KEY_FILE", ""),
		BlobNodeTLSClientCAFile:      getEnv("BLOBNODE_TLS_CLIENT_CA_FILE", ""),
		StorageDefaultClass:          NormalizeStorageClass(getEnv("STORAGE_DEFAULT_CLASS", "standard")),
		S3Region:                     getEnv("S3_REGION", "us-east-1"),
		S3DefaultTenant:              getEnv("S3_DEFAULT_TENANT", "default"),
		S3PresignTTL:                 presignTTL,
		MaxUploadSizeBytes:           maxUploadSizeBytes,
		CORSOrigins:                  splitCSV(getEnv("CORS_ORIGINS", "http://localhost:3000")),
		LogLevel:                     getEnv("LOG_LEVEL", "info"),
		EnableClamAV:                 strings.EqualFold(getEnv("ENABLE_CLAMAV", "false"), "true"),
		ClamAVHost:                   getEnv("CLAMAV_HOST", "clamav"),
		ClamAVPort:                   clamPort,
		MalwareScanMode:              strings.ToLower(getEnv("MALWARE_SCAN_MODE", "advisory")),
		AdminIPAllowlist:             splitCSV(getEnv("ADMIN_IP_ALLOWLIST", "")),
		SoftDeleteRetention:          softDeleteRetention,
		OIDCEnabled:                  strings.EqualFold(getEnv("OIDC_ENABLED", "false"), "true"),
		OIDCIssuerURL:                getEnv("OIDC_ISSUER_URL", ""),
		OIDCClientID:                 getEnv("OIDC_CLIENT_ID", ""),
		OIDCRedirectURL:              getEnv("OIDC_REDIRECT_URL", ""),
		OIDCScopes:                   splitCSV(getEnv("OIDC_SCOPES", "openid,email,profile")),
		OIDCRoleClaim:                getEnv("OIDC_ROLE_CLAIM", ""),
		OIDCRoleMap:                  parseRoleMap(getEnv("OIDC_ROLE_MAP", "")),
		OIDCDefaultRole:              getEnv("OIDC_DEFAULT_ROLE", "admin"),
	}
	postgresPassword, err := getSecretEnv("POSTGRES_PASSWORD", "change_me")
	if err != nil {
		return Config{}, err
	}
	redisPassword, err := getSecretEnv("REDIS_PASSWORD", "")
	if err != nil {
		return Config{}, err
	}
	jwtSecret, err := getSecretEnv("JWT_SECRET", "")
	if err != nil {
		return Config{}, err
	}
	adminBootstrapPassword, err := getSecretEnv("ADMIN_BOOTSTRAP_PASSWORD", "change_me_now")
	if err != nil {
		return Config{}, err
	}
	storageMasterKey, err := getSecretEnv("STORAGE_MASTER_KEY", "")
	if err != nil {
		return Config{}, err
	}
	storageNodeSharedSecret, err := getSecretEnv("STORAGE_NODE_SHARED_SECRET", "")
	if err != nil {
		return Config{}, err
	}
	oidcClientSecret, err := getSecretEnv("OIDC_CLIENT_SECRET", "")
	if err != nil {
		return Config{}, err
	}
	cfg.PostgresDSN = fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		getEnv("POSTGRES_USER", "s3platform"),
		postgresPassword,
		getEnv("POSTGRES_HOST", "postgres"),
		getEnv("POSTGRES_PORT", "5432"),
		getEnv("POSTGRES_DB", "s3platform"),
	)
	cfg.RedisPassword = redisPassword
	cfg.JWTSecret = jwtSecret
	cfg.AdminBootstrapPassword = adminBootstrapPassword
	cfg.StorageMasterKey = storageMasterKey
	cfg.StorageNodeSharedSecret = storageNodeSharedSecret
	cfg.OIDCClientSecret = oidcClientSecret
	if cfg.StorageNodeSharedSecret == "" {
		cfg.StorageNodeSharedSecret = cfg.StorageMasterKey
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	if cfg.StorageBackend == "distributed" && cfg.StorageDistributedReplicas == 0 {
		cfg.StorageDistributedReplicas = len(cfg.StorageDistributedEndpoints)
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getSecretEnv(key, fallback string) (string, error) {
	if value := os.Getenv(key); value != "" {
		return value, nil
	}
	filePath := os.Getenv(key + "_FILE")
	if filePath == "" {
		return fallback, nil
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read %s_FILE: %w", key, err)
	}
	return strings.TrimSpace(string(content)), nil
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseRoleMap(value string) map[string]string {
	if value == "" {
		return nil
	}
	out := map[string]string{}
	for _, entry := range strings.Split(value, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		role := strings.TrimSpace(parts[1])
		if key == "" || role == "" {
			continue
		}
		out[key] = role
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (c Config) Validate() error {
	if c.MalwareScanMode == "" {
		c.MalwareScanMode = "advisory"
	}
	if c.StorageBackend == "" {
		c.StorageBackend = "local"
	}
	if c.JWTSecret == "" {
		return errors.New("JWT_SECRET is required")
	}
	if c.AdminBootstrapEmail == "" || c.AdminBootstrapPassword == "" {
		return errors.New("bootstrap admin credentials are required")
	}
	if c.StorageRoot == "" {
		return errors.New("STORAGE_ROOT is required")
	}
	if c.StorageBackend != "local" && c.StorageBackend != "distributed" {
		return errors.New("STORAGE_BACKEND must be local or distributed")
	}
	if c.StorageDistributedReplicas < 0 {
		return errors.New("STORAGE_DISTRIBUTED_REPLICAS must be zero or greater")
	}
	if c.StorageDefaultClass == "" {
		c.StorageDefaultClass = "standard"
	}
	if !IsValidStorageClass(c.StorageDefaultClass) {
		return fmt.Errorf("STORAGE_DEFAULT_CLASS must be one of %s", strings.Join(SupportedStorageClasses(), ", "))
	}
	if c.StorageBackend == "distributed" {
		if len(c.StorageDistributedEndpoints) == 0 {
			return errors.New("STORAGE_DISTRIBUTED_ENDPOINTS is required when STORAGE_BACKEND is distributed")
		}
		if c.StorageDistributedReplicas == 0 {
			c.StorageDistributedReplicas = len(c.StorageDistributedEndpoints)
		}
		if c.StorageDistributedReplicas > len(c.StorageDistributedEndpoints) {
			return errors.New("STORAGE_DISTRIBUTED_REPLICAS cannot exceed configured storage endpoints")
		}
	}
	if c.StorageMasterKey == "" {
		return errors.New("STORAGE_MASTER_KEY is required")
	}
	if c.StorageNodeSharedSecret == "" {
		return errors.New("STORAGE_NODE_SHARED_SECRET is required")
	}
	if (c.StorageNodeTLSClientCertFile == "") != (c.StorageNodeTLSClientKeyFile == "") {
		return errors.New("STORAGE_NODE_TLS_CLIENT_CERT_FILE and STORAGE_NODE_TLS_CLIENT_KEY_FILE must be set together")
	}
	if (c.BlobNodeTLSCertFile == "") != (c.BlobNodeTLSKeyFile == "") {
		return errors.New("BLOBNODE_TLS_CERT_FILE and BLOBNODE_TLS_KEY_FILE must be set together")
	}
	if c.BlobNodeTLSClientCAFile != "" && (c.BlobNodeTLSCertFile == "" || c.BlobNodeTLSKeyFile == "") {
		return errors.New("BLOBNODE_TLS_CERT_FILE and BLOBNODE_TLS_KEY_FILE are required when BLOBNODE_TLS_CLIENT_CA_FILE is set")
	}
	if _, err := decodeStorageKey(c.StorageMasterKey); err != nil {
		return fmt.Errorf("invalid STORAGE_MASTER_KEY: %w", err)
	}
	if c.MaxUploadSizeBytes <= 0 {
		return errors.New("MAX_UPLOAD_SIZE_BYTES must be greater than zero")
	}
	if c.SoftDeleteRetention <= 0 {
		return errors.New("SOFT_DELETE_RETENTION must be greater than zero")
	}
	if c.MalwareScanMode != "advisory" && c.MalwareScanMode != "enforcement" {
		return errors.New("MALWARE_SCAN_MODE must be advisory or enforcement")
	}
	if c.OIDCEnabled {
		if c.OIDCIssuerURL == "" || c.OIDCClientID == "" || c.OIDCRedirectURL == "" {
			return errors.New("OIDC_ISSUER_URL, OIDC_CLIENT_ID, and OIDC_REDIRECT_URL are required when OIDC is enabled")
		}
		if c.OIDCDefaultRole == "" {
			return errors.New("OIDC_DEFAULT_ROLE is required when OIDC is enabled")
		}
		if !isValidPlatformRole(c.OIDCDefaultRole) {
			return fmt.Errorf("OIDC_DEFAULT_ROLE must be one of %s", strings.Join(sortedPlatformRoles(), ", "))
		}
		for sourceValue, role := range c.OIDCRoleMap {
			if !isValidPlatformRole(role) {
				return fmt.Errorf("OIDC_ROLE_MAP entry %q targets invalid role %q", sourceValue, role)
			}
		}
	}
	if c.AppEnv != "development" {
		if isPlaceholder(c.JWTSecret) {
			return errors.New("JWT_SECRET uses a placeholder value outside development")
		}
		if isPlaceholder(c.AdminBootstrapPassword) {
			return errors.New("ADMIN_BOOTSTRAP_PASSWORD uses a placeholder value outside development")
		}
		if isPlaceholder(extractPassword(c.PostgresDSN)) {
			return errors.New("POSTGRES_PASSWORD uses a placeholder value outside development")
		}
		if isPlaceholder(c.RedisPassword) {
			return errors.New("REDIS_PASSWORD uses a placeholder value outside development")
		}
		if isPlaceholder(c.StorageMasterKey) {
			return errors.New("STORAGE_MASTER_KEY uses a placeholder value outside development")
		}
		if isPlaceholder(c.StorageNodeSharedSecret) {
			return errors.New("STORAGE_NODE_SHARED_SECRET uses a placeholder value outside development")
		}
		if c.OIDCEnabled && isPlaceholder(c.OIDCClientSecret) {
			return errors.New("OIDC_CLIENT_SECRET uses a placeholder value outside development")
		}
	}
	return nil
}

func isPlaceholder(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "", "change_me", "change_me_now", "change_me_long_random_value":
		return true
	default:
		return strings.Contains(value, "change_me")
	}
}

func extractPassword(dsn string) string {
	if !strings.Contains(dsn, "://") || !strings.Contains(dsn, "@") {
		return ""
	}
	withoutScheme := strings.SplitN(dsn, "://", 2)[1]
	userInfo := strings.SplitN(withoutScheme, "@", 2)[0]
	parts := strings.SplitN(userInfo, ":", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}

func decodeStorageKey(value string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("expected 32 decoded bytes, got %d", len(key))
	}
	return key, nil
}

func isValidPlatformRole(value string) bool {
	_, ok := validPlatformRoles[value]
	return ok
}

func sortedPlatformRoles() []string {
	return []string{"superadmin", "admin", "auditor", "bucket-admin", "readonly"}
}

func IsValidPlatformRole(value string) bool {
	return isValidPlatformRole(value)
}

func SortedPlatformRoles() []string {
	return sortedPlatformRoles()
}

func NormalizeStorageClass(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func IsValidStorageClass(value string) bool {
	value = NormalizeStorageClass(value)
	for _, item := range supportedStorageClasses {
		if item == value {
			return true
		}
	}
	return false
}

func SupportedStorageClasses() []string {
	return append([]string(nil), supportedStorageClasses...)
}

func StorageClassPolicies(clusterReplicaTarget int) []StorageClassPolicy {
	classNames := SupportedStorageClasses()
	out := make([]StorageClassPolicy, 0, len(classNames))
	for _, className := range classNames {
		out = append(out, StorageClassPolicy{
			Name:            className,
			Label:           storageClassLabel(className),
			Description:     storageClassDescription(className, clusterReplicaTarget),
			DefaultReplicas: EffectiveReplicaTarget(className, clusterReplicaTarget),
		})
	}
	return out
}

func EffectiveReplicaTarget(class string, clusterReplicaTarget int) int {
	if clusterReplicaTarget <= 0 {
		clusterReplicaTarget = 1
	}
	switch NormalizeStorageClass(class) {
	case "reduced-redundancy":
		if clusterReplicaTarget > 1 {
			return clusterReplicaTarget - 1
		}
		return 1
	case "archive-ready":
		return 1
	default:
		return clusterReplicaTarget
	}
}

func storageClassLabel(class string) string {
	switch NormalizeStorageClass(class) {
	case "reduced-redundancy":
		return "Reduced Redundancy"
	case "archive-ready":
		return "Archive Ready"
	default:
		return "Standard"
	}
}

func storageClassDescription(class string, clusterReplicaTarget int) string {
	switch NormalizeStorageClass(class) {
	case "reduced-redundancy":
		return fmt.Sprintf("Lower replica count for less critical data. Defaults to %d replicas.", EffectiveReplicaTarget(class, clusterReplicaTarget))
	case "archive-ready":
		return "Single replica staging class for archive-oriented or easily reproducible data."
	default:
		return fmt.Sprintf("Balanced default durability class. Defaults to %d replicas.", EffectiveReplicaTarget(class, clusterReplicaTarget))
	}
}
