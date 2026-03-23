package users

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) List(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, email, role, must_change_password, created_at::text
		FROM users
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, email, role, createdAt string
		var mustChange bool
		if err := rows.Scan(&id, &email, &role, &mustChange, &createdAt); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id":                 id,
			"email":              email,
			"role":               role,
			"mustChangePassword": mustChange,
			"createdAt":          createdAt,
		})
	}
	return out, rows.Err()
}

func (s *Service) UpdateRole(ctx context.Context, userID, role string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE users
		SET role = $2, updated_at = NOW()
		WHERE id = $1::uuid
	`, userID, role)
	return err
}
