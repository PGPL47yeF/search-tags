// Package config читает настройки сервиса из переменных окружения.
package config

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

type Config struct {
	HTTPPort       int
	RequestTimeout time.Duration

	ClickHouseAddr     string
	ClickHouseDB       string
	ClickHouseUser     string
	ClickHousePassword string
}

// FromEnv собирает конфиг. lookup обычно os.LookupEnv, в тестах — подмена.
func FromEnv(lookup func(string) (string, bool)) (Config, error) {
	get := func(key, fallback string) string {
		if v, ok := lookup(key); ok && v != "" {
			return v
		}
		return fallback
	}

	cfg := Config{
		ClickHouseAddr:     get("CLICKHOUSE_ADDR", "localhost:9000"),
		ClickHouseDB:       get("CLICKHOUSE_DB", "search"),
		ClickHouseUser:     get("CLICKHOUSE_USER", "default"),
		ClickHousePassword: get("CLICKHOUSE_PASSWORD", ""),
	}

	var errs []error

	port, err := strconv.Atoi(get("HTTP_PORT", "8080"))
	switch {
	case err != nil:
		errs = append(errs, fmt.Errorf("HTTP_PORT: %w", err))
	case port < 1 || port > 65535:
		errs = append(errs, fmt.Errorf("HTTP_PORT: %d is out of range 1-65535", port))
	}
	cfg.HTTPPort = port

	timeout, err := time.ParseDuration(get("REQUEST_TIMEOUT", "2s"))
	switch {
	case err != nil:
		errs = append(errs, fmt.Errorf("REQUEST_TIMEOUT: %w", err))
	case timeout <= 0:
		errs = append(errs, fmt.Errorf("REQUEST_TIMEOUT: must be positive, got %s", timeout))
	}
	cfg.RequestTimeout = timeout

	return cfg, errors.Join(errs...)
}

// ListenAddr — адрес для http.Server.
func (c Config) ListenAddr() string {
	return fmt.Sprintf(":%d", c.HTTPPort)
}
