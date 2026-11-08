package store

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"searchtags/internal/config"
)

type ClickHouse struct {
	conn driver.Conn
}

// NewClickHouse открывает соединение и проверяет его пингом.
func NewClickHouse(ctx context.Context, cfg config.Config) (*ClickHouse, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.ClickHouseAddr},
		Auth: clickhouse.Auth{
			Database: cfg.ClickHouseDB,
			Username: cfg.ClickHouseUser,
			Password: cfg.ClickHousePassword,
		},
		DialTimeout: 5 * time.Second,
		ReadTimeout: cfg.QueryTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("open clickhouse: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := conn.Ping(pingCtx); err != nil {
		return nil, fmt.Errorf("ping clickhouse: %w", err)
	}
	return &ClickHouse{conn: conn}, nil
}

func (c *ClickHouse) Close() error { return c.conn.Close() }

// Tags читает теги запроса. Значение q передаётся именованным параметром, а не склеивается в строку.
func (c *ClickHouse) Tags(ctx context.Context, q string) ([]Tag, error) {
	rows, err := c.conn.Query(ctx,
		`SELECT tag, users FROM tags WHERE query = @q ORDER BY rank`,
		clickhouse.Named("q", q),
	)
	if err != nil {
		return nil, fmt.Errorf("query tags: %w", err)
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.Tag, &t.Users); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tags: %w", err)
	}
	return tags, nil
}
