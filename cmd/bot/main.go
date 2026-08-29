package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"VoiceStandup.ai/config"
	corellm "VoiceStandup.ai/internal/core/llm"
	corelog "VoiceStandup.ai/internal/core/logger"
	coreredis "VoiceStandup.ai/internal/core/redis"
	"VoiceStandup.ai/internal/core/repository/postgres"
	corestt "VoiceStandup.ai/internal/core/stt"
	coretelegram "VoiceStandup.ai/internal/core/transport/telegram"
	"VoiceStandup.ai/internal/standup/confirmation"
	"VoiceStandup.ai/internal/standup/delayed_publish"
	"VoiceStandup.ai/internal/standup/digest"
	"VoiceStandup.ai/internal/standup/gamification"
	"VoiceStandup.ai/internal/standup/miniapp"
	"VoiceStandup.ai/internal/standup/onboarding"
	"VoiceStandup.ai/internal/standup/parser"
	"VoiceStandup.ai/internal/standup/repository"
	"VoiceStandup.ai/internal/transport/bot"
	httptransport "VoiceStandup.ai/internal/transport/http"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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

	textProcessor, err := corellm.NewTextProcessor(llmClient)
	if err != nil {
		log.Fatalf("creating text processor: %v", err)
	}

	slog.Info("llm and stt modules initialized successfully")

	// Инициализация Telegram Bot API
	tgBot, err := tgbotapi.NewBotAPI(cfg.Telegram.BotToken)
	if err != nil {
		log.Fatalf("creating telegram bot: %v", err)
	}
	tgBot.Debug = false
	slog.Info("telegram bot authorized", slog.String("username", tgBot.Self.UserName))

	// Инициализация репозитория (общий для всех сервисов)
	repo := repository.New(db)
	miniAppSvc, err := miniapp.NewService(repo)
	if err != nil {
		return fmt.Errorf("creating Mini App service: %w", err)
	}

	// Инициализация сервиса онбординга
	onboardSvc := onboarding.NewOnboardingService(repo)

	// Инициализация сервиса геймификации
	gamificationSvc := gamification.NewGamificationService(repo, nil)

	// Инициализация RedisStore для отложенной публикации
	redisStore, err := delayed_publish.NewRedisStore(cache, cfg.Redis.DB)
	if err != nil {
		log.Fatalf("creating redis store: %v", err)
	}

	// Инициализация Publisher (подтверждение + геймификация)
	publisher, err := delayed_publish.NewPublisher(repo, gamificationSvc)
	if err != nil {
		log.Fatalf("creating publisher: %v", err)
	}

	// Инициализация Worker (слушает истечение TTL в Redis)
	worker, err := delayed_publish.NewWorker(publisher)
	if err != nil {
		log.Fatalf("creating worker: %v", err)
	}

	// Инициализация сервиса отложенной публикации (Schedule / Cancel / ConfirmNow)
	delayedSvc, err := delayed_publish.NewService(redisStore, repo, publisher)
	if err != nil {
		log.Fatalf("creating delayed publish service: %v", err)
	}

	tgClient := coretelegram.NewClient(tgBot)
	parserSvc, err := parser.NewService(repo, textProcessor, voiceProcessor, tgClient, delayedSvc)
	if err != nil {
		log.Fatalf("creating standup parser service: %v", err)
	}
	confirmationSvc, err := confirmation.NewService(repo, delayedSvc)
	if err != nil {
		log.Fatalf("creating standup confirmation service: %v", err)
	}

	// Инициализация сервиса дайджеста (проверка каждые 30 секунд)
	digestSvc := digest.NewDigestService(repo, tgClient, 30*time.Second)

	// Инициализация Telegram-бота
	standupBot, err := bot.NewStandupTGBot(tgBot, onboardSvc, parserSvc, confirmationSvc)
	if err != nil {
		log.Fatalf("creating telegram standup bot: %v", err)
	}

	initDataValidator, err := httptransport.NewInitDataValidator(
		cfg.Telegram.BotToken,
		cfg.HTTP.TelegramAuthAge,
	)
	if err != nil {
		return fmt.Errorf("creating Telegram init data validator: %w", err)
	}
	httpHandler, err := httptransport.NewHandler(initDataValidator, miniAppSvc)
	if err != nil {
		return fmt.Errorf("creating Mini App HTTP handler: %w", err)
	}
	httpServer, err := httptransport.NewServer(cfg.HTTP.Address, httpHandler)
	if err != nil {
		return fmt.Errorf("creating Mini App HTTP server: %w", err)
	}

	// Запуск обработки обновлений Telegram
	go func() {
		slog.Info("starting telegram bot updates listener")
		if err := standupBot.GetUpdates(shutdownCtx); err != nil {
			slog.Error("telegram bot updates listener stopped", slog.Any("error", err))
		}
	}()

	// Запуск Worker отложенной публикации (слушает истечение TTL в Redis)
	go func() {
		slog.Info("starting delayed publish worker")
		if err := worker.Run(shutdownCtx, redisStore); err != nil && err != context.Canceled {
			slog.Error("delayed publish worker stopped", slog.Any("error", err))
		}
	}()

	// Запуск сервиса дайджеста (публикация по расписанию)
	go func() {
		slog.Info("starting digest service")
		if err := digestSvc.Start(shutdownCtx); err != nil && err != context.Canceled {
			slog.Error("digest service stopped", slog.Any("error", err))
		}
	}()

	httpErrors := make(chan error, 1)
	go func() {
		slog.Info("starting Mini App HTTP server", slog.String("address", cfg.HTTP.Address))
		httpErrors <- httpServer.ListenAndServe()
	}()

	var httpErr error
	select {
	case <-shutdownCtx.Done():
	case httpErr = <-httpErrors:
		stop()
	}

	httpShutdownCtx, cancelHTTPShutdown := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancelHTTPShutdown()
	if err := httpServer.Shutdown(httpShutdownCtx); err != nil {
		slog.Error("Mini App HTTP server shutdown failed", slog.Any("error", err))
	}
	slog.Info("shutting down telegram bot...")
	standupBot.Stop()
	standupBot.Wait()

	if httpErr != nil {
		return fmt.Errorf("Mini App HTTP server stopped: %w", httpErr)
	}
	return nil
}
