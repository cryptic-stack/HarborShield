package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"harborshield/backend/internal/auth"
)

type stubAdminTokens struct {
	claims *auth.Claims
	err    error
}

func (s stubAdminTokens) Authenticate(_ context.Context, _ string) (*auth.Claims, error) {
	return s.claims, s.err
}

func TestAdminAuthAcceptsAdminTokenFallback(t *testing.T) {
	handler := AdminAuth(auth.TokenManager{}, stubAdminTokens{
		claims: &auth.Claims{UserID: "user-1", Role: "admin", Email: "admin@example.com"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromContext(r.Context())
		if claims == nil || claims.Role != "admin" {
			t.Fatalf("expected admin claims, got %#v", claims)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/roles", nil)
	req.Header.Set("Authorization", "Bearer hsat_example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestAdminAuthRejectsUnknownBearer(t *testing.T) {
	handler := AdminAuth(auth.TokenManager{}, stubAdminTokens{err: errors.New("invalid")})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/roles", nil)
	req.Header.Set("Authorization", "Bearer invalid")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestAdminIPAllowlistAllowsConfiguredIP(t *testing.T) {
	handler := AdminIPAllowlist([]string{"127.0.0.1", "10.0.0.0/8"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/roles", nil)
	req.RemoteAddr = "127.0.0.1:5000"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestAdminIPAllowlistRejectsUnknownIP(t *testing.T) {
	handler := AdminIPAllowlist([]string{"10.0.0.0/8"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/roles", nil)
	req.RemoteAddr = "192.168.1.25:5000"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
