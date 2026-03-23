package authz

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type stubPolicyService struct {
	allowed bool
	err     error
	role    string
}

func (s stubPolicyService) AuthorizeRole(_ context.Context, _, _, _ string) (bool, error) {
	return s.allowed, s.err
}

func (s stubPolicyService) AuthorizeSubject(_ context.Context, _, _, fallbackRole, _, _ string) (bool, string, error) {
	role := fallbackRole
	if s.role != "" {
		role = s.role
	}
	return s.allowed, role, s.err
}

func TestCheckRoleReturnsForbiddenWhenDenied(t *testing.T) {
	authorizer := New(stubPolicyService{allowed: false})
	err := authorizer.CheckRole(context.Background(), "readonly", "bucket.create", "*")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestErrForbiddenIsComparable(t *testing.T) {
	if !errors.Is(ErrForbidden, ErrForbidden) {
		t.Fatal("expected ErrForbidden to be comparable")
	}
}

func TestCheckRolePropagatesPolicyError(t *testing.T) {
	authorizer := New(stubPolicyService{err: errors.New("db offline")})
	err := authorizer.CheckRole(context.Background(), "admin", "bucket.list", "*")
	if err == nil || !strings.Contains(err.Error(), "db offline") {
		t.Fatalf("expected wrapped policy error, got %v", err)
	}
}

func TestCheckSubjectReturnsResolvedRole(t *testing.T) {
	authorizer := New(stubPolicyService{allowed: true, role: "bucket-admin"})
	role, err := authorizer.CheckSubject(context.Background(), "credential", "AK123", "readonly", "object.put", "*")
	if err != nil {
		t.Fatalf("expected allow, got %v", err)
	}
	if role != "bucket-admin" {
		t.Fatalf("expected resolved role bucket-admin, got %s", role)
	}
}
