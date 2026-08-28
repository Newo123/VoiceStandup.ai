package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultConnectTimeout = 5 * time.Second

// Config содержит настройки, необходимые для запуска приложения.
type Config struct {
	Postgres Postgres
	Redis    Redis
}

type Postgres struct {
	URL            string
	ConnectTimeout time.Duration
}

type Redis struct {
	Address        string
	Password       string
	DB             int
	ConnectTimeout time.Duration
}

// Load читает конфигурацию из переменных окружения и проверяет настройки
// инфраструктуры, необходимые приложению.
func Load() (Config, error) {
	redisDB, err := envInt("REDIS_DB", 0)
	if err != nil {
		return Config{}, err
	}

	postgresTimeout, err := envDuration("POSTGRES_CONNECT_TIMEOUT", defaultConnectTimeout)
	if err != nil {
		return Config{}, err
	}

	redisTimeout, err := envDuration("REDIS_CONNECT_TIMEOUT", defaultConnectTimeout)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Postgres: Postgres{
			URL:            strings.TrimSpace(os.Getenv("DATABASE_URL")),
			ConnectTimeout: postgresTimeout,
		},
		Redis: Redis{
			Address:        envString("REDIS_ADDR", "localhost:6379"),
			Password:       os.Getenv("REDIS_PASSWORD"),
			DB:             redisDB,
			ConnectTimeout: redisTimeout,
		},
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Validate() error {
	if c.Postgres.URL == "" {
		return errors.New("DATABASE_URL is required")
	}
	if c.Redis.Address == "" {
		return errors.New("REDIS_ADDR is required")
	}
	if c.Redis.Password == "" {
		return errors.New("REDIS_PASSWORD is required")
	}
	if c.Postgres.ConnectTimeout <= 0 {
		return errors.New("POSTGRES_CONNECT_TIMEOUT must be positive")
	}
	if c.Redis.ConnectTimeout <= 0 {
		return errors.New("REDIS_CONNECT_TIMEOUT must be positive")
	}
	if c.Redis.DB < 0 {
		return errors.New("REDIS_DB must not be negative")
	}
	return nil
}

func envString(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envInt(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}

func envDuration(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}
