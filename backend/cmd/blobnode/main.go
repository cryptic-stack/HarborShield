package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"harborshield/backend/internal/config"
	"harborshield/backend/internal/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	name := os.Getenv("BLOBNODE_NAME")
	if name == "" {
		name = "blobnode"
	}
	port := os.Getenv("BLOBNODE_PORT")
	if port == "" {
		port = "9100"
	}
	root := os.Getenv("BLOBNODE_ROOT")
	if root == "" {
		root = cfg.StorageRoot
	}
	tokenFilePath := os.Getenv("BLOBNODE_RPC_TOKEN_FILE")
	if tokenFilePath == "" {
		tokenFilePath = filepath.Join(root, ".blobnode-rpc-token")
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	store, err := storage.NewLocal(root, cfg.StorageMasterKey)
	if err != nil {
		log.Fatal(err)
	}
	nodeToken, err := loadNodeToken(tokenFilePath)
	if err != nil {
		log.Fatal(err)
	}
	if token, err := maybeJoinCluster(context.Background(), logger, name, tokenFilePath); err != nil {
		logger.Error("blobnode_join_failed", slog.String("name", name), slog.String("error", err.Error()))
	} else if token != "" {
		nodeToken = token
	}
	replayGuard := newNonceGuard()
	serverTLSConfig, err := loadBlobNodeServerTLSConfig(cfg.BlobNodeTLSCertFile, cfg.BlobNodeTLSKeyFile, cfg.BlobNodeTLSClientCAFile)
	if err != nil {
		log.Fatal(err)
	}
	requireClientTLS := strings.TrimSpace(cfg.BlobNodeTLSClientCAFile) != ""

	router := chi.NewRouter()
	router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		capacityBytes, usedBytes, fileCount := storageStats(root)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":        "ok",
			"service":       "blobnode",
			"name":          name,
			"capacityBytes": capacityBytes,
			"usedBytes":     usedBytes,
			"fileCount":     fileCount,
		})
	})
	handleBlob := func(w http.ResponseWriter, req *http.Request) {
		expectedToken := cfg.StorageNodeSharedSecret
		if strings.TrimSpace(nodeToken) != "" {
			expectedToken = nodeToken
		}
		if !storage.VerifyBlobNodeAuthRequest(req, expectedToken, time.Now().UTC(), storage.BlobNodeSignatureTolerance) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if requireClientTLS {
			if req.TLS == nil || len(req.TLS.VerifiedChains) == 0 {
				http.Error(w, "client certificate required", http.StatusUnauthorized)
				return
			}
		}
		if !replayGuard.Accept(req.Header.Get("X-Blob-Node-Nonce"), time.Now().UTC(), storage.BlobNodeSignatureTolerance) {
			http.Error(w, "replayed request", http.StatusUnauthorized)
			return
		}
		path := normalizePath(chi.URLParam(req, "*"))
		switch req.Method {
		case http.MethodPut:
			body, err := io.ReadAll(req.Body)
			if err != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
			if !storage.VerifyBlobNodePayload(body, req.Header.Get("X-Blob-Node-Content-SHA256")) {
				http.Error(w, "payload hash mismatch", http.StatusUnauthorized)
				return
			}
			if err := store.Put(req.Context(), path, bytes.NewReader(body)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case http.MethodHead:
			reader, err := store.Get(req.Context(), path)
			if err != nil {
				if os.IsNotExist(err) {
					http.NotFound(w, req)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			reader.Close()
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			reader, err := store.Get(req.Context(), path)
			if err != nil {
				if os.IsNotExist(err) {
					http.NotFound(w, req)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer reader.Close()
			w.WriteHeader(http.StatusOK)
			_, _ = io.Copy(w, reader)
		case http.MethodDelete:
			if err := store.Delete(req.Context(), path); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
	router.MethodFunc(http.MethodPut, "/blobs/*", handleBlob)
	router.MethodFunc(http.MethodHead, "/blobs/*", handleBlob)
	router.MethodFunc(http.MethodGet, "/blobs/*", handleBlob)
	router.MethodFunc(http.MethodDelete, "/blobs/*", handleBlob)

	logger.Info("blobnode_listening", slog.String("name", name), slog.String("addr", ":"+port), slog.String("root", root), slog.Bool("tls", serverTLSConfig != nil))
	server := &http.Server{Addr: ":" + port, Handler: router, TLSConfig: serverTLSConfig}
	var serveErr error
	if serverTLSConfig != nil {
		serveErr = server.ListenAndServeTLS("", "")
	} else {
		serveErr = server.ListenAndServe()
	}
	if err := serveErr; err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

type nonceGuard struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func newNonceGuard() *nonceGuard {
	return &nonceGuard{seen: map[string]time.Time{}}
}

func (g *nonceGuard) Accept(nonce string, now time.Time, ttl time.Duration) bool {
	nonce = strings.TrimSpace(nonce)
	if nonce == "" {
		return false
	}
	if ttl <= 0 {
		ttl = storage.BlobNodeSignatureTolerance
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	for key, expiry := range g.seen {
		if !expiry.After(now) {
			delete(g.seen, key)
		}
	}

	if expiry, ok := g.seen[nonce]; ok && expiry.After(now) {
		return false
	}

	g.seen[nonce] = now.Add(ttl)
	return true
}

func maybeJoinCluster(ctx context.Context, logger *slog.Logger, nodeName, tokenFilePath string) (string, error) {
	apiURL := strings.TrimRight(strings.TrimSpace(os.Getenv("BLOBNODE_API_URL")), "/")
	joinToken := strings.TrimSpace(os.Getenv("BLOBNODE_JOIN_TOKEN"))
	advertiseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("BLOBNODE_ADVERTISE_URL")), "/")
	if apiURL == "" || joinToken == "" || advertiseURL == "" {
		return "", nil
	}

	payload := map[string]string{
		"token":    joinToken,
		"name":     nodeName,
		"endpoint": advertiseURL,
		"zone":     strings.TrimSpace(os.Getenv("BLOBNODE_ZONE")),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/api/v1/internal/storage/join", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("join request failed: %s: %s", resp.Status, strings.TrimSpace(string(responseBody)))
	}
	var enrollment struct {
		RPCToken string `json:"rpcToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&enrollment); err != nil {
		return "", err
	}
	if strings.TrimSpace(enrollment.RPCToken) == "" {
		return "", fmt.Errorf("join response missing rpcToken")
	}
	if err := persistNodeToken(tokenFilePath, enrollment.RPCToken); err != nil {
		return "", err
	}
	logger.Info("blobnode_joined", slog.String("name", nodeName), slog.String("apiUrl", apiURL), slog.String("advertiseUrl", advertiseURL))
	return enrollment.RPCToken, nil
}

func loadNodeToken(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}

func persistNodeToken(path, token string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(token+"\n"), 0o600)
}

func normalizePath(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "/")
	if decoded, err := url.PathUnescape(value); err == nil {
		return decoded
	}
	return value
}

func storageStats(root string) (capacityBytes int64, usedBytes int64, fileCount int64) {
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		usedBytes += info.Size()
		fileCount++
		return nil
	})
	return 0, usedBytes, fileCount
}

func loadBlobNodeServerTLSConfig(certFile, keyFile, clientCAFile string) (*tls.Config, error) {
	if strings.TrimSpace(certFile) == "" && strings.TrimSpace(keyFile) == "" {
		return nil, nil
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load blobnode server certificate: %w", err)
	}
	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.NoClientCert,
	}
	if strings.TrimSpace(clientCAFile) != "" {
		caPEM, err := os.ReadFile(clientCAFile)
		if err != nil {
			return nil, fmt.Errorf("read blobnode client CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("parse blobnode client CA file")
		}
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		tlsConfig.ClientCAs = pool
	}
	return tlsConfig, nil
}
