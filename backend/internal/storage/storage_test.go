package storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalStoreEncryptsAtRestAndDecryptsOnRead(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocal(root, "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	if err != nil {
		t.Fatalf("new local store: %v", err)
	}

	relativePath := filepath.ToSlash(filepath.Join("tenants", "default", "buckets", "bucket-1", "objects", "object-1"))
	plaintext := "hello encrypted world"
	if err := store.Put(context.Background(), relativePath, strings.NewReader(plaintext)); err != nil {
		t.Fatalf("put object: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, relativePath))
	if err != nil {
		t.Fatalf("read raw file: %v", err)
	}
	if strings.Contains(string(raw), plaintext) {
		t.Fatal("expected plaintext not to appear in stored file")
	}

	reader, err := store.Get(context.Background(), relativePath)
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	defer reader.Close()

	decrypted, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read decrypted object: %v", err)
	}
	if string(decrypted) != plaintext {
		t.Fatalf("expected %q, got %q", plaintext, string(decrypted))
	}
}

func TestLocalStoreDeleteIgnoresMissingFiles(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocal(root, "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	if err != nil {
		t.Fatalf("new local store: %v", err)
	}

	if err := store.Delete(context.Background(), filepath.ToSlash(filepath.Join("tenants", "default", "missing"))); err != nil {
		t.Fatalf("delete missing file: %v", err)
	}
}

func TestNewStoreRejectsUnsupportedBackend(t *testing.T) {
	_, err := NewStore("mystery", t.TempDir(), "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=", nil, 0, "token", nil)
	if err == nil {
		t.Fatal("expected unsupported backend error")
	}
}

func TestNewStoreRejectsDistributedWithoutEndpoints(t *testing.T) {
	_, err := NewStore("distributed", t.TempDir(), "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=", nil, 0, "token", nil)
	if err == nil {
		t.Fatal("expected distributed backend without endpoints to fail")
	}
}

func TestDistributedStoreMirrorsAcrossEndpoints(t *testing.T) {
	payloads := map[string][]byte{}
	handler := func(w http.ResponseWriter, req *http.Request) {
		if !VerifyBlobNodeAuthRequest(req, "token", time.Now().UTC(), BlobNodeSignatureTolerance) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch req.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(req.Body)
			payloads[req.URL.Path] = body
			w.WriteHeader(http.StatusNoContent)
		case http.MethodGet:
			body, ok := payloads[req.URL.Path]
			if !ok {
				http.NotFound(w, req)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		case http.MethodDelete:
			delete(payloads, req.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unsupported", http.StatusMethodNotAllowed)
		}
	}

	serverA := httptest.NewServer(http.HandlerFunc(handler))
	defer serverA.Close()
	serverB := httptest.NewServer(http.HandlerFunc(handler))
	defer serverB.Close()

	store := NewDistributed([]string{serverA.URL, serverB.URL}, 0, "token", nil)
	if err := store.Put(context.Background(), "tenants/default/test", strings.NewReader("replicated")); err != nil {
		t.Fatalf("put distributed object: %v", err)
	}
	reader, err := store.Get(context.Background(), "tenants/default/test")
	if err != nil {
		t.Fatalf("get distributed object: %v", err)
	}
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read distributed object: %v", err)
	}
	if string(body) != "replicated" {
		t.Fatalf("unexpected distributed body %q", string(body))
	}
	if err := store.Delete(context.Background(), "tenants/default/test"); err != nil {
		t.Fatalf("delete distributed object: %v", err)
	}
}

func TestDistributedStoreExistsAndCopyBetweenEndpoints(t *testing.T) {
	payloads := map[string][]byte{}
	handler := func(w http.ResponseWriter, req *http.Request) {
		if !VerifyBlobNodeAuthRequest(req, "token", time.Now().UTC(), BlobNodeSignatureTolerance) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch req.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(req.Body)
			payloads[req.Host+req.URL.Path] = body
			w.WriteHeader(http.StatusNoContent)
		case http.MethodHead:
			if _, ok := payloads[req.Host+req.URL.Path]; !ok {
				http.NotFound(w, req)
				return
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			body, ok := payloads[req.Host+req.URL.Path]
			if !ok {
				http.NotFound(w, req)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		case http.MethodDelete:
			delete(payloads, req.Host+req.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unsupported", http.StatusMethodNotAllowed)
		}
	}

	serverA := httptest.NewServer(http.HandlerFunc(handler))
	defer serverA.Close()
	serverB := httptest.NewServer(http.HandlerFunc(handler))
	defer serverB.Close()

	store := NewDistributed([]string{serverA.URL, serverB.URL}, 0, "token", nil)
	if err := store.Put(context.Background(), "tenants/default/repair", strings.NewReader("replica")); err != nil {
		t.Fatalf("put distributed object: %v", err)
	}

	exists, err := store.ExistsOnEndpoint(context.Background(), serverA.URL, "tenants/default/repair")
	if err != nil {
		t.Fatalf("exists on endpoint: %v", err)
	}
	if !exists {
		t.Fatal("expected replica to exist on source endpoint")
	}

	if err := store.deleteOne(context.Background(), serverB.URL, "tenants/default/repair"); err != nil {
		t.Fatalf("delete endpoint replica: %v", err)
	}
	exists, err = store.ExistsOnEndpoint(context.Background(), serverB.URL, "tenants/default/repair")
	if err != nil {
		t.Fatalf("exists on target after delete: %v", err)
	}
	if exists {
		t.Fatal("expected replica to be missing after delete")
	}

	if err := store.CopyBetweenEndpoints(context.Background(), serverA.URL, serverB.URL, "tenants/default/repair"); err != nil {
		t.Fatalf("copy between endpoints: %v", err)
	}
	exists, err = store.ExistsOnEndpoint(context.Background(), serverB.URL, "tenants/default/repair")
	if err != nil {
		t.Fatalf("exists on target after repair: %v", err)
	}
	if !exists {
		t.Fatal("expected replica to be restored after copy")
	}
}

func TestDistributedStoreHonorsReplicaLimit(t *testing.T) {
	payloads := map[string][]byte{}
	handler := func(w http.ResponseWriter, req *http.Request) {
		if !VerifyBlobNodeAuthRequest(req, "token", time.Now().UTC(), BlobNodeSignatureTolerance) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch req.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(req.Body)
			payloads[req.Host+req.URL.Path] = body
			w.WriteHeader(http.StatusNoContent)
		case http.MethodHead:
			if _, ok := payloads[req.Host+req.URL.Path]; !ok {
				http.NotFound(w, req)
				return
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			body, ok := payloads[req.Host+req.URL.Path]
			if !ok {
				http.NotFound(w, req)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		default:
			http.Error(w, "unsupported", http.StatusMethodNotAllowed)
		}
	}

	serverA := httptest.NewServer(http.HandlerFunc(handler))
	defer serverA.Close()
	serverB := httptest.NewServer(http.HandlerFunc(handler))
	defer serverB.Close()
	serverC := httptest.NewServer(http.HandlerFunc(handler))
	defer serverC.Close()

	store := NewDistributed([]string{serverA.URL, serverB.URL, serverC.URL}, 2, "token", nil)
	if err := store.Put(context.Background(), "tenants/default/replicas", strings.NewReader("replica-limit")); err != nil {
		t.Fatalf("put distributed object: %v", err)
	}

	if len(store.Endpoints()) != 2 {
		t.Fatalf("expected 2 active endpoints, got %d", len(store.Endpoints()))
	}
	if _, ok := payloads[mustHostPath(t, serverC.URL, "/blobs/tenants%2Fdefault%2Freplicas")]; ok {
		t.Fatal("expected third endpoint not to receive the blob")
	}
}

func TestBlobNodeSignedAuthHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://blobnode.local/blobs/test-object", nil)
	now := time.Unix(1_700_000_000, 0).UTC()

	WriteBlobNodeAuthHeaders(req, "token", now, nil)

	if !VerifyBlobNodeAuthRequest(req, "token", now, BlobNodeSignatureTolerance) {
		t.Fatal("expected signed request to verify")
	}
	if VerifyBlobNodeAuthRequest(req, "wrong-token", now, BlobNodeSignatureTolerance) {
		t.Fatal("expected wrong token to fail verification")
	}
	if req.Header.Get("X-Blob-Node-Nonce") == "" {
		t.Fatal("expected nonce header to be set")
	}
	if got := req.Header.Get("X-Blob-Node-Content-SHA256"); got != BlobNodeUnsignedPayload {
		t.Fatalf("expected unsigned payload marker, got %q", got)
	}
}

func TestBlobNodeSignedAuthHeadersForPutBindPayload(t *testing.T) {
	payload := []byte("payload")
	req := httptest.NewRequest(http.MethodPut, "http://blobnode.local/blobs/test-object", io.NopCloser(strings.NewReader(string(payload))))
	now := time.Unix(1_700_000_000, 0).UTC()

	WriteBlobNodeAuthHeaders(req, "token", now, payload)

	if !VerifyBlobNodeAuthRequest(req, "token", now, BlobNodeSignatureTolerance) {
		t.Fatal("expected signed put request to verify")
	}
	if !VerifyBlobNodePayload(payload, req.Header.Get("X-Blob-Node-Content-SHA256")) {
		t.Fatal("expected payload hash to match body")
	}
	if VerifyBlobNodePayload([]byte("tampered"), req.Header.Get("X-Blob-Node-Content-SHA256")) {
		t.Fatal("expected tampered payload to fail hash verification")
	}

	req.Header.Set("X-Blob-Node-Nonce", "different")
	if VerifyBlobNodeAuthRequest(req, "token", now, BlobNodeSignatureTolerance) {
		t.Fatal("expected modified nonce to fail verification")
	}
}

func mustHostPath(t *testing.T, rawURL, path string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, rawURL+path, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	return req.URL.Host + req.URL.Path
}
