package audit

import "testing"

func TestSanitizeAuditValueRedactsSensitiveKeys(t *testing.T) {
	payload := map[string]any{
		"clientSecret":           "super-secret",
		"accessToken":            "abc",
		"refreshToken":           "def",
		"authorization":          "Bearer xyz",
		"cookie":                 "session=123",
		"sessionId":              "session-1",
		"clientSecretConfigured": true,
		"accessKey":              "AKIA_TEST",
		"nested": map[string]any{
			"signingSecret": "hook-secret",
			"passwordHash":  "hash",
		},
	}

	sanitized, ok := sanitizeAuditValue(payload).(map[string]any)
	if !ok {
		t.Fatalf("expected sanitized payload map")
	}

	for _, key := range []string{"clientSecret", "accessToken", "refreshToken", "authorization", "cookie", "sessionId"} {
		if sanitized[key] != "[redacted]" {
			t.Fatalf("expected %s to be redacted, got %#v", key, sanitized[key])
		}
	}
	if sanitized["clientSecretConfigured"] != true {
		t.Fatalf("expected configured boolean to remain visible")
	}
	if sanitized["accessKey"] != "AKIA_TEST" {
		t.Fatalf("expected accessKey to remain visible")
	}

	nested, ok := sanitized["nested"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested map")
	}
	if nested["signingSecret"] != "[redacted]" || nested["passwordHash"] != "[redacted]" {
		t.Fatalf("expected nested sensitive keys to be redacted, got %#v", nested)
	}
}

func TestIsSensitiveAuditKeyHonorsConfiguredFlags(t *testing.T) {
	cases := []struct {
		key       string
		sensitive bool
	}{
		{key: "clientSecret", sensitive: true},
		{key: "clientSecretConfigured", sensitive: false},
		{key: "authorization", sensitive: true},
		{key: "sessionCookie", sensitive: true},
		{key: "accessKey", sensitive: false},
	}

	for _, tc := range cases {
		if got := isSensitiveAuditKey(tc.key); got != tc.sensitive {
			t.Fatalf("key %q sensitive=%v, got %v", tc.key, tc.sensitive, got)
		}
	}
}
