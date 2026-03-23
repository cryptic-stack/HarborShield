package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Entry struct {
	Actor     string         `json:"actor"`
	Action    string         `json:"action"`
	Resource  string         `json:"resource"`
	Outcome   string         `json:"outcome"`
	RequestID string         `json:"requestId"`
	Detail    map[string]any `json:"detail"`
}

type ListFilter struct {
	Actor    string
	Action   string
	Outcome  string
	Category string
	Severity string
	Query    string
	Limit    int
}

type Service struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) Record(ctx context.Context, entry Entry) error {
	payload, err := json.Marshal(entry.Detail)
	if err != nil {
		return fmt.Errorf("marshal audit detail: %w", err)
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO audit_logs (actor, action, resource, outcome, request_id, detail)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, entry.Actor, entry.Action, entry.Resource, entry.Outcome, entry.RequestID, payload)
	return err
}

func (s *Service) List(ctx context.Context, filter ListFilter) ([]map[string]any, error) {
	if filter.Limit <= 0 || filter.Limit > 500 {
		filter.Limit = 100
	}

	rows, err := s.db.Query(ctx, `
		SELECT actor, action, resource, outcome, request_id, detail, created_at::text
		FROM audit_logs
		WHERE ($1 = '' OR actor ILIKE '%' || $1 || '%')
		  AND ($2 = '' OR action ILIKE '%' || $2 || '%')
		  AND ($3 = '' OR outcome = $3)
		  AND (
			$4 = ''
			OR resource ILIKE '%' || $4 || '%'
			OR request_id ILIKE '%' || $4 || '%'
			OR COALESCE(detail::text, '') ILIKE '%' || $4 || '%'
		  )
		ORDER BY created_at DESC
		LIMIT $5
	`, filter.Actor, filter.Action, filter.Outcome, filter.Query, filter.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var actor, action, resource, outcome, requestID string
		var detail []byte
		var createdAt string
		if err := rows.Scan(&actor, &action, &resource, &outcome, &requestID, &detail, &createdAt); err != nil {
			return nil, err
		}
		var payload map[string]any
		_ = json.Unmarshal(detail, &payload)
		category := classifyAuditCategory(action)
		severity := classifyAuditSeverity(action, outcome)
		if filter.Category != "" && !strings.EqualFold(filter.Category, category) {
			continue
		}
		if filter.Severity != "" && !strings.EqualFold(filter.Severity, severity) {
			continue
		}
		out = append(out, map[string]any{
			"actor":     actor,
			"action":    action,
			"resource":  resource,
			"outcome":   outcome,
			"category":  category,
			"severity":  severity,
			"requestId": requestID,
			"detail":    sanitizeAuditValue(payload),
			"createdAt": createdAt,
		})
	}
	return out, rows.Err()
}

func sanitizeAuditValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if isSensitiveAuditKey(key) {
				out[key] = "[redacted]"
				continue
			}
			out[key] = sanitizeAuditValue(item)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, sanitizeAuditValue(item))
		}
		return out
	default:
		return value
	}
}

func isSensitiveAuditKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return false
	}
	if strings.HasSuffix(key, "configured") {
		return false
	}
	sensitiveTerms := []string{"secret", "password", "token", "ciphertext", "privatekey", "clientsecret", "authorization", "cookie", "session"}
	for _, term := range sensitiveTerms {
		if strings.Contains(key, term) {
			return true
		}
	}
	return false
}

func classifyAuditCategory(action string) string {
	action = strings.ToLower(strings.TrimSpace(action))
	switch {
	case strings.HasPrefix(action, "auth."), strings.HasPrefix(action, "admin-token."):
		return "authentication"
	case strings.HasPrefix(action, "settings."):
		return "settings"
	case strings.HasPrefix(action, "storage.node"), strings.HasPrefix(action, "storage.join"), strings.HasPrefix(action, "storage."):
		return "storage"
	case strings.HasPrefix(action, "bucket."), strings.HasPrefix(action, "object."), strings.HasPrefix(action, "multipart."):
		return "data"
	case strings.HasPrefix(action, "credential."), strings.HasPrefix(action, "role."), strings.HasPrefix(action, "policy."), strings.HasPrefix(action, "user."):
		return "access-control"
	case strings.HasPrefix(action, "quota."):
		return "quota"
	case strings.HasPrefix(action, "malware."):
		return "malware"
	case strings.HasPrefix(action, "event."), strings.HasPrefix(action, "webhook."), strings.HasPrefix(action, "delivery."):
		return "eventing"
	case strings.HasPrefix(action, "setup."):
		return "deployment"
	default:
		return "system"
	}
}

func classifyAuditSeverity(action, outcome string) string {
	action = strings.ToLower(strings.TrimSpace(action))
	outcome = strings.ToLower(strings.TrimSpace(outcome))
	switch {
	case outcome == "failure":
		return "high"
	case outcome == "denied":
		return "medium"
	case strings.Contains(action, "delete"), strings.Contains(action, "clear-secret"), strings.Contains(action, "repin"), strings.Contains(action, "logout-all"):
		return "medium"
	case strings.Contains(action, "create"), strings.Contains(action, "update"), strings.Contains(action, "restore"), strings.Contains(action, "durability"), strings.Contains(action, "tagging"):
		return "low"
	default:
		return "info"
	}
}
