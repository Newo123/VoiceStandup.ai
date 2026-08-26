package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	redisv9 "github.com/redis/go-redis/v9"
)

type Config struct {
	Address        string
	Password       string
	DB             int
	ConnectTimeout time.Duration
}

func New(ctx context.Context, cfg Config) (*redisv9.Client, error) {
	if cfg.Address == "" {
		return nil, errors.New("redis address is required")
	}
	if cfg.DB < 0 {
		return nil, errors.New("redis db must not be negative")
	}
	if cfg.ConnectTimeout <= 0 {
		return nil, errors.New("redis connect timeout must be positive")
	}

	client := redisv9.NewClient(&redisv9.Options{
		Addr:        cfg.Address,
		Password:    cfg.Password,
		DB:          cfg.DB,
		DialTimeout: cfg.ConnectTimeout,
	})

	connectCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()

	if err := client.Ping(connectCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("could not ping redis: %w", err)
	}

	return client, nil
}
