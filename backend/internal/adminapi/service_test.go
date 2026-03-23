package adminapi

import (
	"strings"
	"testing"
)

func TestRandomTokenPrefix(t *testing.T) {
	token, err := randomToken()
	if err != nil {
		t.Fatalf("randomToken: %v", err)
	}
	if !strings.HasPrefix(token, "hsat_") {
		t.Fatalf("expected hsat_ prefix, got %s", token)
	}
	if len(token) <= len("hsat_") {
		t.Fatalf("expected token payload, got %s", token)
	}
}
