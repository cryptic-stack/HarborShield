package adminapi

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"harborshield/backend/internal/auth"
)

var ErrInvalidToken = errors.New("invalid admin api token")

type Service struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) Create(ctx context.Context, createdByUserID, role, description string, expiresAt time.Time) (map[string]any, error) {
	tokenPlain, err := randomToken()
	if err != nil {
		return nil, err
	}
	tokenHash := auth.HashSecret(tokenPlain)
	var id, createdAtText, expiresAtText string
	err = s.db.QueryRow(ctx, `
		INSERT INTO admin_api_tokens (created_by_user_id, role, token_hash, description, expires_at)
		VALUES (NULLIF($1, '')::uuid, $2, $3, $4, $5)
		RETURNING id::text, created_at::text, expires_at::text
	`, createdByUserID, role, tokenHash, description, expiresAt.UTC()).
		Scan(&id, &createdAtText, &expiresAtText)
	if err != nil {
		return nil, fmt.Errorf("create admin api token: %w", err)
	}
	return map[string]any{
		"id":          id,
		"token":       tokenPlain,
		"role":        role,
		"description": description,
		"createdAt":   createdAtText,
		"expiresAt":   expiresAtText,
	}, nil
}

func (s *Service) List(ctx context.Context, limit int) ([]map[string]any, error) {
	rows, err := s.db.Query(ctx, `
		SELECT t.id::text, COALESCE(u.email, ''), t.role, t.description, COALESCE(t.last_used_at::text, ''), t.expires_at::text, t.created_at::text
		FROM admin_api_tokens t
		LEFT JOIN users u ON u.id = t.created_by_user_id
		ORDER BY t.created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []map[string]any
	for rows.Next() {
		var id, email, role, description, lastUsedAt, expiresAt, createdAt string
		if err := rows.Scan(&id, &email, &role, &description, &lastUsedAt, &expiresAt, &createdAt); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id":           id,
			"creatorEmail": email,
			"role":         role,
			"description":  description,
			"lastUsedAt":   lastUsedAt,
			"expiresAt":    expiresAt,
			"createdAt":    createdAt,
		})
	}
	return items, rows.Err()
}

func (s *Service) Authenticate(ctx context.Context, token string) (*auth.Claims, error) {
	tokenHash := auth.HashSecret(token)
	var id, userID, email, role string
	err := s.db.QueryRow(ctx, `
		SELECT t.id::text, COALESCE(t.created_by_user_id::text, ''), COALESCE(u.email, ''), t.role
		FROM admin_api_tokens t
		LEFT JOIN users u ON u.id = t.created_by_user_id
		WHERE t.token_hash = $1 AND t.expires_at > NOW() AND t.revoked_at IS NULL
	`, tokenHash).Scan(&id, &userID, &email, &role)
	if err != nil {
		return nil, ErrInvalidToken
	}
	_, _ = s.db.Exec(ctx, `UPDATE admin_api_tokens SET last_used_at = NOW() WHERE token_hash = $1`, tokenHash)
	return &auth.Claims{
		UserID:      userID,
		Role:        role,
		Email:       email,
		SubjectType: "admin_token",
		SubjectID:   id,
	}, nil
}

func randomToken() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("random token: %w", err)
	}
	return "hsat_" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)), nil
}
