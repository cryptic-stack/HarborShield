package quotas

import "errors"

type Snapshot struct {
	CurrentBytes   int64
	CurrentObjects int64
	MaxBytes       *int64
	MaxObjects     *int64
}

type WarningState struct {
	BucketBytes bool
	BucketCount bool
	UserBytes   bool
}

var ErrQuotaExceeded = errors.New("quota exceeded")

func Allows(nextBytes int64, nextObjects int64, snapshot Snapshot) bool {
	if snapshot.MaxBytes != nil && snapshot.CurrentBytes+nextBytes > *snapshot.MaxBytes {
		return false
	}
	if snapshot.MaxObjects != nil && snapshot.CurrentObjects+nextObjects > *snapshot.MaxObjects {
		return false
	}
	return true
}
