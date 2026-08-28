package main

import (
	"context"
	"log"

	"VoiceStandup.ai/config"
	coreredis "VoiceStandup.ai/internal/core/redis"
	"VoiceStandup.ai/internal/core/repository/postgres"
	"github.com/joho/godotenv"
)

func main() {
	ctx := context.Background()

	if err := godotenv.Load(); err != nil {
		log.Printf(".env file not loaded: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	db, err := postgres.New(ctx, cfg.Postgres.URL, cfg.Postgres.ConnectTimeout)
	if err != nil {
		log.Fatalf("connecting to database: %v", err)
	}
	defer db.Close()

	cache, err := coreredis.New(ctx, coreredis.Config{
		Address:        cfg.Redis.Address,
		Password:       cfg.Redis.Password,
		DB:             cfg.Redis.DB,
		ConnectTimeout: cfg.Redis.ConnectTimeout,
	})
	if err != nil {
		log.Fatalf("connecting to redis: %v", err)
	}
	defer func() {
		if err := cache.Close(); err != nil {
			log.Printf("closing redis: %v", err)
		}
	}()

	log.Println("connected to PostgreSQL and Redis")
}
