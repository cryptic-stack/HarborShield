package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"harborshield/backend/internal/config"
	cryptopkg "harborshield/backend/internal/crypto"
)

type Service struct {
	db     *pgxpool.Pool
	cfg    config.Config
	sealer *cryptopkg.Sealer
}

type StoragePolicy struct {
	DefaultClass string                      `json:"defaultClass"`
	Policies     []config.StorageClassPolicy `json:"policies"`
}

type DeploymentSetup struct {
	Completed               bool     `json:"completed"`
	Required                bool     `json:"required"`
	DesiredStorageBackend   string   `json:"desiredStorageBackend"`
	DistributedScope        string   `json:"distributedScope"`
	RemoteEndpoints         []string `json:"remoteEndpoints"`
	RuntimeStorageBackend   string   `json:"runtimeStorageBackend"`
	RuntimeEndpointCount    int      `json:"runtimeEndpointCount"`
	ApplyRequired           bool     `json:"applyRequired"`
	RecommendedLocalProfile string   `json:"recommendedLocalProfile"`
}

type DeploymentSetupInput struct {
	Mode            string   `json:"mode"`
	DistributedMode string   `json:"distributedMode"`
	RemoteEndpoints []string `json:"remoteEndpoints"`
}

type Snapshot struct {
	AppEnv                      string                      `json:"appEnv"`
	Region                      string                      `json:"region"`
	DefaultTenant               string                      `json:"defaultTenant"`
	PresignTTL                  string                      `json:"presignTTL"`
	MaxUploadSizeBytes          int64                       `json:"maxUploadSizeBytes"`
	StorageBackend              string                      `json:"storageBackend"`
	StorageRoot                 string                      `json:"storageRoot"`
	StorageDistributedEndpoints []string                    `json:"storageDistributedEndpoints"`
	StorageDistributedReplicas  int                         `json:"storageDistributedReplicas"`
	StorageDefaultClass         string                      `json:"storageDefaultClass"`
	StorageSupportedClasses     []string                    `json:"storageSupportedClasses"`
	StorageClassPolicies        []config.StorageClassPolicy `json:"storageClassPolicies"`
	StorageEncrypted            bool                        `json:"storageEncrypted"`
	ClamAVEnabled               bool                        `json:"clamavEnabled"`
	MalwareScanMode             string                      `json:"malwareScanMode"`
	AdminIPAllowlist            []string                    `json:"adminIpAllowlist"`
	CORSOrigins                 []string                    `json:"corsOrigins"`
	LogLevel                    string                      `json:"logLevel"`
	OIDCEnabled                 bool                        `json:"oidcEnabled"`
	OIDCIssuerURL               string                      `json:"oidcIssuerUrl"`
	OIDCClientID                string                      `json:"oidcClientId"`
	OIDCClientSecretConfigured  bool                        `json:"oidcClientSecretConfigured"`
	OIDCRedirectURL             string                      `json:"oidcRedirectUrl"`
	OIDCScopes                  []string                    `json:"oidcScopes"`
	OIDCRoleClaim               string                      `json:"oidcRoleClaim"`
	OIDCDefaultRole             string                      `json:"oidcDefaultRole"`
	OIDCRoleMap                 map[string]string           `json:"oidcRoleMap"`
}

type OIDCSettings struct {
	Enabled                bool              `json:"enabled"`
	IssuerURL              string            `json:"issuerUrl"`
	ClientID               string            `json:"clientId"`
	ClientSecret           string            `json:"-"`
	ClientSecretConfigured bool              `json:"clientSecretConfigured"`
	RedirectURL            string            `json:"redirectUrl"`
	Scopes                 []string          `json:"scopes"`
	RoleClaim              string            `json:"roleClaim"`
	DefaultRole            string            `json:"defaultRole"`
	RoleMap                map[string]string `json:"roleMap"`
}

type OIDCSettingsInput struct {
	Enabled      bool              `json:"enabled"`
	IssuerURL    string            `json:"issuerUrl"`
	ClientID     string            `json:"clientId"`
	ClientSecret string            `json:"clientSecret"`
	RedirectURL  string            `json:"redirectUrl"`
	Scopes       []string          `json:"scopes"`
	RoleClaim    string            `json:"roleClaim"`
	DefaultRole  string            `json:"defaultRole"`
	RoleMap      map[string]string `json:"roleMap"`
}

type MalwareSettings struct {
	ScanMode string `json:"scanMode"`
}

type OIDCTestResult struct {
	IssuerURL             string   `json:"issuerUrl"`
	AuthorizationEndpoint string   `json:"authorizationEndpoint"`
	TokenEndpoint         string   `json:"tokenEndpoint"`
	UserInfoEndpoint      string   `json:"userInfoEndpoint"`
	JWKSURL               string   `json:"jwksUrl"`
	Scopes                []string `json:"scopes"`
	Message               string   `json:"message"`
}

func NewService(db *pgxpool.Pool, cfg config.Config) *Service {
	sealer, err := cryptopkg.NewSealer(cfg.StorageMasterKey)
	if err != nil {
		panic(fmt.Sprintf("settings sealer: %v", err))
	}
	return &Service{db: db, cfg: cfg, sealer: sealer}
}

func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	policy, err := s.ResolveStoragePolicy(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	distributedEndpoints, err := s.ResolveDistributedEndpoints(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	malwareSettings, err := s.ResolveMalwareSettings(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	oidcSettings, err := s.ResolveOIDCSettings(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		AppEnv:                      s.cfg.AppEnv,
		Region:                      s.cfg.S3Region,
		DefaultTenant:               s.cfg.S3DefaultTenant,
		PresignTTL:                  s.cfg.S3PresignTTL.String(),
		MaxUploadSizeBytes:          s.cfg.MaxUploadSizeBytes,
		StorageBackend:              s.cfg.StorageBackend,
		StorageRoot:                 s.cfg.StorageRoot,
		StorageDistributedEndpoints: distributedEndpoints,
		StorageDistributedReplicas:  s.cfg.StorageDistributedReplicas,
		StorageDefaultClass:         policy.DefaultClass,
		StorageSupportedClasses:     config.SupportedStorageClasses(),
		StorageClassPolicies:        policy.Policies,
		StorageEncrypted:            s.cfg.StorageMasterKey != "",
		ClamAVEnabled:               s.cfg.EnableClamAV,
		MalwareScanMode:             malwareSettings.ScanMode,
		AdminIPAllowlist:            append([]string(nil), s.cfg.AdminIPAllowlist...),
		CORSOrigins:                 append([]string(nil), s.cfg.CORSOrigins...),
		LogLevel:                    s.cfg.LogLevel,
		OIDCEnabled:                 oidcSettings.Enabled,
		OIDCIssuerURL:               oidcSettings.IssuerURL,
		OIDCClientID:                oidcSettings.ClientID,
		OIDCClientSecretConfigured:  oidcSettings.ClientSecretConfigured,
		OIDCRedirectURL:             oidcSettings.RedirectURL,
		OIDCScopes:                  append([]string(nil), oidcSettings.Scopes...),
		OIDCRoleClaim:               oidcSettings.RoleClaim,
		OIDCDefaultRole:             oidcSettings.DefaultRole,
		OIDCRoleMap:                 copyRoleMap(oidcSettings.RoleMap),
	}, nil
}

func (s *Service) ResolveDistributedEndpoints(ctx context.Context) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT endpoint
		FROM storage_nodes
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
	return append([]string(nil), s.cfg.StorageDistributedEndpoints...), nil
}

func (s *Service) ResolveMalwareSettings(ctx context.Context) (MalwareSettings, error) {
	if err := s.ensureMalwareSettings(ctx); err != nil {
		return MalwareSettings{}, err
	}
	var scanMode string
	if err := s.db.QueryRow(ctx, `
		SELECT scan_mode
		FROM cluster_malware_settings
		WHERE singleton = TRUE
	`).Scan(&scanMode); err != nil {
		return MalwareSettings{}, err
	}
	return MalwareSettings{ScanMode: normalizeMalwareScanMode(scanMode)}, nil
}

func (s *Service) UpdateMalwareSettings(ctx context.Context, input MalwareSettings) (MalwareSettings, error) {
	if err := s.ensureMalwareSettings(ctx); err != nil {
		return MalwareSettings{}, err
	}
	scanMode := normalizeMalwareScanMode(input.ScanMode)
	if err := validateMalwareScanMode(scanMode); err != nil {
		return MalwareSettings{}, err
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE cluster_malware_settings
		SET scan_mode = $1,
		    updated_at = NOW()
		WHERE singleton = TRUE
	`, scanMode); err != nil {
		return MalwareSettings{}, err
	}
	return s.ResolveMalwareSettings(ctx)
}

func (s *Service) ResolveOIDCSettings(ctx context.Context) (OIDCSettings, error) {
	if err := s.ensureOIDCSettings(ctx); err != nil {
		return OIDCSettings{}, err
	}
	var (
		enabled                bool
		issuerURL              string
		clientID               string
		clientSecretCiphertext string
		redirectURL            string
		rawScopes              []byte
		roleClaim              string
		defaultRole            string
		rawRoleMap             []byte
	)
	if err := s.db.QueryRow(ctx, `
		SELECT enabled, issuer_url, client_id, client_secret_ciphertext, redirect_url, scopes, role_claim, default_role, role_map
		FROM cluster_oidc_settings
		WHERE singleton = TRUE
	`).Scan(
		&enabled,
		&issuerURL,
		&clientID,
		&clientSecretCiphertext,
		&redirectURL,
		&rawScopes,
		&roleClaim,
		&defaultRole,
		&rawRoleMap,
	); err != nil {
		return OIDCSettings{}, err
	}
	var scopes []string
	if len(rawScopes) > 0 {
		_ = json.Unmarshal(rawScopes, &scopes)
	}
	var roleMap map[string]string
	if len(rawRoleMap) > 0 {
		_ = json.Unmarshal(rawRoleMap, &roleMap)
	}
	clientSecret := ""
	if strings.TrimSpace(clientSecretCiphertext) != "" {
		var err error
		clientSecret, err = s.sealer.OpenString(clientSecretCiphertext)
		if err != nil {
			return OIDCSettings{}, err
		}
	}
	return OIDCSettings{
		Enabled:                enabled,
		IssuerURL:              issuerURL,
		ClientID:               clientID,
		ClientSecret:           clientSecret,
		ClientSecretConfigured: strings.TrimSpace(clientSecretCiphertext) != "",
		RedirectURL:            redirectURL,
		Scopes:                 normalizeScopes(scopes),
		RoleClaim:              roleClaim,
		DefaultRole:            defaultRole,
		RoleMap:                copyRoleMap(roleMap),
	}, nil
}

func (s *Service) UpdateOIDCSettings(ctx context.Context, input OIDCSettingsInput) (OIDCSettings, error) {
	if err := s.ensureOIDCSettings(ctx); err != nil {
		return OIDCSettings{}, err
	}
	current, err := s.ResolveOIDCSettings(ctx)
	if err != nil {
		return OIDCSettings{}, err
	}
	next := OIDCSettings{
		Enabled:                input.Enabled,
		IssuerURL:              strings.TrimSpace(input.IssuerURL),
		ClientID:               strings.TrimSpace(input.ClientID),
		ClientSecret:           current.ClientSecret,
		ClientSecretConfigured: current.ClientSecretConfigured,
		RedirectURL:            strings.TrimSpace(input.RedirectURL),
		Scopes:                 normalizeScopes(input.Scopes),
		RoleClaim:              strings.TrimSpace(input.RoleClaim),
		DefaultRole:            strings.TrimSpace(input.DefaultRole),
		RoleMap:                normalizeRoleMap(input.RoleMap),
	}
	if strings.TrimSpace(input.ClientSecret) != "" {
		next.ClientSecret = strings.TrimSpace(input.ClientSecret)
		next.ClientSecretConfigured = true
	}
	if err := validateOIDCSettings(next); err != nil {
		return OIDCSettings{}, err
	}
	clientSecretCiphertext := ""
	if strings.TrimSpace(next.ClientSecret) != "" {
		clientSecretCiphertext, err = s.sealer.SealString(next.ClientSecret)
		if err != nil {
			return OIDCSettings{}, err
		}
	}
	rawScopes, err := json.Marshal(next.Scopes)
	if err != nil {
		return OIDCSettings{}, err
	}
	rawRoleMap, err := json.Marshal(next.RoleMap)
	if err != nil {
		return OIDCSettings{}, err
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE cluster_oidc_settings
		SET enabled = $1,
		    issuer_url = $2,
		    client_id = $3,
		    client_secret_ciphertext = $4,
		    redirect_url = $5,
		    scopes = $6::jsonb,
		    role_claim = $7,
		    default_role = $8,
		    role_map = $9::jsonb,
		    updated_at = NOW()
		WHERE singleton = TRUE
	`, next.Enabled, next.IssuerURL, next.ClientID, clientSecretCiphertext, next.RedirectURL, string(rawScopes), next.RoleClaim, next.DefaultRole, string(rawRoleMap)); err != nil {
		return OIDCSettings{}, err
	}
	return s.ResolveOIDCSettings(ctx)
}

func (s *Service) ClearOIDCClientSecret(ctx context.Context) (OIDCSettings, error) {
	if err := s.ensureOIDCSettings(ctx); err != nil {
		return OIDCSettings{}, err
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE cluster_oidc_settings
		SET client_secret_ciphertext = '',
		    updated_at = NOW()
		WHERE singleton = TRUE
	`); err != nil {
		return OIDCSettings{}, err
	}
	return s.ResolveOIDCSettings(ctx)
}

func (s *Service) DeploymentSetupStatus(ctx context.Context) (DeploymentSetup, error) {
	if err := s.ensureDeploymentSetup(ctx); err != nil {
		return DeploymentSetup{}, err
	}

	var completed bool
	var desiredStorageBackend string
	var distributedScope string
	var rawRemoteEndpoints []byte
	if err := s.db.QueryRow(ctx, `
		SELECT completed, desired_storage_backend, distributed_scope, remote_endpoints
		FROM deployment_setups
		WHERE singleton = TRUE
	`).Scan(&completed, &desiredStorageBackend, &distributedScope, &rawRemoteEndpoints); err != nil {
		return DeploymentSetup{}, err
	}

	var remoteEndpoints []string
	if len(rawRemoteEndpoints) > 0 {
		_ = json.Unmarshal(rawRemoteEndpoints, &remoteEndpoints)
	}
	status := DeploymentSetup{
		Completed:               completed,
		Required:                !completed,
		DesiredStorageBackend:   desiredStorageBackend,
		DistributedScope:        distributedScope,
		RemoteEndpoints:         remoteEndpoints,
		RuntimeStorageBackend:   s.cfg.StorageBackend,
		RuntimeEndpointCount:    len(s.cfg.StorageDistributedEndpoints),
		RecommendedLocalProfile: "docker compose --profile distributed up -d",
	}
	status.ApplyRequired = s.applyRequired(status)
	return status, nil
}

func (s *Service) CompleteDeploymentSetup(ctx context.Context, input DeploymentSetupInput) (DeploymentSetup, error) {
	if err := s.ensureDeploymentSetup(ctx); err != nil {
		return DeploymentSetup{}, err
	}

	mode := strings.ToLower(strings.TrimSpace(input.Mode))
	distributedMode := strings.ToLower(strings.TrimSpace(input.DistributedMode))
	remoteEndpoints := normalizeEndpoints(input.RemoteEndpoints)

	desiredStorageBackend := "local"
	switch mode {
	case "single-node":
		distributedMode = ""
		remoteEndpoints = nil
	case "distributed":
		desiredStorageBackend = "distributed"
		if distributedMode != "local" && distributedMode != "remote" {
			return DeploymentSetup{}, fmt.Errorf("distributed mode must be local or remote")
		}
		if distributedMode == "remote" && len(remoteEndpoints) == 0 {
			return DeploymentSetup{}, fmt.Errorf("remote distributed mode requires at least one endpoint")
		}
		if distributedMode == "remote" {
			if err := validateRemoteEndpoints(remoteEndpoints); err != nil {
				return DeploymentSetup{}, err
			}
		}
		if distributedMode == "local" {
			remoteEndpoints = nil
		}
	default:
		return DeploymentSetup{}, fmt.Errorf("mode must be single-node or distributed")
	}

	rawRemoteEndpoints, err := json.Marshal(remoteEndpoints)
	if err != nil {
		return DeploymentSetup{}, err
	}

	if _, err := s.db.Exec(ctx, `
		UPDATE deployment_setups
		SET completed = TRUE,
		    desired_storage_backend = $1,
		    distributed_scope = $2,
		    remote_endpoints = $3::jsonb,
		    updated_at = NOW()
		WHERE singleton = TRUE
	`, desiredStorageBackend, distributedMode, string(rawRemoteEndpoints)); err != nil {
		return DeploymentSetup{}, err
	}
	return s.DeploymentSetupStatus(ctx)
}

func (s *Service) ResolveStoragePolicy(ctx context.Context) (StoragePolicy, error) {
	if err := s.ensureStoragePolicy(ctx); err != nil {
		return StoragePolicy{}, err
	}
	var defaultClass string
	var standardReplicas int
	var reducedReplicas int
	var archiveReplicas int
	if err := s.db.QueryRow(ctx, `
		SELECT default_storage_class, standard_replicas, reduced_redundancy_replicas, archive_ready_replicas
		FROM cluster_storage_policies
		WHERE singleton = TRUE
	`).Scan(&defaultClass, &standardReplicas, &reducedReplicas, &archiveReplicas); err != nil {
		return StoragePolicy{}, err
	}
	return StoragePolicy{
		DefaultClass: defaultClass,
		Policies: []config.StorageClassPolicy{
			{Name: "standard", Label: "Standard", Description: "Balanced default durability class.", DefaultReplicas: standardReplicas},
			{Name: "reduced-redundancy", Label: "Reduced Redundancy", Description: "Lower replica count for less critical data.", DefaultReplicas: reducedReplicas},
			{Name: "archive-ready", Label: "Archive Ready", Description: "Single replica staging class for archive-oriented or reproducible data.", DefaultReplicas: archiveReplicas},
		},
	}, nil
}

func (s *Service) UpdateStoragePolicy(ctx context.Context, defaultClass string, replicas map[string]int) (StoragePolicy, error) {
	if err := s.ensureStoragePolicy(ctx); err != nil {
		return StoragePolicy{}, err
	}
	defaultClass = config.NormalizeStorageClass(defaultClass)
	if !config.IsValidStorageClass(defaultClass) {
		return StoragePolicy{}, fmt.Errorf("default storage class must be one of %s", joinClasses())
	}
	standardReplicas := replicas["standard"]
	reducedReplicas := replicas["reduced-redundancy"]
	archiveReplicas := replicas["archive-ready"]
	if err := s.validateReplicaSetting(ctx, standardReplicas); err != nil {
		return StoragePolicy{}, fmt.Errorf("standard replicas: %w", err)
	}
	if err := s.validateReplicaSetting(ctx, reducedReplicas); err != nil {
		return StoragePolicy{}, fmt.Errorf("reduced redundancy replicas: %w", err)
	}
	if err := s.validateReplicaSetting(ctx, archiveReplicas); err != nil {
		return StoragePolicy{}, fmt.Errorf("archive ready replicas: %w", err)
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE cluster_storage_policies
		SET default_storage_class = $1,
		    standard_replicas = $2,
		    reduced_redundancy_replicas = $3,
		    archive_ready_replicas = $4,
		    updated_at = NOW()
		WHERE singleton = TRUE
	`, defaultClass, standardReplicas, reducedReplicas, archiveReplicas); err != nil {
		return StoragePolicy{}, err
	}
	return s.ResolveStoragePolicy(ctx)
}

func (s *Service) ensureStoragePolicy(ctx context.Context) error {
	standardReplicas := config.EffectiveReplicaTarget("standard", s.cfg.StorageDistributedReplicas)
	reducedReplicas := config.EffectiveReplicaTarget("reduced-redundancy", s.cfg.StorageDistributedReplicas)
	archiveReplicas := config.EffectiveReplicaTarget("archive-ready", s.cfg.StorageDistributedReplicas)
	_, err := s.db.Exec(ctx, `
		INSERT INTO cluster_storage_policies (singleton, default_storage_class, standard_replicas, reduced_redundancy_replicas, archive_ready_replicas)
		VALUES (TRUE, $1, $2, $3, $4)
		ON CONFLICT (singleton) DO NOTHING
	`, s.cfg.StorageDefaultClass, standardReplicas, reducedReplicas, archiveReplicas)
	return err
}

func (s *Service) ensureOIDCSettings(ctx context.Context) error {
	rawScopes, err := json.Marshal(normalizeScopes(s.cfg.OIDCScopes))
	if err != nil {
		return err
	}
	rawRoleMap, err := json.Marshal(copyRoleMap(s.cfg.OIDCRoleMap))
	if err != nil {
		return err
	}
	clientSecretCiphertext := ""
	if strings.TrimSpace(s.cfg.OIDCClientSecret) != "" {
		clientSecretCiphertext, err = s.sealer.SealString(s.cfg.OIDCClientSecret)
		if err != nil {
			return err
		}
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO cluster_oidc_settings (
			singleton, enabled, issuer_url, client_id, client_secret_ciphertext, redirect_url, scopes, role_claim, default_role, role_map
		)
		VALUES (TRUE, $1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9::jsonb)
		ON CONFLICT (singleton) DO NOTHING
	`, s.cfg.OIDCEnabled, s.cfg.OIDCIssuerURL, s.cfg.OIDCClientID, clientSecretCiphertext, s.cfg.OIDCRedirectURL, string(rawScopes), s.cfg.OIDCRoleClaim, defaultOIDCRole(s.cfg.OIDCDefaultRole), string(rawRoleMap))
	return err
}

func (s *Service) ensureMalwareSettings(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO cluster_malware_settings (singleton, scan_mode)
		VALUES (TRUE, $1)
		ON CONFLICT (singleton) DO NOTHING
	`, normalizeMalwareScanMode(s.cfg.MalwareScanMode))
	return err
}

func (s *Service) ensureDeploymentSetup(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO deployment_setups (singleton, completed, desired_storage_backend, distributed_scope, remote_endpoints)
		VALUES (TRUE, FALSE, $1, '', '[]'::jsonb)
		ON CONFLICT (singleton) DO NOTHING
	`, s.cfg.StorageBackend)
	return err
}

func (s *Service) validateReplicaSetting(ctx context.Context, value int) error {
	if value <= 0 {
		return fmt.Errorf("must be greater than zero")
	}
	if s.cfg.StorageBackend == "distributed" {
		configuredEndpointCount := len(s.cfg.StorageDistributedEndpoints)
		var registeredEndpointCount int
		if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM storage_nodes`).Scan(&registeredEndpointCount); err == nil && registeredEndpointCount > configuredEndpointCount {
			configuredEndpointCount = registeredEndpointCount
		}
		if configuredEndpointCount > 0 && value > configuredEndpointCount {
			return fmt.Errorf("cannot exceed registered storage endpoints")
		}
	}
	return nil
}

func joinClasses() string {
	classes := config.SupportedStorageClasses()
	return strings.Join(classes, ", ")
}

func copyRoleMap(value map[string]string) map[string]string {
	if len(value) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func normalizeEndpoints(values []string) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		value = strings.TrimRight(value, "/")
		if value != "" {
			items = append(items, value)
		}
	}
	if items == nil {
		return []string{}
	}
	return items
}

func validateRemoteEndpoints(values []string) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		parsed, err := url.Parse(value)
		if err != nil {
			return fmt.Errorf("remote endpoint %q is not a valid URL", value)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("remote endpoint %q must start with http:// or https://", value)
		}
		if strings.TrimSpace(parsed.Host) == "" {
			return fmt.Errorf("remote endpoint %q must include a host", value)
		}
		if parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("remote endpoint %q cannot include a query string or fragment", value)
		}
		normalized := strings.TrimRight(parsed.String(), "/")
		if _, ok := seen[normalized]; ok {
			return fmt.Errorf("remote endpoint %q is duplicated", value)
		}
		seen[normalized] = struct{}{}
	}
	return nil
}

func normalizeScopes(values []string) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			items = append(items, value)
		}
	}
	if len(items) == 0 {
		return []string{"openid", "email", "profile"}
	}
	return items
}

func normalizeRoleMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	items := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		items[key] = value
	}
	return items
}

func normalizeMalwareScanMode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "advisory"
	}
	return value
}

func validateMalwareScanMode(value string) error {
	if value != "advisory" && value != "enforcement" {
		return fmt.Errorf("malware scan mode must be advisory or enforcement")
	}
	return nil
}

func defaultOIDCRole(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "admin"
	}
	return value
}

func validateOIDCSettings(value OIDCSettings) error {
	if !value.Enabled {
		return nil
	}
	if value.IssuerURL == "" || value.ClientID == "" || value.RedirectURL == "" {
		return fmt.Errorf("issuer URL, client ID, and redirect URL are required when OIDC is enabled")
	}
	if strings.TrimSpace(value.ClientSecret) == "" {
		return fmt.Errorf("client secret is required when OIDC is enabled")
	}
	if !config.IsValidPlatformRole(defaultOIDCRole(value.DefaultRole)) {
		return fmt.Errorf("default role must be one of %s", strings.Join(config.SortedPlatformRoles(), ", "))
	}
	for sourceValue, role := range value.RoleMap {
		if !config.IsValidPlatformRole(role) {
			return fmt.Errorf("role mapping %q targets invalid role %q", sourceValue, role)
		}
	}
	return nil
}

func (s *Service) applyRequired(status DeploymentSetup) bool {
	if !status.Completed {
		return false
	}
	if status.DesiredStorageBackend != status.RuntimeStorageBackend {
		return true
	}
	if status.DesiredStorageBackend != "distributed" {
		return false
	}
	if status.DistributedScope == "remote" {
		return !sameStringSlices(status.RemoteEndpoints, s.cfg.StorageDistributedEndpoints)
	}
	return false
}

func sameStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
