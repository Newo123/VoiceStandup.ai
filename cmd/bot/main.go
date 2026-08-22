package main

import (
	"context"
	"log"
	"os"

	"VoiceStandup.ai/internal/core/repository/postgres"
	"github.com/joho/godotenv"
)

func main() {
	ctx := context.Background()

	if err := godotenv.Load(); err != nil {
		log.Printf(".env file not loaded: %v", err)
	}

	db, err := postgres.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("connecting to database: %v", err)
	}
	defer db.Close()

	log.Println("connected to PostgreSQL database")
}
