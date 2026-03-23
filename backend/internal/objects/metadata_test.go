package objects

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"testing"
)

func TestPhysicalPath(t *testing.T) {
	got := PhysicalPath("default", "bucket-1", "object-1")
	want := "tenants/default/buckets/bucket-1/objects/object-1"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestValidatePayloadAcceptsMatchingLengthAndDigest(t *testing.T) {
	payload := []byte("hello world")
	sum := md5.Sum(payload)
	contentMD5 := base64.StdEncoding.EncodeToString(sum[:])

	if err := ValidatePayload(payload, int64(len(payload)), contentMD5); err != nil {
		t.Fatalf("validate payload: %v", err)
	}
}

func TestValidatePayloadRejectsLengthMismatch(t *testing.T) {
	err := ValidatePayload([]byte("hello"), 99, "")
	if !errors.Is(err, ErrIncompleteBody) {
		t.Fatalf("expected ErrIncompleteBody, got %v", err)
	}
}

func TestValidatePayloadRejectsInvalidDigest(t *testing.T) {
	err := ValidatePayload([]byte("hello"), 5, "not-base64")
	if !errors.Is(err, ErrInvalidDigest) {
		t.Fatalf("expected ErrInvalidDigest, got %v", err)
	}
}

func TestValidatePayloadRejectsMismatchedDigest(t *testing.T) {
	err := ValidatePayload([]byte("hello"), 5, base64.StdEncoding.EncodeToString([]byte("wrongwrongwrong12")))
	if !errors.Is(err, ErrBadDigest) {
		t.Fatalf("expected ErrBadDigest, got %v", err)
	}
}

func TestDeleteVersionRejectsInvalidVersionID(t *testing.T) {
	service := &Service{}
	err := service.DeleteVersion(context.Background(), "bucket-1", "demo.txt", "not-a-uuid")
	if !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("expected ErrVersionNotFound, got %v", err)
	}
}
