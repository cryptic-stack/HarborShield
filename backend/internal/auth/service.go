package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials    = errors.New("invalid credentials")
	invalidLoginPasswordHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
)

type TokenManager struct {
	secret          []byte
	issuer          string
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

type Claims struct {
	UserID      string `json:"userId"`
	Role        string `json:"role"`
	Email       string `json:"email"`
	SubjectType string `json:"subjectType"`
	SubjectID   string `json:"subjectId"`
	jwt.RegisteredClaims
}

type Service struct {
	db     *pgxpool.Pool
	tokens TokenManager
}

func New(db *pgxpool.Pool, secret, issuer string, accessTokenTTL, refreshTokenTTL time.Duration) *Service {
	return &Service{
		db: db,
		tokens: TokenManager{
			secret:          []byte(secret),
			issuer:          issuer,
			accessTokenTTL:  accessTokenTTL,
			refreshTokenTTL: refreshTokenTTL,
		},
	}
}

func (s *Service) Tokens() TokenManager {
	return s.tokens
}

func HashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

func ComparePassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func HashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (s *Service) EnsureBootstrapAdmin(ctx context.Context, email, password string) error {
	var exists bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE role = 'superadmin')`).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO users (email, password_hash, role, must_change_password)
		VALUES ($1, $2, 'superadmin', TRUE)
	`, email, hash)
	return err
}

func (s *Service) Login(ctx context.Context, email, password string) (map[string]any, error) {
	var id, hash, role, authProvider string
	var mustChange bool
	err := s.db.QueryRow(ctx, `
		SELECT id::text, password_hash, role, must_change_password, auth_provider
		FROM users WHERE email = $1
	`, email).Scan(&id, &hash, &role, &mustChange, &authProvider)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			_ = ComparePassword(invalidLoginPasswordHash, password)
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if authProvider != "local" {
		_ = ComparePassword(invalidLoginPasswordHash, password)
		return nil, ErrInvalidCredentials
	}
	if err := ComparePassword(hash, password); err != nil {
		return nil, ErrInvalidCredentials
	}
	return s.issueSession(ctx, id, email, role, mustChange)
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (map[string]any, error) {
	refreshHash := HashSecret(refreshToken)
	var userID, email, role string
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	err = tx.QueryRow(ctx, `
		SELECT u.id::text, u.email, u.role
		FROM refresh_tokens rt
		JOIN users u ON u.id = rt.user_id
		WHERE rt.token_hash = $1 AND rt.expires_at > NOW()
	`, refreshHash).Scan(&userID, &email, &role)
	if err != nil {
		return nil, errors.New("invalid refresh token")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM refresh_tokens WHERE token_hash = $1`, refreshHash); err != nil {
		return nil, err
	}
	session, err := s.issueSessionWithQuerier(ctx, tx, userID, email, role, false)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return map[string]any{
		"accessToken":  session["accessToken"],
		"refreshToken": session["refreshToken"],
	}, nil
}

func (s *Service) ChangePassword(ctx context.Context, userID, currentPassword, nextPassword string) error {
	if len(nextPassword) < 12 {
		return errors.New("new password must be at least 12 characters")
	}

	var currentHash string
	if err := s.db.QueryRow(ctx, `
		SELECT password_hash
		FROM users
		WHERE id = $1::uuid
	`, userID).Scan(&currentHash); err != nil {
		return errors.New("user not found")
	}

	if err := ComparePassword(currentHash, currentPassword); err != nil {
		return errors.New("current password is incorrect")
	}

	nextHash, err := HashPassword(nextPassword)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(ctx, `
		UPDATE users
		SET password_hash = $2, must_change_password = FALSE, auth_provider = 'local', updated_at = NOW()
		WHERE id = $1::uuid
	`, userID, nextHash)
	return err
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	refreshHash := HashSecret(refreshToken)
	_, err := s.db.Exec(ctx, `DELETE FROM refresh_tokens WHERE token_hash = $1`, refreshHash)
	return err
}

func (s *Service) LogoutAll(ctx context.Context, userID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1::uuid`, userID)
	return err
}

func (s *Service) LoginOIDC(ctx context.Context, issuer, subject, email string, emailVerified bool, mappedRole string) (map[string]any, error) {
	if subject == "" || email == "" {
		return nil, errors.New("oidc subject and email are required")
	}
	if mappedRole == "" {
		mappedRole = "admin"
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var id, role string
	var mustChange bool
	err = tx.QueryRow(ctx, `
		SELECT id::text, role, must_change_password
		FROM users
		WHERE auth_provider = $1 AND external_subject = $2
	`, issuer, subject).Scan(&id, &role, &mustChange)
	switch {
	case err == nil:
		if mappedRole != "" && mappedRole != role {
			if _, err := tx.Exec(ctx, `
				UPDATE users
				SET role = $2, updated_at = NOW()
				WHERE id = $1::uuid
			`, id, mappedRole); err != nil {
				return nil, err
			}
			role = mappedRole
		}
	case errors.Is(err, pgx.ErrNoRows):
		err = tx.QueryRow(ctx, `
			SELECT id::text, role, must_change_password
			FROM users
			WHERE email = $1
		`, email).Scan(&id, &role, &mustChange)
		switch {
		case err == nil:
			if _, err := tx.Exec(ctx, `
				UPDATE users
				SET auth_provider = $2, external_subject = $3, role = $4, updated_at = NOW()
				WHERE id = $1::uuid
			`, id, issuer, subject, mappedRole); err != nil {
				return nil, err
			}
			role = mappedRole
		case errors.Is(err, pgx.ErrNoRows):
			passwordHash, hashErr := HashPassword(uuid.NewString())
			if hashErr != nil {
				return nil, hashErr
			}
			err = tx.QueryRow(ctx, `
				INSERT INTO users (email, password_hash, role, must_change_password, auth_provider, external_subject)
				VALUES ($1, $2, $3, FALSE, $4, $5)
				RETURNING id::text, role, must_change_password
			`, email, passwordHash, mappedRole, issuer, subject).Scan(&id, &role, &mustChange)
			if err != nil {
				return nil, err
			}
		default:
			return nil, err
		}
	default:
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	_ = emailVerified
	return s.issueSession(ctx, id, email, role, mustChange)
}

func (s *Service) issueSession(ctx context.Context, userID, email, role string, mustChange bool) (map[string]any, error) {
	return s.issueSessionWithQuerier(ctx, s.db, userID, email, role, mustChange)
}

type dbQuerier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func (s *Service) issueSessionWithQuerier(ctx context.Context, querier dbQuerier, userID, email, role string, mustChange bool) (map[string]any, error) {
	accessToken, err := s.tokens.signToken(userID, email, role, s.tokens.accessTokenTTL)
	if err != nil {
		return nil, err
	}
	refreshPlain := uuid.NewString()
	refreshHash := HashSecret(refreshPlain)
	_, err = querier.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, refreshHash, time.Now().UTC().Add(s.tokens.refreshTokenTTL))
	if err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	return map[string]any{
		"accessToken":        accessToken,
		"refreshToken":       refreshPlain,
		"mustChangePassword": mustChange,
		"user": map[string]any{
			"id":    userID,
			"email": email,
			"role":  role,
		},
	}, nil
}

func (tm TokenManager) signToken(userID, email, role string, ttl time.Duration) (string, error) {
	claims := Claims{
		UserID:      userID,
		Role:        role,
		Email:       email,
		SubjectType: "user",
		SubjectID:   userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    tm.issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(tm.secret)
}

func (tm TokenManager) Parse(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		return tm.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
