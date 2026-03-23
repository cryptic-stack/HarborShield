package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres pool: %w", err)
	}
	return pool, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migration dir: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		content, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), readErr)
		}
		if _, execErr := pool.Exec(ctx, string(content)); execErr != nil {
			return fmt.Errorf("run migration %s: %w", entry.Name(), execErr)
		}
	}
	return nil
}
