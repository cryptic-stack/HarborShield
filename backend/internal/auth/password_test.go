package auth

import "testing"

func TestHashAndComparePassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if err := ComparePassword(hash, "correct horse battery staple"); err != nil {
		t.Fatalf("compare password: %v", err)
	}
	if err := ComparePassword(hash, "wrong"); err == nil {
		t.Fatal("expected mismatch for wrong password")
	}
}
