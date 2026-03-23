package s3

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"harborshield/backend/internal/objects"
)

func TestWriteErrorReturnsXMLPayload(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/s3/demo-bucket/demo.txt", nil)
	req.Header.Set("X-Request-Id", "req-123")
	recorder := httptest.NewRecorder()

	writeError(recorder, req, http.StatusNotFound, "NoSuchKey", "The specified key does not exist.")

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", recorder.Code)
	}

	var payload errorResponse
	if err := xml.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal xml: %v", err)
	}
	if payload.Code != "NoSuchKey" {
		t.Fatalf("expected NoSuchKey, got %s", payload.Code)
	}
	if payload.Resource != "/s3/demo-bucket/demo.txt" {
		t.Fatalf("unexpected resource %s", payload.Resource)
	}
	if payload.RequestID != "req-123" {
		t.Fatalf("unexpected request id %s", payload.RequestID)
	}
}

func TestExtractUserMetadata(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Amz-Meta-Team", "platform")
	headers.Set("X-Amz-Meta-Env", "dev")
	headers.Set("Content-Type", "text/plain")

	got := extractUserMetadata(headers)
	if len(got) != 2 {
		t.Fatalf("expected 2 metadata entries, got %#v", got)
	}
	if got["team"] != "platform" || got["env"] != "dev" {
		t.Fatalf("unexpected metadata %#v", got)
	}
}

func TestApplyObjectHeaders(t *testing.T) {
	recorder := httptest.NewRecorder()
	item := objects.Metadata{
		ContentType:        "text/plain",
		CacheControl:       "no-cache",
		ContentDisposition: "attachment; filename=test.txt",
		ContentEncoding:    "gzip",
		ETag:               "etag-123",
		SizeBytes:          12,
		UserMetadata: map[string]string{
			"team": "platform",
		},
		Tags: map[string]string{
			"classification": "internal",
		},
	}

	applyObjectHeaders(recorder, item)

	if got := recorder.Header().Get("Content-Type"); got != "text/plain" {
		t.Fatalf("unexpected content type %s", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("unexpected cache control %s", got)
	}
	if got := recorder.Header().Get("Content-Disposition"); got != "attachment; filename=test.txt" {
		t.Fatalf("unexpected content disposition %s", got)
	}
	if got := recorder.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("unexpected content encoding %s", got)
	}
	if got := recorder.Header().Get("ETag"); got != "\"etag-123\"" {
		t.Fatalf("unexpected etag %s", got)
	}
	if got := recorder.Header().Get("x-amz-meta-team"); got != "platform" {
		t.Fatalf("unexpected user metadata %s", got)
	}
	if got := recorder.Header().Get("x-amz-tagging-count"); got != "1" {
		t.Fatalf("unexpected tagging count %s", got)
	}
}

func TestParseCopySource(t *testing.T) {
	bucket, key, versionID, err := parseCopySource("/source-bucket/folder%2Fdoc.txt?versionId=abc-123")
	if err != nil {
		t.Fatalf("parse copy source: %v", err)
	}
	if bucket != "source-bucket" || key != "folder/doc.txt" || versionID != "abc-123" {
		t.Fatalf("unexpected copy source values %q %q %q", bucket, key, versionID)
	}
}

func TestParseTaggingXML(t *testing.T) {
	tags, err := parseTaggingXML(strings.NewReader(`<Tagging><TagSet><Tag><Key>team</Key><Value>platform</Value></Tag><Tag><Key>tier</Key><Value>gold</Value></Tag></TagSet></Tagging>`))
	if err != nil {
		t.Fatalf("parse tagging xml: %v", err)
	}
	if tags["team"] != "platform" || tags["tier"] != "gold" {
		t.Fatalf("unexpected tags %#v", tags)
	}
}
