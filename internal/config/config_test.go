package config

import (
	"strings"
	"testing"
	"time"
)

func env(vars map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := vars[key]
		return v, ok
	}
}

func TestDefaults(t *testing.T) {
	cfg, err := FromEnv(env(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPPort != 8080 || cfg.RequestTimeout != 2*time.Second {
		t.Errorf("port/timeout = %d/%s, want 8080/2s", cfg.HTTPPort, cfg.RequestTimeout)
	}
	if cfg.ClickHouseAddr != "localhost:9000" || cfg.ClickHouseDB != "search" || cfg.ClickHouseUser != "default" {
		t.Errorf("clickhouse defaults = %+v", cfg)
	}
	if cfg.ListenAddr() != ":8080" {
		t.Errorf("ListenAddr() = %q", cfg.ListenAddr())
	}
}

func TestOverrides(t *testing.T) {
	cfg, err := FromEnv(env(map[string]string{
		"HTTP_PORT":           "9090",
		"REQUEST_TIMEOUT":     "750ms",
		"CLICKHOUSE_ADDR":     "ch:9000",
		"CLICKHOUSE_DB":       "prod",
		"CLICKHOUSE_USER":     "tags",
		"CLICKHOUSE_PASSWORD": "secret",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Config{9090, 750 * time.Millisecond, "ch:9000", "prod", "tags", "secret"}
	if cfg != want {
		t.Errorf("got %+v, want %+v", cfg, want)
	}
}

func TestEmptyValueMeansDefault(t *testing.T) {
	cfg, err := FromEnv(env(map[string]string{"HTTP_PORT": "", "CLICKHOUSE_DB": ""}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPPort != 8080 || cfg.ClickHouseDB != "search" {
		t.Errorf("got %+v", cfg)
	}
}

func TestInvalidValues(t *testing.T) {
	tests := map[string]map[string]string{
		"порт не число":      {"HTTP_PORT": "http"},
		"порт вне диапазона": {"HTTP_PORT": "70000"},
		"нулевой порт":       {"HTTP_PORT": "0"},
		"таймаут без единиц": {"REQUEST_TIMEOUT": "5"},
		"отрицательный":      {"REQUEST_TIMEOUT": "-1s"},
		"нулевой таймаут":    {"REQUEST_TIMEOUT": "0s"},
	}
	for name, vars := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := FromEnv(env(vars)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestAllErrorsReported(t *testing.T) {
	_, err := FromEnv(env(map[string]string{"HTTP_PORT": "x", "REQUEST_TIMEOUT": "y"}))
	if err == nil || !strings.Contains(err.Error(), "HTTP_PORT") || !strings.Contains(err.Error(), "REQUEST_TIMEOUT") {
		t.Fatalf("want both variables in error, got %v", err)
	}
}
