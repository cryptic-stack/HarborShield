package policies

import "testing"

func TestEvaluateDenyOverridesAllow(t *testing.T) {
	statements := []Statement{
		{Subject: "user:123", Action: "object.get", Resource: "*", Effect: "allow"},
		{Subject: "user:123", Action: "object.get", Resource: "bucket/secret", Effect: "deny"},
	}
	if !Evaluate(statements, "user:123", "object.get", "*") {
		t.Fatal("expected wildcard allow")
	}
	if Evaluate(statements, "user:123", "object.get", "bucket/secret") {
		t.Fatal("expected explicit deny")
	}
}
