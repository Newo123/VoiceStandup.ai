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

const (
	defaultHTTPAddress         = ":8080"
	defaultHTTPShutdownTimeout = 5 * time.Second
	defaultTelegramAuthMaxAge  = 5 * time.Minute
)

// Config содержит настройки, необходимые для запуска приложения.
type Config struct {
	Postgres Postgres
	Redis    Redis
	LLM      LLM
	STT      STT
	Telegram Telegram
	HTTP     HTTP
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

// LLM содержит настройки для OpenRouter chat completions API.
type LLM struct {
	APIKey  string
	Model   string
	Timeout time.Duration
}

// STT содержит настройки для OpenRouter audio transcriptions API.
type STT struct {
	APIKey  string
	Model   string
	Timeout time.Duration
}

// Telegram содержит настройки для Telegram Bot API.
type Telegram struct {
	BotToken string
}

type HTTP struct {
	Address         string
	ShutdownTimeout time.Duration
	TelegramAuthAge time.Duration
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

	llmTimeout, err := envDuration("LLM_TIMEOUT", 30*time.Second)
	if err != nil {
		return Config{}, err
	}

	sttTimeout, err := envDuration("STT_TIMEOUT", 60*time.Second)
	if err != nil {
		return Config{}, err
	}

	httpShutdownTimeout, err := envDuration("HTTP_SHUTDOWN_TIMEOUT", defaultHTTPShutdownTimeout)
	if err != nil {
		return Config{}, err
	}

	telegramAuthAge, err := envDuration("TELEGRAM_AUTH_MAX_AGE", defaultTelegramAuthMaxAge)
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
		LLM: LLM{
			APIKey:  os.Getenv("LLM_API_KEY"),
			Model:   envString("LLM_MODEL", "openai/gpt-4o-mini"),
			Timeout: llmTimeout,
		},
		STT: STT{
			// STT использует тот же API-ключ, что и LLM (OpenRouter)
			APIKey:  os.Getenv("LLM_API_KEY"),
			Model:   envString("STT_MODEL", "openai/whisper-large-v3-turbo"),
			Timeout: sttTimeout,
		},
		Telegram: Telegram{
			BotToken: os.Getenv("BOT_TOKEN"),
		},
		HTTP: HTTP{
			Address:         envString("HTTP_ADDR", defaultHTTPAddress),
			ShutdownTimeout: httpShutdownTimeout,
			TelegramAuthAge: telegramAuthAge,
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
	if c.LLM.APIKey == "" {
		return errors.New("LLM_API_KEY is required")
	}
	if c.STT.APIKey == "" {
		return errors.New("LLM_API_KEY is required")
	}
	if c.Telegram.BotToken == "" {
		return errors.New("BOT_TOKEN is required")
	}
	if c.HTTP.Address == "" {
		return errors.New("HTTP_ADDR is required")
	}
	if c.HTTP.ShutdownTimeout <= 0 {
		return errors.New("HTTP_SHUTDOWN_TIMEOUT must be positive")
	}
	if c.HTTP.TelegramAuthAge <= 0 {
		return errors.New("TELEGRAM_AUTH_MAX_AGE must be positive")
	}
	if c.LLM.Timeout <= 0 {
		return errors.New("LLM_TIMEOUT must be positive")
	}
	if c.STT.Timeout <= 0 {
		return errors.New("STT_TIMEOUT must be positive")
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
