package storage

import (
	"context"
	"fmt"
	"thesis/internal/config"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)

type DatabaseClient struct {
}

// Connect Database returning pool
func (dbc *DatabaseClient) Connect(ctx context.Context, storage config.Storage) (*pgxpool.Pool, error) {
	connStr := storage.GetDBConnString()

	var pool *pgxpool.Pool
	var err error

	for i := 0; i < 5; i++ {
		pool, err = pgxpool.Connect(ctx, connStr)
		if err == nil {
			if err = pool.Ping(ctx); err == nil {
				return pool, nil
			}
			pool.Close()
		}

		if i < 4 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Second * time.Duration(i+1)):
			}
		}
	}

	return nil, fmt.Errorf("failed to connect to database after 5 attempts: %w", err)
}
