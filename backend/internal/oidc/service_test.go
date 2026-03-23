package oidc

import (
	"context"
	"strings"
	"testing"

	"harborshield/backend/internal/config"
)

func TestStateTokenRoundTrip(t *testing.T) {
	service := New(config.Config{JWTSecret: "test-secret", OIDCScopes: []string{"openid"}}, nil, nil)

	token, nonce, err := service.newStateToken()
	if err != nil {
		t.Fatalf("newStateToken failed: %v", err)
	}
	claims, err := service.parseStateToken(token)
	if err != nil {
		t.Fatalf("parseStateToken failed: %v", err)
	}
	if claims.Nonce != nonce {
		t.Fatalf("expected nonce %q, got %q", nonce, claims.Nonce)
	}
}

func TestRenderSessionBootstrapContainsSessionStorageWrite(t *testing.T) {
	page, err := renderSessionBootstrap([]byte(`{"accessToken":"abc","user":{"id":"1","email":"a@example.com","role":"admin"}}`))
	if err != nil {
		t.Fatalf("renderSessionBootstrap failed: %v", err)
	}
	if !strings.Contains(page, `localStorage.setItem("harborshield-session"`) {
		t.Fatal("expected callback page to store harborshield session")
	}
	if !strings.Contains(page, `window.location.replace("/")`) {
		t.Fatal("expected callback page to redirect to root")
	}
}

func TestResolveRoleUsesMappedStringClaim(t *testing.T) {
	service := New(config.Config{
		OIDCDefaultRole: "readonly",
		OIDCRoleClaim:   "role",
		OIDCRoleMap: map[string]string{
			"storage-admin": "admin",
		},
	}, nil, nil)

	role := service.resolveRole(service.currentSettings(context.Background()), map[string]any{"role": "storage-admin"})
	if role != "admin" {
		t.Fatalf("expected mapped admin role, got %q", role)
	}
}

func TestResolveRoleUsesMappedArrayClaim(t *testing.T) {
	service := New(config.Config{
		OIDCDefaultRole: "readonly",
		OIDCRoleClaim:   "groups",
		OIDCRoleMap: map[string]string{
			"bucket-auditors": "auditor",
		},
	}, nil, nil)

	role := service.resolveRole(service.currentSettings(context.Background()), map[string]any{"groups": []any{"team-one", "bucket-auditors"}})
	if role != "auditor" {
		t.Fatalf("expected mapped auditor role, got %q", role)
	}
}

func TestResolveRoleFallsBackToDefaultRole(t *testing.T) {
	service := New(config.Config{
		OIDCDefaultRole: "readonly",
		OIDCRoleClaim:   "groups",
		OIDCRoleMap: map[string]string{
			"storage-admin": "admin",
		},
	}, nil, nil)

	role := service.resolveRole(service.currentSettings(context.Background()), map[string]any{"groups": []any{"team-one"}})
	if role != "readonly" {
		t.Fatalf("expected default readonly role, got %q", role)
	}
}

func TestFlattenRoleMappings(t *testing.T) {
	got := flattenRoleMappings(map[string]string{
		"storage-admin": "admin",
		"auditors":      "auditor",
	})
	if len(got) != 2 {
		t.Fatalf("expected 2 mappings, got %#v", got)
	}
	if !contains(got, "storage-admin => admin") {
		t.Fatalf("expected storage-admin mapping, got %#v", got)
	}
}

func contains(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}
