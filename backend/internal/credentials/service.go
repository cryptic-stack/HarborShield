package credentials

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"harborshield/backend/internal/auth"
	cryptopkg "harborshield/backend/internal/crypto"
)

type Service struct {
	db     *pgxpool.Pool
	sealer *cryptopkg.Sealer
}

func New(db *pgxpool.Pool, base64Key string) (*Service, error) {
	sealer, err := cryptopkg.NewSealer(base64Key)
	if err != nil {
		return nil, err
	}
	return &Service{db: db, sealer: sealer}, nil
}

func (s *Service) Create(ctx context.Context, userID, role, description string) (map[string]any, error) {
	accessKey, err := randomKey(12)
	if err != nil {
		return nil, err
	}
	secretKey, err := randomKey(24)
	if err != nil {
		return nil, err
	}
	secretCiphertext, err := s.sealer.SealString(secretKey)
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO credentials (user_id, access_key, secret_hash, secret_ciphertext, description, role)
		VALUES (NULLIF($1, '')::uuid, $2, $3, $4, $5, $6)
	`, userID, accessKey, auth.HashSecret(secretKey), secretCiphertext, description, role)
	if err != nil {
		return nil, fmt.Errorf("create credential: %w", err)
	}
	return map[string]any{
		"accessKey":   accessKey,
		"secretKey":   secretKey,
		"role":        role,
		"description": description,
		"createdAt":   time.Now().UTC(),
	}, nil
}

func (s *Service) Validate(ctx context.Context, accessKey, secretKey string) (map[string]string, error) {
	var userID, secretHash, role string
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(user_id::text, ''), secret_hash, role
		FROM credentials WHERE access_key = $1
	`, accessKey).Scan(&userID, &secretHash, &role)
	if err != nil {
		return nil, err
	}
	if !auth.ConstantTimeEqual(secretHash, auth.HashSecret(secretKey)) {
		return nil, fmt.Errorf("invalid secret")
	}
	_, _ = s.db.Exec(ctx, `UPDATE credentials SET last_used_at = NOW() WHERE access_key = $1`, accessKey)
	return map[string]string{"userID": userID, "role": role}, nil
}

func (s *Service) Lookup(ctx context.Context, accessKey string) (map[string]string, error) {
	var userID, secretHash, role, secretCiphertext string
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(user_id::text, ''), secret_hash, role, secret_ciphertext
		FROM credentials WHERE access_key = $1
	`, accessKey).Scan(&userID, &secretHash, &role, &secretCiphertext)
	if err != nil {
		return nil, err
	}
	secretPlaintext := ""
	if secretCiphertext != "" {
		secretPlaintext, err = s.sealer.OpenString(secretCiphertext)
		if err != nil {
			return nil, err
		}
	}
	return map[string]string{"userID": userID, "role": role, "secretHash": secretHash, "secretKey": secretPlaintext}, nil
}

func (s *Service) List(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.db.Query(ctx, `
		SELECT access_key, COALESCE(user_id::text, ''), role, description, COALESCE(last_used_at::text, ''), created_at::text
		FROM credentials
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []map[string]any
	for rows.Next() {
		var accessKey, userID, role, description, lastUsedAt, createdAt string
		if err := rows.Scan(&accessKey, &userID, &role, &description, &lastUsedAt, &createdAt); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"accessKey":   accessKey,
			"userId":      userID,
			"role":        role,
			"description": description,
			"lastUsedAt":  lastUsedAt,
			"createdAt":   createdAt,
		})
	}
	return items, rows.Err()
}

func (s *Service) UpdateRole(ctx context.Context, accessKey, role string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE credentials
		SET role = $2
		WHERE access_key = $1
	`, accessKey, role)
	return err
}

func randomKey(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(bytes), nil
}
