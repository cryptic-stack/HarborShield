package events

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"harborshield/backend/internal/audit"
	cryptopkg "harborshield/backend/internal/crypto"
)

type Service struct {
	db         *pgxpool.Pool
	sealer     *cryptopkg.Sealer
	audit      *audit.Service
	httpClient *http.Client
}

type Target struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	TargetType  string   `json:"targetType"`
	EndpointURL string   `json:"endpointUrl"`
	EventTypes  []string `json:"eventTypes"`
	Enabled     bool     `json:"enabled"`
	CreatedAt   string   `json:"createdAt"`
}

type Delivery struct {
	ID               string `json:"id"`
	TargetID         string `json:"targetId"`
	TargetName       string `json:"targetName"`
	EventType        string `json:"eventType"`
	Status           string `json:"status"`
	Attempts         int    `json:"attempts"`
	LastError        string `json:"lastError"`
	LastResponseCode *int   `json:"lastResponseCode"`
	CreatedAt        string `json:"createdAt"`
	UpdatedAt        string `json:"updatedAt"`
}

type DeliveryFilters struct {
	Status    string
	TargetID  string
	EventType string
	Limit     int
}

type deliveryJob struct {
	ID                      string
	TargetID                string
	TargetName              string
	EndpointURL             string
	SigningSecretCiphertext string
	EventType               string
	Payload                 []byte
	Attempts                int
}

func NewService(db *pgxpool.Pool, base64Key string, auditSvc *audit.Service) (*Service, error) {
	sealer, err := cryptopkg.NewSealer(base64Key)
	if err != nil {
		return nil, err
	}
	return &Service{
		db:     db,
		sealer: sealer,
		audit:  auditSvc,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}, nil
}

func (s *Service) CreateTarget(ctx context.Context, name, endpointURL, signingSecret string, eventTypes []string) (Target, error) {
	normalized := normalizeEventTypes(eventTypes)
	payload, err := json.Marshal(normalized)
	if err != nil {
		return Target{}, err
	}
	secretCiphertext := ""
	if signingSecret != "" {
		secretCiphertext, err = s.sealer.SealString(signingSecret)
		if err != nil {
			return Target{}, err
		}
	}

	var target Target
	var rawEventTypes []byte
	err = s.db.QueryRow(ctx, `
		INSERT INTO event_targets (name, target_type, endpoint_url, signing_secret_ciphertext, event_types, enabled)
		VALUES ($1, 'webhook', $2, $3, $4::jsonb, TRUE)
		RETURNING id::text, name, target_type, endpoint_url, event_types, enabled, created_at::text
	`, name, endpointURL, secretCiphertext, payload).Scan(
		&target.ID,
		&target.Name,
		&target.TargetType,
		&target.EndpointURL,
		&rawEventTypes,
		&target.Enabled,
		&target.CreatedAt,
	)
	if err != nil {
		return Target{}, err
	}
	if err := json.Unmarshal(rawEventTypes, &target.EventTypes); err != nil {
		return Target{}, err
	}
	return target, nil
}

func (s *Service) ListTargets(ctx context.Context) ([]Target, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, target_type, endpoint_url, event_types, enabled, created_at::text
		FROM event_targets
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Target{}
	for rows.Next() {
		var target Target
		var rawEventTypes []byte
		if err := rows.Scan(&target.ID, &target.Name, &target.TargetType, &target.EndpointURL, &rawEventTypes, &target.Enabled, &target.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(rawEventTypes, &target.EventTypes); err != nil {
			return nil, err
		}
		out = append(out, target)
	}
	return out, rows.Err()
}

func (s *Service) ListDeliveries(ctx context.Context, filters DeliveryFilters) ([]Delivery, error) {
	if filters.Limit <= 0 || filters.Limit > 200 {
		filters.Limit = 100
	}
	conditions := []string{}
	args := []any{}
	if filters.Status != "" {
		args = append(args, filters.Status)
		conditions = append(conditions, fmt.Sprintf("d.status = $%d", len(args)))
	}
	if filters.TargetID != "" {
		args = append(args, filters.TargetID)
		conditions = append(conditions, fmt.Sprintf("d.target_id = $%d::uuid", len(args)))
	}
	if filters.EventType != "" {
		args = append(args, filters.EventType)
		conditions = append(conditions, fmt.Sprintf("d.event_type = $%d", len(args)))
	}
	query := `
		SELECT d.id::text, d.target_id::text, t.name, d.event_type, d.status, d.attempts, d.last_error, d.last_response_code, d.created_at::text, d.updated_at::text
		FROM event_deliveries d
		JOIN event_targets t ON t.id = d.target_id
	`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	args = append(args, filters.Limit)
	query += fmt.Sprintf(" ORDER BY d.created_at DESC LIMIT $%d", len(args))
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Delivery{}
	for rows.Next() {
		var item Delivery
		if err := rows.Scan(&item.ID, &item.TargetID, &item.TargetName, &item.EventType, &item.Status, &item.Attempts, &item.LastError, &item.LastResponseCode, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) Emit(ctx context.Context, eventType string, payload map[string]any) error {
	targets, err := s.matchingTargets(ctx, eventType)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	for _, targetID := range targets {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO event_deliveries (target_id, event_type, payload, status, next_attempt_at)
			VALUES ($1::uuid, $2, $3::jsonb, 'pending', NOW())
		`, targetID, eventType, body); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) DeliverPending(ctx context.Context, limit int) (int64, error) {
	if limit <= 0 {
		limit = 10
	}
	var delivered int64
	for i := 0; i < limit; i++ {
		job, err := s.claimNextDelivery(ctx)
		if err != nil {
			if err == pgx.ErrNoRows {
				return delivered, nil
			}
			return delivered, err
		}
		if err := s.deliverOne(ctx, job); err != nil {
			return delivered, err
		}
		delivered++
	}
	return delivered, nil
}

func (s *Service) claimNextDelivery(ctx context.Context) (deliveryJob, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return deliveryJob{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var item deliveryJob
	err = tx.QueryRow(ctx, `
		WITH candidate AS (
			SELECT d.id
			FROM event_deliveries d
			JOIN event_targets t ON t.id = d.target_id
			WHERE d.status IN ('pending', 'retrying')
			  AND d.next_attempt_at <= NOW()
			  AND t.enabled = TRUE
			ORDER BY d.next_attempt_at, d.created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE event_deliveries d
		SET status = 'running', updated_at = NOW()
		FROM event_targets t
		WHERE d.id IN (SELECT id FROM candidate)
		  AND t.id = d.target_id
		RETURNING d.id::text, d.target_id::text, t.name, t.endpoint_url, t.signing_secret_ciphertext, d.event_type, d.payload, d.attempts
	`).Scan(&item.ID, &item.TargetID, &item.TargetName, &item.EndpointURL, &item.SigningSecretCiphertext, &item.EventType, &item.Payload, &item.Attempts)
	if err != nil {
		return deliveryJob{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return deliveryJob{}, err
	}
	return item, nil
}

func (s *Service) deliverOne(ctx context.Context, item deliveryJob) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, item.EndpointURL, bytes.NewReader(item.Payload))
	if err != nil {
		return s.markFailure(ctx, item, 0, err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-S3P-Event", item.EventType)
	req.Header.Set("X-S3P-Delivery-Id", item.ID)

	if item.SigningSecretCiphertext != "" {
		secret, err := s.sealer.OpenString(item.SigningSecretCiphertext)
		if err != nil {
			return s.markFailure(ctx, item, 0, err.Error())
		}
		sig := signPayload(secret, item.Payload)
		req.Header.Set("X-S3P-Signature", sig)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return s.markFailure(ctx, item, 0, err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if _, err := s.db.Exec(ctx, `
			UPDATE event_deliveries
			SET status = 'delivered', attempts = attempts + 1, last_error = '', last_response_code = $2, updated_at = NOW()
			WHERE id = $1::uuid
		`, item.ID, resp.StatusCode); err != nil {
			return err
		}
		return s.recordAudit(ctx, audit.Entry{
			Actor:    "worker",
			Action:   "event.delivery.success",
			Resource: item.TargetName,
			Outcome:  "success",
			Detail: map[string]any{
				"deliveryId": item.ID,
				"eventType":  item.EventType,
				"statusCode": resp.StatusCode,
			},
		})
	}
	return s.markFailure(ctx, item, resp.StatusCode, fmt.Sprintf("unexpected status %d", resp.StatusCode))
}

func (s *Service) markFailure(ctx context.Context, item deliveryJob, statusCode int, errMessage string) error {
	nextAttempts := item.Attempts + 1
	status := "retrying"
	nextRun := time.Now().UTC().Add(time.Duration(nextAttempts*15) * time.Second)
	if nextAttempts >= 5 {
		status = "dead_letter"
		nextRun = time.Now().UTC()
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE event_deliveries
		SET status = $2, attempts = attempts + 1, next_attempt_at = $3, last_error = $4, last_response_code = NULLIF($5, 0), updated_at = NOW()
		WHERE id = $1::uuid
	`, item.ID, status, nextRun, errMessage, statusCode); err != nil {
		return err
	}
	outcome := "failure"
	if status == "dead_letter" {
		outcome = "dead_letter"
	}
	return s.recordAudit(ctx, audit.Entry{
		Actor:    "worker",
		Action:   "event.delivery.failure",
		Resource: item.TargetName,
		Outcome:  outcome,
		Detail: map[string]any{
			"deliveryId": item.ID,
			"eventType":  item.EventType,
			"error":      errMessage,
			"statusCode": statusCode,
			"attempts":   nextAttempts,
		},
	})
}

func (s *Service) matchingTargets(ctx context.Context, eventType string) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, event_types
		FROM event_targets
		WHERE enabled = TRUE
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		var eventTypes []string
		if err := json.Unmarshal(raw, &eventTypes); err != nil {
			return nil, err
		}
		if matchesEventType(eventTypes, eventType) {
			out = append(out, id)
		}
	}
	return out, rows.Err()
}

func normalizeEventTypes(items []string) []string {
	if len(items) == 0 {
		return []string{"*"}
	}
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return []string{"*"}
	}
	return out
}

func matchesEventType(items []string, eventType string) bool {
	for _, item := range items {
		if item == "*" || item == eventType {
			return true
		}
	}
	return false
}

func signPayload(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) recordAudit(ctx context.Context, entry audit.Entry) error {
	if s.audit == nil {
		return nil
	}
	return s.audit.Record(ctx, entry)
}
