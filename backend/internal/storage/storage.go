package storage

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type BlobStore interface {
	Put(ctx context.Context, path string, body io.Reader) error
	Get(ctx context.Context, path string) (io.ReadCloser, error)
	Delete(ctx context.Context, path string) error
}

var ErrUnsupportedBackend = errors.New("unsupported storage backend")

type LocalStore struct {
	root string
	aead cipher.AEAD
}

type DistributedStore struct {
	endpoints      []string
	client         *http.Client
	sharedToken    string
	mu             sync.RWMutex
	endpointTokens map[string]string
}

const fileMagic = "HSENC1"
const BlobNodeSignatureTolerance = 5 * time.Minute
const BlobNodeUnsignedPayload = "UNSIGNED-PAYLOAD"

func NewLocal(root, base64Key string) (*LocalStore, error) {
	key, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return nil, fmt.Errorf("decode storage key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("storage key must decode to 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("build cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("build gcm: %w", err)
	}
	return &LocalStore{root: root, aead: aead}, nil
}

func NewStore(backend, root, base64Key string, distributedEndpoints []string, distributedReplicas int, distributedToken string, tlsConfig *tls.Config) (BlobStore, error) {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "", "local":
		return NewLocal(root, base64Key)
	case "distributed":
		if len(distributedEndpoints) == 0 {
			return nil, fmt.Errorf("%w: STORAGE_DISTRIBUTED_ENDPOINTS is required when STORAGE_BACKEND=distributed", ErrUnsupportedBackend)
		}
		return NewDistributed(distributedEndpoints, distributedReplicas, distributedToken, tlsConfig), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedBackend, backend)
	}
}

func NewDistributed(endpoints []string, replicas int, token string, tlsConfig *tls.Config) *DistributedStore {
	normalized := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
		if endpoint != "" {
			normalized = append(normalized, endpoint)
		}
	}
	if replicas > 0 && replicas < len(normalized) {
		normalized = normalized[:replicas]
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       tlsConfig,
	}
	return &DistributedStore{
		endpoints:      normalized,
		client:         &http.Client{Timeout: 30 * time.Second, Transport: transport},
		sharedToken:    token,
		endpointTokens: map[string]string{},
	}
}

func (s *DistributedStore) Endpoints() []string {
	return append([]string(nil), s.endpoints...)
}

func (s *DistributedStore) SetEndpointToken(endpoint, token string) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(token) == "" {
		delete(s.endpointTokens, endpoint)
		return
	}
	s.endpointTokens[endpoint] = token
}

func (s *DistributedStore) tokenForEndpoint(endpoint string) string {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	s.mu.RLock()
	defer s.mu.RUnlock()
	if token, ok := s.endpointTokens[endpoint]; ok && token != "" {
		return token
	}
	return s.sharedToken
}

func (s *LocalStore) Put(_ context.Context, path string, body io.Reader) error {
	fullPath := filepath.Join(s.root, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return fmt.Errorf("mkdir storage path: %w", err)
	}
	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("create object file: %w", err)
	}
	defer file.Close()
	plaintext, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("read plaintext: %w", err)
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext := s.aead.Seal(nil, nonce, plaintext, nil)
	if _, err := file.Write([]byte(fileMagic)); err != nil {
		return fmt.Errorf("write magic: %w", err)
	}
	if _, err := file.Write(nonce); err != nil {
		return fmt.Errorf("write nonce: %w", err)
	}
	if _, err := file.Write(ciphertext); err != nil {
		return fmt.Errorf("write ciphertext: %w", err)
	}
	return nil
}

func (s *LocalStore) Get(_ context.Context, path string) (io.ReadCloser, error) {
	payload, err := os.ReadFile(filepath.Join(s.root, path))
	if err != nil {
		return nil, err
	}
	if len(payload) < len(fileMagic)+s.aead.NonceSize() {
		return nil, errors.New("encrypted blob payload is too short")
	}
	if string(payload[:len(fileMagic)]) != fileMagic {
		return nil, errors.New("encrypted blob header mismatch")
	}
	nonceOffset := len(fileMagic)
	ciphertextOffset := nonceOffset + s.aead.NonceSize()
	plaintext, err := s.aead.Open(nil, payload[nonceOffset:ciphertextOffset], payload[ciphertextOffset:], nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt blob: %w", err)
	}
	return io.NopCloser(bytesReader(plaintext)), nil
}

func (s *LocalStore) Delete(_ context.Context, path string) error {
	err := os.Remove(filepath.Join(s.root, path))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func bytesReader(payload []byte) io.Reader {
	return bytes.NewReader(payload)
}

func (s *DistributedStore) Put(ctx context.Context, path string, body io.Reader) error {
	return s.putToEndpoints(ctx, path, body, s.endpoints)
}

func (s *DistributedStore) PutToEndpoints(ctx context.Context, path string, body io.Reader, endpoints []string) error {
	return s.putToEndpoints(ctx, path, body, endpoints)
}

func (s *DistributedStore) putToEndpoints(ctx context.Context, path string, body io.Reader, endpoints []string) error {
	payload, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("read distributed payload: %w", err)
	}
	targets := s.normalizeTargetEndpoints(endpoints)
	written := make([]string, 0, len(targets))
	for _, endpoint := range targets {
		if err := s.putOne(ctx, endpoint, path, payload); err != nil {
			for _, writtenEndpoint := range written {
				_ = s.deleteOne(ctx, writtenEndpoint, path)
			}
			return err
		}
		written = append(written, endpoint)
	}
	return nil
}

func (s *DistributedStore) Get(ctx context.Context, path string) (io.ReadCloser, error) {
	var lastErr error
	for _, endpoint := range s.endpoints {
		reader, err := s.getOne(ctx, endpoint, path)
		if err == nil {
			return reader, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = os.ErrNotExist
	}
	return nil, lastErr
}

func (s *DistributedStore) Delete(ctx context.Context, path string) error {
	var lastErr error
	for _, endpoint := range s.endpoints {
		if err := s.deleteOne(ctx, endpoint, path); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (s *DistributedStore) normalizeTargetEndpoints(endpoints []string) []string {
	if len(endpoints) == 0 {
		return append([]string(nil), s.endpoints...)
	}
	allowed := map[string]struct{}{}
	for _, endpoint := range s.endpoints {
		allowed[endpoint] = struct{}{}
	}
	out := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
		if endpoint == "" {
			continue
		}
		if _, ok := allowed[endpoint]; !ok {
			continue
		}
		out = append(out, endpoint)
	}
	if len(out) == 0 {
		return append([]string(nil), s.endpoints...)
	}
	return out
}

func (s *DistributedStore) ExistsOnEndpoint(ctx context.Context, endpoint, path string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, blobNodeURL(endpoint, path), nil)
	if err != nil {
		return false, err
	}
	WriteBlobNodeAuthHeaders(req, s.tokenForEndpoint(endpoint), time.Now().UTC(), nil)
	resp, err := s.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("blob node head failed: %s", resp.Status)
	}
	return true, nil
}

func (s *DistributedStore) CopyBetweenEndpoints(ctx context.Context, sourceEndpoint, targetEndpoint, path string) error {
	reader, err := s.getOne(ctx, sourceEndpoint, path)
	if err != nil {
		return err
	}
	defer reader.Close()

	payload, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("read replica payload: %w", err)
	}

	return s.putOne(ctx, targetEndpoint, path, payload)
}

func (s *DistributedStore) putOne(ctx context.Context, endpoint, path string, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, blobNodeURL(endpoint, path), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	WriteBlobNodeAuthHeaders(req, s.tokenForEndpoint(endpoint), time.Now().UTC(), payload)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("blob node put failed: %s", resp.Status)
	}
	return nil
}

func (s *DistributedStore) getOne(ctx context.Context, endpoint, path string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, blobNodeURL(endpoint, path), nil)
	if err != nil {
		return nil, err
	}
	WriteBlobNodeAuthHeaders(req, s.tokenForEndpoint(endpoint), time.Now().UTC(), nil)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, os.ErrNotExist
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("blob node get failed: %s", resp.Status)
	}
	return resp.Body, nil
}

func (s *DistributedStore) deleteOne(ctx context.Context, endpoint, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, blobNodeURL(endpoint, path), nil)
	if err != nil {
		return err
	}
	WriteBlobNodeAuthHeaders(req, s.tokenForEndpoint(endpoint), time.Now().UTC(), nil)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return fmt.Errorf("blob node delete failed: %s", resp.Status)
}

func blobNodeURL(endpoint, path string) string {
	return strings.TrimRight(endpoint, "/") + "/blobs/" + url.PathEscape(strings.TrimLeft(path, "/"))
}

func WriteBlobNodeAuthHeaders(req *http.Request, token string, now time.Time, payload []byte) {
	timestamp := strconv.FormatInt(now.Unix(), 10)
	nonce := newBlobNodeNonce()
	payloadHash := blobNodePayloadHash(payload)
	req.Header.Set("X-Blob-Node-Timestamp", timestamp)
	req.Header.Set("X-Blob-Node-Nonce", nonce)
	req.Header.Set("X-Blob-Node-Content-SHA256", payloadHash)
	req.Header.Set("X-Blob-Node-Signature", signBlobNodeRequest(req.Method, req.URL.EscapedPath(), timestamp, nonce, payloadHash, token))
}

func VerifyBlobNodeAuthRequest(req *http.Request, token string, now time.Time, tolerance time.Duration) bool {
	if tolerance <= 0 {
		tolerance = BlobNodeSignatureTolerance
	}
	timestamp := strings.TrimSpace(req.Header.Get("X-Blob-Node-Timestamp"))
	nonce := strings.TrimSpace(req.Header.Get("X-Blob-Node-Nonce"))
	signature := strings.TrimSpace(req.Header.Get("X-Blob-Node-Signature"))
	payloadHash := strings.TrimSpace(req.Header.Get("X-Blob-Node-Content-SHA256"))
	if timestamp == "" || nonce == "" || signature == "" || payloadHash == "" || strings.TrimSpace(token) == "" {
		return false
	}
	unixSeconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	requestTime := time.Unix(unixSeconds, 0).UTC()
	if requestTime.Before(now.UTC().Add(-tolerance)) || requestTime.After(now.UTC().Add(tolerance)) {
		return false
	}
	expected := signBlobNodeRequest(req.Method, req.URL.EscapedPath(), timestamp, nonce, payloadHash, token)
	return hmac.Equal([]byte(expected), []byte(signature))
}

func VerifyBlobNodePayload(body []byte, headerValue string) bool {
	return strings.EqualFold(strings.TrimSpace(headerValue), blobNodePayloadHash(body))
}

func signBlobNodeRequest(method, escapedPath, timestamp, nonce, payloadHash, token string) string {
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write([]byte(strings.ToUpper(method)))
	mac.Write([]byte("\n"))
	mac.Write([]byte(escapedPath))
	mac.Write([]byte("\n"))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("\n"))
	mac.Write([]byte(nonce))
	mac.Write([]byte("\n"))
	mac.Write([]byte(payloadHash))
	return hex.EncodeToString(mac.Sum(nil))
}

func blobNodePayloadHash(payload []byte) string {
	if len(payload) == 0 {
		return BlobNodeUnsignedPayload
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func newBlobNodeNonce() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		fallback := sha256.Sum256([]byte(strconv.FormatInt(time.Now().UnixNano(), 10)))
		return hex.EncodeToString(fallback[:16])
	}
	return hex.EncodeToString(value)
}

func LoadBlobNodeClientTLSConfig(caFile, certFile, keyFile string) (*tls.Config, error) {
	if strings.TrimSpace(caFile) == "" && strings.TrimSpace(certFile) == "" && strings.TrimSpace(keyFile) == "" {
		return nil, nil
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if strings.TrimSpace(caFile) != "" {
		caPEM, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("read blobnode CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, errors.New("parse blobnode CA file")
		}
		tlsConfig.RootCAs = pool
	}
	if strings.TrimSpace(certFile) != "" || strings.TrimSpace(keyFile) != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("load blobnode client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	return tlsConfig, nil
}
