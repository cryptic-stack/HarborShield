package s3

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCanonicalRequestBuildsDeterministicSignedHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://localhost/s3/demo-bucket/test.txt?prefix=a&list-type=2", nil)
	req.Header.Set("X-Amz-Date", "20260320T020000Z")
	req.Header.Set("X-Amz-Content-Sha256", "UNSIGNED-PAYLOAD")

	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonical, err := canonicalRequest(req, "UNSIGNED-PAYLOAD", signedHeaders)
	if err != nil {
		t.Fatalf("canonical request: %v", err)
	}
	if !strings.Contains(canonical, "/s3/demo-bucket/test.txt") {
		t.Fatalf("unexpected canonical uri: %s", canonical)
	}
	if !strings.Contains(canonical, "list-type=2&prefix=a") {
		t.Fatalf("unexpected canonical query: %s", canonical)
	}
}

func TestDeriveSigningKeyMatchesManualHMACChain(t *testing.T) {
	secret := "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"
	got := deriveSigningKey(secret, "20130524", "us-east-1", "s3")

	kDate := hmacSHA256([]byte("AWS4"+secret), "20130524")
	kRegion := hmacSHA256(kDate, "us-east-1")
	kService := hmacSHA256(kRegion, "s3")
	want := hmacSHA256(kService, "aws4_request")

	if hex.EncodeToString(got) != hex.EncodeToString(want) {
		t.Fatal("derived signing key mismatch")
	}
}

func TestBuildStringToSignIncludesCanonicalHash(t *testing.T) {
	canonical := "GET\n/s3/demo-bucket/test.txt\n\nhost:localhost\nx-amz-content-sha256:UNSIGNED-PAYLOAD\nx-amz-date:20260320T020000Z\n\nhost;x-amz-content-sha256;x-amz-date\nUNSIGNED-PAYLOAD"
	got := buildStringToSign("20260320T020000Z", "20260320/us-east-1/s3/aws4_request", canonical)

	sum := sha256.Sum256([]byte(canonical))
	expectedHash := hex.EncodeToString(sum[:])
	if !strings.Contains(got, expectedHash) {
		t.Fatalf("expected string to sign to include canonical hash %s", expectedHash)
	}
}

func TestGeneratePresignedURLIncludesSigV4QueryFields(t *testing.T) {
	now := time.Date(2026, 3, 20, 3, 0, 0, 0, time.UTC)
	got, err := GeneratePresignedURL(http.MethodGet, "http://localhost/s3/demo-bucket/test.txt", "AKID123", "secret123", "us-east-1", 15*time.Minute, now)
	if err != nil {
		t.Fatalf("generate presigned url: %v", err)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse generated url: %v", err)
	}
	query := parsed.Query()
	if query.Get("X-Amz-Algorithm") != algorithm {
		t.Fatalf("unexpected algorithm %s", query.Get("X-Amz-Algorithm"))
	}
	if query.Get("X-Amz-SignedHeaders") != "host" {
		t.Fatalf("unexpected signed headers %s", query.Get("X-Amz-SignedHeaders"))
	}
	if query.Get("X-Amz-Signature") == "" {
		t.Fatal("expected signature to be present")
	}
}

func TestPresignedExpiryValid(t *testing.T) {
	requestTime := time.Date(2026, 3, 20, 3, 0, 0, 0, time.UTC)
	if !presignedExpiryValid(requestTime, 60, requestTime.Add(59*time.Second)) {
		t.Fatal("expected url to be valid before expiry")
	}
	if presignedExpiryValid(requestTime, 60, requestTime.Add(61*time.Second)) {
		t.Fatal("expected url to be invalid after expiry")
	}
}
