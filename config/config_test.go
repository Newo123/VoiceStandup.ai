package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadInfrastructureConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://user:password@localhost:5432/app")
	t.Setenv("POSTGRES_CONNECT_TIMEOUT", "3s")
	t.Setenv("REDIS_ADDR", "redis:6379")
	t.Setenv("REDIS_PASSWORD", "secret")
	t.Setenv("REDIS_DB", "2")
	t.Setenv("REDIS_CONNECT_TIMEOUT", "4s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Postgres.ConnectTimeout != 3*time.Second {
		t.Fatalf("Postgres.ConnectTimeout = %v", cfg.Postgres.ConnectTimeout)
	}
	if cfg.Redis.Address != "redis:6379" {
		t.Fatalf("Redis.Address = %q", cfg.Redis.Address)
	}
	if cfg.Redis.Password != "secret" {
		t.Fatal("Redis.Password was not loaded")
	}
	if cfg.Redis.DB != 2 {
		t.Fatalf("Redis.DB = %d", cfg.Redis.DB)
	}
	if cfg.Redis.ConnectTimeout != 4*time.Second {
		t.Fatalf("Redis.ConnectTimeout = %v", cfg.Redis.ConnectTimeout)
	}
}

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://user:password@localhost:5432/app")
	t.Setenv("POSTGRES_CONNECT_TIMEOUT", "")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("REDIS_PASSWORD", "secret")
	t.Setenv("REDIS_DB", "")
	t.Setenv("REDIS_CONNECT_TIMEOUT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Postgres.ConnectTimeout != defaultConnectTimeout {
		t.Fatalf("Postgres.ConnectTimeout = %v", cfg.Postgres.ConnectTimeout)
	}
	if cfg.Redis.Address != "localhost:6379" {
		t.Fatalf("Redis.Address = %q", cfg.Redis.Address)
	}
	if cfg.Redis.DB != 0 {
		t.Fatalf("Redis.DB = %d", cfg.Redis.DB)
	}
}

func TestLoadRejectsMissingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadRejectsInvalidRedisDB(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://user:password@localhost:5432/app")
	t.Setenv("REDIS_DB", "invalid")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "REDIS_DB") {
		t.Fatalf("Load() error = %v", err)
	}
}
