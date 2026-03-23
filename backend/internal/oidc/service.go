package oidc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"sync"
	"time"

	coreoidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"

	"harborshield/backend/internal/auth"
	"harborshield/backend/internal/config"
	"harborshield/backend/internal/settings"
)

type Service struct {
	cfg             config.Config
	auth            *auth.Service
	settingsService *settings.Service
	httpClient      *http.Client

	mu        sync.Mutex
	provider  *coreoidc.Provider
	verifier  *coreoidc.IDTokenVerifier
	oauth     *oauth2.Config
	cachedKey string
	active    settings.OIDCSettings
}

type Status struct {
	Enabled         bool     `json:"enabled"`
	Configured      bool     `json:"configured"`
	LoginReady      bool     `json:"loginReady"`
	IssuerURL       string   `json:"issuerUrl"`
	ClientID        string   `json:"clientId"`
	RedirectURL     string   `json:"redirectUrl"`
	Scopes          []string `json:"scopes"`
	RoleClaim       string   `json:"roleClaim"`
	DefaultRole     string   `json:"defaultRole"`
	RoleMappings    []string `json:"roleMappings"`
	StatusMessage   string   `json:"statusMessage"`
	NextStepMessage string   `json:"nextStepMessage"`
}

type TestResult struct {
	IssuerURL             string   `json:"issuerUrl"`
	AuthorizationEndpoint string   `json:"authorizationEndpoint"`
	TokenEndpoint         string   `json:"tokenEndpoint"`
	UserInfoEndpoint      string   `json:"userInfoEndpoint"`
	JWKSURL               string   `json:"jwksUrl"`
	Scopes                []string `json:"scopes"`
	Message               string   `json:"message"`
}

type callbackStateClaims struct {
	Nonce string `json:"nonce"`
	jwt.RegisteredClaims
}

type idTokenClaims struct {
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Nonce         string `json:"nonce"`
}

func New(cfg config.Config, authSvc *auth.Service, settingsSvc *settings.Service) *Service {
	return &Service{
		cfg:             cfg,
		auth:            authSvc,
		settingsService: settingsSvc,
		httpClient:      &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *Service) Status(ctx context.Context) Status {
	current := s.currentSettings(ctx)
	configured := current.Enabled && current.IssuerURL != "" && current.ClientID != "" && current.RedirectURL != "" && current.ClientSecret != ""
	loginReady := configured
	statusMessage := "OIDC is disabled"
	nextStepMessage := "Configure OIDC settings in the admin console to enable provider-backed login."
	if configured {
		statusMessage = "OIDC is configured and ready for authorization-code login"
		nextStepMessage = "Use the login page button to start OIDC authentication. New OIDC users will receive the configured mapped or default HarborShield role."
	} else if current.Enabled {
		statusMessage = "OIDC is enabled but incomplete"
		nextStepMessage = "Provide issuer, client ID, redirect URL, and client secret. Optional role claim mapping can be configured too."
	}

	return Status{
		Enabled:         current.Enabled,
		Configured:      configured,
		LoginReady:      loginReady,
		IssuerURL:       current.IssuerURL,
		ClientID:        current.ClientID,
		RedirectURL:     current.RedirectURL,
		Scopes:          append([]string(nil), current.Scopes...),
		RoleClaim:       current.RoleClaim,
		DefaultRole:     current.DefaultRole,
		RoleMappings:    flattenRoleMappings(current.RoleMap),
		StatusMessage:   statusMessage,
		NextStepMessage: nextStepMessage,
	}
}

func (s *Service) Start(ctx context.Context) (string, error) {
	if !s.Status(ctx).Configured {
		return "", fmt.Errorf("oidc is not configured")
	}
	if _, err := s.ensureProvider(ctx); err != nil {
		return "", err
	}
	stateToken, nonce, err := s.newStateToken()
	if err != nil {
		return "", err
	}
	return s.oauth.AuthCodeURL(
		stateToken,
		oauth2.AccessTypeOffline,
		coreoidc.Nonce(nonce),
	), nil
}

func (s *Service) Callback(ctx context.Context, code, state string) (string, error) {
	if code == "" || state == "" {
		return "", fmt.Errorf("missing oidc code or state")
	}
	current, err := s.ensureProvider(ctx)
	if err != nil {
		return "", err
	}
	stateClaims, err := s.parseStateToken(state)
	if err != nil {
		return "", err
	}
	token, err := s.oauth.Exchange(ctx, code)
	if err != nil {
		return "", err
	}
	rawIDToken, _ := token.Extra("id_token").(string)
	if rawIDToken == "" {
		return "", fmt.Errorf("provider did not return id_token")
	}
	idToken, err := s.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return "", err
	}

	var claims idTokenClaims
	if err := idToken.Claims(&claims); err != nil {
		return "", err
	}
	var rawClaims map[string]any
	if err := idToken.Claims(&rawClaims); err != nil {
		return "", err
	}
	if claims.Nonce != stateClaims.Nonce {
		return "", fmt.Errorf("oidc nonce mismatch")
	}
	if claims.Subject == "" || claims.Email == "" {
		return "", fmt.Errorf("provider did not return required subject or email claims")
	}

	session, err := s.auth.LoginOIDC(ctx, current.IssuerURL, claims.Subject, claims.Email, claims.EmailVerified, s.resolveRole(current, rawClaims))
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(session)
	if err != nil {
		return "", err
	}
	return renderSessionBootstrap(payload)
}

func (s *Service) TestConnection(ctx context.Context) (TestResult, error) {
	current := s.currentSettings(ctx)
	if current.IssuerURL == "" || current.ClientID == "" || current.RedirectURL == "" {
		return TestResult{}, fmt.Errorf("issuer URL, client ID, and redirect URL are required before testing OIDC")
	}
	if strings.TrimSpace(current.ClientSecret) == "" {
		return TestResult{}, fmt.Errorf("client secret is required before testing OIDC")
	}
	providerCtx := coreoidc.ClientContext(ctx, s.httpClient)
	provider, err := coreoidc.NewProvider(providerCtx, current.IssuerURL)
	if err != nil {
		return TestResult{}, err
	}
	var metadata struct {
		IssuerURL             string `json:"issuer"`
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
		UserInfoEndpoint      string `json:"userinfo_endpoint"`
		JWKSURL               string `json:"jwks_uri"`
	}
	if err := provider.Claims(&metadata); err != nil {
		return TestResult{}, err
	}
	return TestResult{
		IssuerURL:             coalesce(metadata.IssuerURL, current.IssuerURL),
		AuthorizationEndpoint: metadata.AuthorizationEndpoint,
		TokenEndpoint:         metadata.TokenEndpoint,
		UserInfoEndpoint:      metadata.UserInfoEndpoint,
		JWKSURL:               metadata.JWKSURL,
		Scopes:                append([]string(nil), current.Scopes...),
		Message:               "OIDC discovery succeeded",
	}, nil
}

func (s *Service) resolveRole(current settings.OIDCSettings, claims map[string]any) string {
	defaultRole := current.DefaultRole
	if defaultRole == "" {
		defaultRole = "admin"
	}
	if current.RoleClaim == "" || len(current.RoleMap) == 0 {
		return defaultRole
	}
	value, ok := claims[current.RoleClaim]
	if !ok {
		return defaultRole
	}
	for _, candidate := range claimValues(value) {
		if mappedRole, exists := current.RoleMap[candidate]; exists && mappedRole != "" {
			return mappedRole
		}
	}
	return defaultRole
}

func claimValues(value any) []string {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{typed}
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if ok && strings.TrimSpace(text) != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func flattenRoleMappings(value map[string]string) []string {
	if len(value) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(value))
	for key, role := range value {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(role) == "" {
			continue
		}
		out = append(out, fmt.Sprintf("%s => %s", key, role))
	}
	return out
}

func (s *Service) ensureProvider(ctx context.Context) (settings.OIDCSettings, error) {
	current := s.currentSettings(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	cacheKey := fmt.Sprintf(
		"%t|%s|%s|%s|%s|%s|%s|%s|%s",
		current.Enabled,
		current.IssuerURL,
		current.ClientID,
		current.ClientSecret,
		current.RedirectURL,
		strings.Join(current.Scopes, ","),
		current.RoleClaim,
		current.DefaultRole,
		strings.Join(flattenRoleMappings(current.RoleMap), ","),
	)
	if s.provider != nil && s.oauth != nil && s.verifier != nil && s.cachedKey == cacheKey {
		return current, nil
	}
	providerCtx := coreoidc.ClientContext(ctx, s.httpClient)
	provider, err := coreoidc.NewProvider(providerCtx, current.IssuerURL)
	if err != nil {
		return settings.OIDCSettings{}, err
	}
	oauthCfg := &oauth2.Config{
		ClientID:     current.ClientID,
		ClientSecret: current.ClientSecret,
		RedirectURL:  current.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       append([]string(nil), current.Scopes...),
	}
	verifier := provider.Verifier(&coreoidc.Config{ClientID: current.ClientID})
	s.provider = provider
	s.oauth = oauthCfg
	s.verifier = verifier
	s.cachedKey = cacheKey
	s.active = current
	return current, nil
}

func (s *Service) currentSettings(ctx context.Context) settings.OIDCSettings {
	if s.settingsService != nil {
		current, err := s.settingsService.ResolveOIDCSettings(ctx)
		if err == nil {
			return current
		}
	}
	return settings.OIDCSettings{
		Enabled:                s.cfg.OIDCEnabled,
		IssuerURL:              s.cfg.OIDCIssuerURL,
		ClientID:               s.cfg.OIDCClientID,
		ClientSecret:           s.cfg.OIDCClientSecret,
		ClientSecretConfigured: s.cfg.OIDCClientSecret != "",
		RedirectURL:            s.cfg.OIDCRedirectURL,
		Scopes:                 append([]string(nil), s.cfg.OIDCScopes...),
		RoleClaim:              s.cfg.OIDCRoleClaim,
		DefaultRole:            s.cfg.OIDCDefaultRole,
		RoleMap:                copyRoleMap(s.cfg.OIDCRoleMap),
	}
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

func coalesce(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func (s *Service) newStateToken() (string, string, error) {
	nonce, err := randomURLSafe(24)
	if err != nil {
		return "", "", err
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, callbackStateClaims{
		Nonce: nonce,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(10 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		},
	})
	signed, err := token.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return "", "", err
	}
	return signed, nonce, nil
}

func (s *Service) parseStateToken(value string) (*callbackStateClaims, error) {
	token, err := jwt.ParseWithClaims(value, &callbackStateClaims{}, func(token *jwt.Token) (any, error) {
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*callbackStateClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid oidc state")
	}
	return claims, nil
}

func randomURLSafe(byteCount int) (string, error) {
	buf := make([]byte, byteCount)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return strings.TrimRight(base64.RawURLEncoding.EncodeToString(buf), "="), nil
}

func renderSessionBootstrap(payload []byte) (string, error) {
	escaped, err := json.Marshal(string(payload))
	if err != nil {
		return "", err
	}
	tpl := template.Must(template.New("oidc-callback").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>HarborShield OIDC Login</title>
</head>
<body style="font-family: Segoe UI, sans-serif; background:#020617; color:#e2e8f0; display:flex; align-items:center; justify-content:center; min-height:100vh;">
  <div style="max-width:520px; padding:32px; border:1px solid rgba(148,163,184,.3); border-radius:24px; background:rgba(15,23,42,.85);">
    <h1 style="margin:0 0 12px;">Signing you in</h1>
    <p style="margin:0; color:#cbd5e1;">HarborShield is finalizing your OIDC session and returning you to the admin console.</p>
  </div>
  <script>
    const raw = {{ . }};
    const session = JSON.parse(raw);
    localStorage.setItem("harborshield-session", JSON.stringify(session));
    window.location.replace("/");
  </script>
</body>
</html>`))
	var out strings.Builder
	if err := tpl.Execute(&out, string(escaped)); err != nil {
		return "", err
	}
	return out.String(), nil
}
