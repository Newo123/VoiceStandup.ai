package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"VoiceStandup.ai/config"
	corellm "VoiceStandup.ai/internal/core/llm"
	corelog "VoiceStandup.ai/internal/core/logger"
	coreredis "VoiceStandup.ai/internal/core/redis"
	"VoiceStandup.ai/internal/core/repository/postgres"
	corestt "VoiceStandup.ai/internal/core/stt"
	"github.com/joho/godotenv"
)

func main() {
	// Инициализация логгера
	cleanup, err := corelog.SetupLogger("bot")
	if err != nil {
		log.Printf("[CRITICAL] Ошибка до инициализации логгера: %v", err)
		os.Exit(1)
	}
	defer cleanup()

	if err := run(); err != nil {
		slog.Error("Критическая ошибка при запуске приложения", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	// контекст для остановки (сигналов)
	shutdownCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	if err := godotenv.Load(); err != nil {
		log.Printf(".env file not loaded: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	db, err := postgres.New(shutdownCtx, cfg.Postgres.URL, cfg.Postgres.ConnectTimeout)
	if err != nil {
		log.Fatalf("connecting to database: %v", err)
	}
	defer db.Close()

	cache, err := coreredis.New(shutdownCtx, coreredis.Config{
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

	// Инициализация LLM-клиента (OpenRouter chat completions)
	llmClient := corellm.NewOpenRouterClient(cfg.LLM.APIKey, cfg.LLM.Model, cfg.LLM.Timeout)
	slog.Info("llm client initialized", slog.String("model", cfg.LLM.Model))

	// Инициализация STT-клиента (OpenRouter Whisper)
	sttClient := corestt.NewWhisperClient(cfg.STT.APIKey, cfg.STT.Model, cfg.STT.Timeout)
	slog.Info("stt client initialized", slog.String("model", cfg.STT.Model))

	// Инициализация бизнес-процессоров LLM
	voiceProcessor, err := corellm.NewVoiceProcessor(sttClient, llmClient)
	if err != nil {
		log.Fatalf("creating voice processor: %v", err)
	}
	_ = voiceProcessor // будет использован при обработке голосовых сообщений

	textProcessor, err := corellm.NewTextProcessor(llmClient)
	if err != nil {
		log.Fatalf("creating text processor: %v", err)
	}
	_ = textProcessor // будет использован при обработке текстовых сообщений

	slog.Info("llm and stt modules initialized successfully")

	return nil
}
