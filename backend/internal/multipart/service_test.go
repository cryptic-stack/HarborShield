package multipart

import (
	"strings"
	"testing"
)

func TestUploadPath(t *testing.T) {
	got := UploadPath("default", "bucket-1", "upload-1", 3)
	want := "tenants/default/buckets/bucket-1/multipart/upload-1/3"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestParseCompleteRequest(t *testing.T) {
	body := strings.NewReader(`<CompleteMultipartUpload><Part><PartNumber>1</PartNumber><ETag>etag-1</ETag></Part><Part><PartNumber>2</PartNumber><ETag>etag-2</ETag></Part></CompleteMultipartUpload>`)
	got, err := ParseCompleteRequest(body)
	if err != nil {
		t.Fatalf("parse complete request: %v", err)
	}
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("unexpected parsed parts: %#v", got)
	}
}

func TestSortPartNumbers(t *testing.T) {
	got := SortPartNumbers([]int{3, 1, 2})
	if got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("unexpected sort result: %#v", got)
	}
}

func TestStrictAscending(t *testing.T) {
	if !strictAscending([]int{1, 2, 3}) {
		t.Fatal("expected ascending part numbers to be accepted")
	}
	if strictAscending([]int{1, 1, 2}) {
		t.Fatal("expected duplicate part numbers to be rejected")
	}
	if strictAscending([]int{2, 1}) {
		t.Fatal("expected out-of-order parts to be rejected")
	}
}
