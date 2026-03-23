package quotas

import "testing"

func TestAllows(t *testing.T) {
	maxBytes := int64(100)
	maxObjects := int64(5)
	snapshot := Snapshot{CurrentBytes: 80, CurrentObjects: 4, MaxBytes: &maxBytes, MaxObjects: &maxObjects}
	if !Allows(10, 1, snapshot) {
		t.Fatal("expected insert to fit quota")
	}
	if Allows(25, 0, snapshot) {
		t.Fatal("expected bytes quota denial")
	}
	if Allows(0, 2, snapshot) {
		t.Fatal("expected object quota denial")
	}
}
