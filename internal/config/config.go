// Package config читает настройки сервиса из переменных окружения.
package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	ClickHouseAddr     string // host:port нативного протокола
	ClickHouseDB       string
	ClickHouseUser     string
	ClickHousePassword string
	HTTPPort           string
	QueryTimeout       time.Duration
}

// FromEnv собирает конфиг. Значения по умолчанию подходят под docker-compose.yml из репозитория.
func FromEnv() (Config, error) {
	timeout, err := time.ParseDuration(getenv("QUERY_TIMEOUT", "3s"))
	if err != nil {
		return Config{}, fmt.Errorf("QUERY_TIMEOUT: %w", err)
	}
	return Config{
		ClickHouseAddr:     getenv("CLICKHOUSE_ADDR", "localhost:9000"),
		ClickHouseDB:       getenv("CLICKHOUSE_DB", "tags_db"),
		ClickHouseUser:     getenv("CLICKHOUSE_USER", "app"),
		ClickHousePassword: getenv("CLICKHOUSE_PASSWORD", "app"),
		HTTPPort:           getenv("HTTP_PORT", "8080"),
		QueryTimeout:       timeout,
	}, nil
}

func getenv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}
