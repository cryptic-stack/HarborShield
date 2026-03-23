package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"harborshield/backend/internal/auth"
	metricspkg "harborshield/backend/internal/metrics"
)

type contextKey string

const claimsKey contextKey = "claims"

type AdminTokenAuthenticator interface {
	Authenticate(ctx context.Context, token string) (*auth.Claims, error)
}

func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := chimiddleware.GetReqID(r.Context())
			start := time.Now()
			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.Info("request",
				slog.String("timestamp", time.Now().UTC().Format(time.RFC3339)),
				slog.String("service", "api"),
				slog.String("request_id", requestID),
				slog.String("actor", r.Header.Get("X-Actor")),
				slog.String("action", r.Method+" "+r.URL.Path),
				slog.String("outcome", http.StatusText(ww.Status())),
				slog.Int64("latency_ms", time.Since(start).Milliseconds()),
			)
		})
	}
}

func Metrics(reg *metricspkg.Registry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			reg.RequestCount.WithLabelValues(r.URL.Path, r.Method, http.StatusText(ww.Status())).Inc()
			reg.RequestLatency.WithLabelValues(r.URL.Path, r.Method).Observe(time.Since(start).Seconds())
		})
	}
}

func AdminAuth(tokens auth.TokenManager, adminTokens AdminTokenAuthenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authz := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if authz == "" {
				WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing bearer token"})
				return
			}
			claims, err := tokens.Parse(authz)
			if err != nil && adminTokens != nil {
				claims, err = adminTokens.Authenticate(r.Context(), authz)
			}
			if err != nil {
				WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
				return
			}
			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func AdminIPAllowlist(allowlist []string) func(http.Handler) http.Handler {
	if len(allowlist) == 0 {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	allowed := make([]string, 0, len(allowlist))
	for _, entry := range allowlist {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			allowed = append(allowed, entry)
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := requestIP(r)
			if clientIP == "" || !ipAllowed(clientIP, allowed) {
				WriteJSON(w, http.StatusForbidden, map[string]string{"error": "admin access denied from this network"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func requestIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func ipAllowed(clientIP string, allowlist []string) bool {
	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}
	for _, entry := range allowlist {
		if entry == clientIP {
			return true
		}
		if parsed := net.ParseIP(entry); parsed != nil && parsed.Equal(ip) {
			return true
		}
		if _, network, err := net.ParseCIDR(entry); err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

func ClaimsFromContext(ctx context.Context) *auth.Claims {
	claims, _ := ctx.Value(claimsKey).(*auth.Claims)
	return claims
}

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
