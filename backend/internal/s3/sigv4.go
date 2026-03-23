package s3

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"harborshield/backend/internal/auth"
	"harborshield/backend/internal/credentials"
)

const algorithm = "AWS4-HMAC-SHA256"

type sigV4Identity struct {
	AccessKey string
	SecretKey string
	UserID    string
	Role      string
}

func authorize(req *http.Request, creds *credentials.Service, region string) (map[string]string, bool) {
	if authz := req.Header.Get("Authorization"); strings.HasPrefix(authz, algorithm) {
		identity, err := validateSigV4(req, creds, region)
		if err == nil {
			return map[string]string{"userID": identity.UserID, "role": identity.Role, "accessKey": identity.AccessKey}, true
		}
		return nil, false
	}

	accessKey := req.Header.Get("X-S3P-Access-Key")
	secretKey := req.Header.Get("X-S3P-Secret")
	if accessKey == "" || secretKey == "" {
		return nil, false
	}
	info, err := creds.Validate(req.Context(), accessKey, secretKey)
	return info, err == nil
}

func isSigV4Presigned(req *http.Request) bool {
	return req.URL.Query().Get("X-Amz-Algorithm") == algorithm
}

func validateSigV4(req *http.Request, creds *credentials.Service, region string) (sigV4Identity, error) {
	authHeader := req.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, algorithm+" ") {
		return sigV4Identity{}, fmt.Errorf("missing algorithm")
	}
	parts := parseAuthorizationHeader(strings.TrimPrefix(authHeader, algorithm+" "))
	credentialParts := strings.Split(parts["Credential"], "/")
	if len(credentialParts) != 5 {
		return sigV4Identity{}, fmt.Errorf("invalid credential scope")
	}
	accessKey := credentialParts[0]
	dateScope := credentialParts[1]
	regionScope := credentialParts[2]
	serviceScope := credentialParts[3]
	terminalScope := credentialParts[4]
	if regionScope != region || serviceScope != "s3" || terminalScope != "aws4_request" {
		return sigV4Identity{}, fmt.Errorf("invalid scope")
	}

	credential, err := creds.Lookup(req.Context(), accessKey)
	if err != nil {
		return sigV4Identity{}, err
	}
	secretKey := credential["secretKey"]
	if secretKey == "" {
		return sigV4Identity{}, fmt.Errorf("secret key unavailable")
	}

	amzDate := req.Header.Get("X-Amz-Date")
	if amzDate == "" {
		return sigV4Identity{}, fmt.Errorf("missing x-amz-date")
	}
	requestTime, err := time.Parse("20060102T150405Z", amzDate)
	if err != nil {
		return sigV4Identity{}, err
	}
	if requestTime.UTC().Format("20060102") != dateScope {
		return sigV4Identity{}, fmt.Errorf("date scope mismatch")
	}

	payloadHash, err := payloadHashForRequest(req)
	if err != nil {
		return sigV4Identity{}, err
	}
	signedHeaders := parts["SignedHeaders"]
	canonicalRequest, err := canonicalRequest(req, payloadHash, signedHeaders)
	if err != nil {
		return sigV4Identity{}, err
	}
	if signedHeaders == "" {
		return sigV4Identity{}, fmt.Errorf("missing signed headers")
	}
	stringToSign := buildStringToSign(amzDate, fmt.Sprintf("%s/%s/%s/%s", dateScope, regionScope, serviceScope, terminalScope), canonicalRequest)
	signingKey := deriveSigningKey(secretKey, dateScope, regionScope, serviceScope)
	expectedSignature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))
	if !auth.ConstantTimeEqual(expectedSignature, parts["Signature"]) {
		return sigV4Identity{}, fmt.Errorf("signature mismatch")
	}
	return sigV4Identity{AccessKey: accessKey, SecretKey: secretKey, UserID: credential["userID"], Role: credential["role"]}, nil
}

func validatePresignedSigV4(req *http.Request, creds *credentials.Service, region string, now time.Time) (sigV4Identity, error) {
	query := req.URL.Query()
	if query.Get("X-Amz-Algorithm") != algorithm {
		return sigV4Identity{}, fmt.Errorf("missing algorithm")
	}
	credentialParts := strings.Split(query.Get("X-Amz-Credential"), "/")
	if len(credentialParts) != 5 {
		return sigV4Identity{}, fmt.Errorf("invalid credential scope")
	}
	accessKey := credentialParts[0]
	dateScope := credentialParts[1]
	regionScope := credentialParts[2]
	serviceScope := credentialParts[3]
	terminalScope := credentialParts[4]
	if regionScope != region || serviceScope != "s3" || terminalScope != "aws4_request" {
		return sigV4Identity{}, fmt.Errorf("invalid scope")
	}
	expiresSeconds, err := strconv.Atoi(query.Get("X-Amz-Expires"))
	if err != nil || expiresSeconds <= 0 {
		return sigV4Identity{}, fmt.Errorf("invalid expiry")
	}
	amzDate := query.Get("X-Amz-Date")
	requestTime, err := time.Parse("20060102T150405Z", amzDate)
	if err != nil {
		return sigV4Identity{}, err
	}
	if requestTime.UTC().Format("20060102") != dateScope {
		return sigV4Identity{}, fmt.Errorf("date scope mismatch")
	}
	if !presignedExpiryValid(requestTime, expiresSeconds, now) {
		return sigV4Identity{}, fmt.Errorf("presigned url expired")
	}

	credential, err := creds.Lookup(req.Context(), accessKey)
	if err != nil {
		return sigV4Identity{}, err
	}
	secretKey := credential["secretKey"]
	if secretKey == "" {
		return sigV4Identity{}, fmt.Errorf("secret key unavailable")
	}

	signedHeaders := query.Get("X-Amz-SignedHeaders")
	if signedHeaders == "" {
		return sigV4Identity{}, fmt.Errorf("missing signed headers")
	}
	canonicalRequest, err := canonicalRequest(req, "UNSIGNED-PAYLOAD", signedHeaders)
	if err != nil {
		return sigV4Identity{}, err
	}
	stringToSign := buildStringToSign(amzDate, fmt.Sprintf("%s/%s/%s/%s", dateScope, regionScope, serviceScope, terminalScope), canonicalRequest)
	signingKey := deriveSigningKey(secretKey, dateScope, regionScope, serviceScope)
	expected := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))
	if !auth.ConstantTimeEqual(expected, query.Get("X-Amz-Signature")) {
		return sigV4Identity{}, fmt.Errorf("signature mismatch")
	}
	return sigV4Identity{AccessKey: accessKey, SecretKey: secretKey, UserID: credential["userID"], Role: credential["role"]}, nil
}

func GeneratePresignedURL(method, rawURL, accessKey, secretKey, region string, ttl time.Duration, now time.Time) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	amzDate := now.UTC().Format("20060102T150405Z")
	dateScope := now.UTC().Format("20060102")
	scope := fmt.Sprintf("%s/%s/s3/aws4_request", dateScope, region)
	query := parsed.Query()
	query.Set("X-Amz-Algorithm", algorithm)
	query.Set("X-Amz-Credential", accessKey+"/"+scope)
	query.Set("X-Amz-Date", amzDate)
	query.Set("X-Amz-Expires", strconv.Itoa(int(ttl.Seconds())))
	query.Set("X-Amz-SignedHeaders", "host")
	parsed.RawQuery = query.Encode()

	req, err := http.NewRequest(method, parsed.String(), nil)
	if err != nil {
		return "", err
	}
	req.Host = parsed.Host
	canonical, err := canonicalRequest(req, "UNSIGNED-PAYLOAD", "host")
	if err != nil {
		return "", err
	}
	stringToSign := buildStringToSign(amzDate, scope, canonical)
	signature := hex.EncodeToString(hmacSHA256(deriveSigningKey(secretKey, dateScope, region, "s3"), stringToSign))
	query.Set("X-Amz-Signature", signature)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func presignedExpiryValid(requestTime time.Time, expiresSeconds int, now time.Time) bool {
	return !now.UTC().After(requestTime.Add(time.Duration(expiresSeconds) * time.Second))
}

func parseAuthorizationHeader(value string) map[string]string {
	result := map[string]string{}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		result[kv[0]] = kv[1]
	}
	return result
}

func payloadHashForRequest(req *http.Request) (string, error) {
	hashHeader := req.Header.Get("X-Amz-Content-Sha256")
	if hashHeader == "" {
		return "", fmt.Errorf("missing x-amz-content-sha256")
	}
	if hashHeader == "UNSIGNED-PAYLOAD" {
		return hashHeader, nil
	}
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		return "", err
	}
	req.Body = io.NopCloser(bytes.NewReader(payload))
	sum := sha256.Sum256(payload)
	actual := hex.EncodeToString(sum[:])
	if !auth.ConstantTimeEqual(actual, hashHeader) {
		return "", fmt.Errorf("payload hash mismatch")
	}
	return hashHeader, nil
}

func canonicalRequest(req *http.Request, payloadHash, signedHeaders string) (string, error) {
	canonicalHeaders := canonicalHeaders(req.Header, signedHeaders, req.Host)
	canonicalQuery := canonicalQueryString(req.URL.Query())
	canonicalURI := canonicalURI(req.URL.EscapedPath())
	return strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n"), nil
}

func canonicalHeaders(headers http.Header, signedHeaders, host string) string {
	out := []string{}
	for _, name := range strings.Split(signedHeaders, ";") {
		if name == "host" {
			out = append(out, "host:"+strings.TrimSpace(host))
			continue
		}
		values := headers.Values(name)
		if len(values) == 0 {
			values = headers.Values(http.CanonicalHeaderKey(name))
		}
		normalized := make([]string, 0, len(values))
		for _, value := range values {
			normalized = append(normalized, strings.Join(strings.Fields(value), " "))
		}
		out = append(out, name+":"+strings.Join(normalized, ","))
	}
	return strings.Join(out, "\n") + "\n"
}

func canonicalQueryString(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	pairs := []string{}
	for key, items := range values {
		if strings.EqualFold(key, "X-Amz-Signature") {
			continue
		}
		sort.Strings(items)
		escapedKey := url.QueryEscape(key)
		for _, item := range items {
			pairs = append(pairs, escapedKey+"="+url.QueryEscape(item))
		}
	}
	sort.Strings(pairs)
	return strings.ReplaceAll(strings.Join(pairs, "&"), "+", "%20")
}

func canonicalURI(path string) string {
	if path == "" {
		return "/"
	}
	return path
}

func buildStringToSign(amzDate, scope, canonicalRequest string) string {
	sum := sha256.Sum256([]byte(canonicalRequest))
	return strings.Join([]string{
		algorithm,
		amzDate,
		scope,
		hex.EncodeToString(sum[:]),
	}, "\n")
}

func deriveSigningKey(secretKey, date, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secretKey), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	return hmacSHA256(kService, "aws4_request")
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(data))
	return mac.Sum(nil)
}
