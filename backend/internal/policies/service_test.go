package policies

import (
	"errors"
	"testing"
)

func TestEvaluateHonorsDenyBeforeAllow(t *testing.T) {
	statements := []Statement{
		{Subject: "admin", Action: "bucket.*", Resource: "*", Effect: "allow"},
		{Subject: "admin", Action: "bucket.delete", Resource: "bucket:locked", Effect: "deny"},
	}

	if allowed := Evaluate(statements, "admin", "bucket.create", "bucket:demo"); !allowed {
		t.Fatal("expected create bucket to be allowed")
	}
	if allowed := Evaluate(statements, "admin", "bucket.delete", "bucket:locked"); allowed {
		t.Fatal("expected locked bucket delete to be denied")
	}
}

func TestMatchSupportsSuffixWildcards(t *testing.T) {
	if !match("bucket.*", "bucket.create") {
		t.Fatal("expected wildcard match to succeed")
	}
	if !match("bucket:*", "bucket:demo") {
		t.Fatal("expected resource wildcard match to succeed")
	}
	if match("object.get", "object.delete") {
		t.Fatal("expected different action to fail")
	}
}

func TestEvaluateDefaultsToDeny(t *testing.T) {
	statements := []Statement{
		{Subject: "readonly", Action: "object.get", Resource: "*", Effect: "allow"},
	}
	if allowed := Evaluate(statements, "readonly", "bucket.create", "*"); allowed {
		t.Fatal("expected unmatched action to be denied by default")
	}
}

func TestValidateActionResourceEffectRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name     string
		action   string
		resource string
		effect   string
		wantErr  error
	}{
		{name: "missing action", action: "", resource: "*", effect: "allow", wantErr: ErrInvalidAction},
		{name: "action with spaces", action: "bucket create", resource: "*", effect: "allow", wantErr: ErrInvalidAction},
		{name: "missing resource", action: "bucket.create", resource: "", effect: "allow", wantErr: ErrInvalidResource},
		{name: "invalid effect", action: "bucket.create", resource: "*", effect: "maybe", wantErr: ErrInvalidEffect},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateActionResourceEffect(tc.action, tc.resource, tc.effect)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidateBindingFields(t *testing.T) {
	if err := validateBindingFields("credential", "AKIA123", "bucket:demo"); err != nil {
		t.Fatalf("expected valid binding fields, got %v", err)
	}
	if err := validateBindingFields("credential", "", "bucket:demo"); !errors.Is(err, ErrInvalidSubjectID) {
		t.Fatalf("expected ErrInvalidSubjectID, got %v", err)
	}
	if err := validateBindingFields("session", "subject-1", "bucket:demo"); !errors.Is(err, ErrInvalidSubjectType) {
		t.Fatalf("expected ErrInvalidSubjectType, got %v", err)
	}
	if err := validateBindingFields("credential", "AKIA123", "bucket demo"); !errors.Is(err, ErrInvalidResource) {
		t.Fatalf("expected ErrInvalidResource, got %v", err)
	}
}

func TestBindingMatchesResource(t *testing.T) {
	if !bindingMatchesResource("*", "bucket:demo") {
		t.Fatal("expected wildcard resource binding to match")
	}
	if !bindingMatchesResource("bucket:demo", "bucket:demo") {
		t.Fatal("expected exact binding resource match")
	}
	if !bindingMatchesResource("bucket:demo*", "bucket:demo/object.txt") {
		t.Fatal("expected prefix binding resource match")
	}
	if bindingMatchesResource("bucket:other*", "bucket:demo/object.txt") {
		t.Fatal("expected non-matching resource binding to fail")
	}
}

func TestEvaluateRoleStatements(t *testing.T) {
	statements := []StatementRecord{
		{ID: "1", Action: "object.*", Resource: "bucket:demo*", Effect: "allow"},
		{ID: "2", Action: "object.delete", Resource: "bucket:demo/object:locked", Effect: "deny"},
		{ID: "3", Action: "bucket.list", Resource: "*", Effect: "allow"},
	}

	allowed, explicitDeny, matched := evaluateRoleStatements(statements, "object.put", "bucket:demo/object:file.txt")
	if !allowed || explicitDeny {
		t.Fatalf("expected allow without explicit deny, got allowed=%v explicitDeny=%v", allowed, explicitDeny)
	}
	if len(matched) != 1 || matched[0].ID != "1" {
		t.Fatalf("expected only statement 1 to match, got %+v", matched)
	}

	allowed, explicitDeny, matched = evaluateRoleStatements(statements, "object.delete", "bucket:demo/object:locked")
	if allowed || !explicitDeny {
		t.Fatalf("expected explicit deny, got allowed=%v explicitDeny=%v", allowed, explicitDeny)
	}
	if len(matched) != 2 {
		t.Fatalf("expected two matched statements for deny case, got %d", len(matched))
	}
}
