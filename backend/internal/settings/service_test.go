package settings

import "testing"

func TestValidateRemoteEndpointsAcceptsHTTPAndHTTPS(t *testing.T) {
	err := validateRemoteEndpoints([]string{
		"https://node-a.example.internal:9100",
		"http://10.0.0.25:9100",
	})
	if err != nil {
		t.Fatalf("expected endpoints to validate, got %v", err)
	}
}

func TestValidateRemoteEndpointsRejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name   string
		values []string
	}{
		{
			name:   "missing scheme",
			values: []string{"node-a.example.internal:9100"},
		},
		{
			name:   "unsupported scheme",
			values: []string{"ftp://node-a.example.internal:9100"},
		},
		{
			name:   "duplicate",
			values: []string{"https://node-a.example.internal:9100", "https://node-a.example.internal:9100/"},
		},
		{
			name:   "query string",
			values: []string{"https://node-a.example.internal:9100?token=bad"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateRemoteEndpoints(tc.values); err == nil {
				t.Fatalf("expected validation error for %v", tc.values)
			}
		})
	}
}
