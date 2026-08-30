package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	domain "VoiceStandup.ai/internal/core/domain"
	coretelegram "VoiceStandup.ai/internal/core/transport/telegram"
	"VoiceStandup.ai/internal/standup/parser"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

type StandupTGBot struct {
	onboard OnboardingService
	standup StandupIngestionService
	confirm StandupConfirmationService
	bot     *tgbotapi.BotAPI
	done    chan struct{}
	logger  *slog.Logger
}

type OnboardingService interface {
	// /start
	StartOnboarding(ctx context.Context, req *domain.StandupTGBotTeamRequestDTO) (*domain.StandupTGBotResponseDTO, error)

	// Записывать роль юзера
	SetUserRole(ctx context.Context, req *domain.StandupTGBotTextRequestDTO) (*domain.StandupTGBotResponseDTO, error)

	// Статус юзера (FSM: ждем роль, ждем отчет и т.д.)
	GetUserState(ctx context.Context, userID int64) (string, error)

	// Сформировать сообщение после добавления бота в группу (инструкция с Chat ID)
	BotAddedToGroup(ctx context.Context, chatID int64) (*domain.StandupTGBotResponseDTO, error)
}
type StandupIngestionService interface {
	ProcessVoice(ctx context.Context, input parser.VoiceInput) (*domain.StandupPreview, error)
	ProcessText(ctx context.Context, input parser.TextInput) (*domain.StandupPreview, error)
}
type StandupConfirmationService interface {
	ConfirmNow(ctx context.Context, telegramUserID int64, submissionID uuid.UUID) (*domain.UserStats, error)
	Cancel(ctx context.Context, telegramUserID int64, submissionID uuid.UUID) error
}

func NewStandupTGBot(
	bot *tgbotapi.BotAPI,
	onboard OnboardingService,
	standup StandupIngestionService,
	confirm StandupConfirmationService,
) (*StandupTGBot, error) {
	if bot == nil {
		return nil, fmt.Errorf("telegram bot is required")
	}
	if onboard == nil {
		return nil, fmt.Errorf("onboarding service is required")
	}
	if standup == nil {
		return nil, fmt.Errorf("standup ingestion service is required")
	}
	if confirm == nil {
		return nil, fmt.Errorf("standup confirmation service is required")
	}

	return &StandupTGBot{
		onboard: onboard,
		standup: standup,
		confirm: confirm,
		bot:     bot,
		done:    make(chan struct{}),
		logger:  slog.Default().With("component", "standup_tg_bot"),
	}, nil
}

func (s *StandupTGBot) GetUpdates(ctx context.Context) error {
	defer close(s.done)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	for {
		select {
		case <-ctx.Done():
			s.logger.Debug("Контекст GetUpdates завершен")
			return ctx.Err()
		default:
		}

		updates, err := s.bot.GetUpdates(u)
		if err != nil {
			s.logger.Error("GetUpdates error", "error", err)
			time.Sleep(3 * time.Second)
			continue
		}

		for _, update := range updates {
			u.Offset = update.UpdateID + 1

			err := s.route(ctx, update)
			if err != nil {
				var chatID int64
				if update.Message != nil {
					chatID = update.Message.Chat.ID
				} else if update.CallbackQuery != nil && update.CallbackQuery.Message != nil {
					chatID = update.CallbackQuery.Message.Chat.ID
				}
				if chatID != 0 {
					s.answerToTGError(err, chatID)
				}
			}
		}
	}
}

// обрабатывает response от сервиса для формирования ответа юзеру
func (s *StandupTGBot) answerToTG(resp *domain.StandupTGBotResponseDTO) {
	msg := tgbotapi.NewMessage(resp.TargetChatID, resp.Text)
	msg.ParseMode = tgbotapi.ModeHTML

	if _, err := s.bot.Send(msg); err != nil {
		s.logger.Error("send failed", "error", err, "chat_id", resp.TargetChatID)
	}
}

// в случае если сервис отдает ошибку. Возвращаем типовой ответ
func (s *StandupTGBot) answerToTGError(err error, chatID int64) {
	s.logger.Error("Ошибка обработки запроса", "error", err)

	text := userErrorMessage(err)
	msg := tgbotapi.NewMessage(chatID, text)

	if _, err := s.bot.Send(msg); err != nil {
		s.logger.Error("send failed", "error", err, "chat_id", chatID)
	}
}

func (s *StandupTGBot) answerPlainText(chatID int64, text string) error {
	if _, err := s.bot.Send(tgbotapi.NewMessage(chatID, text)); err != nil {
		return fmt.Errorf("send Telegram message: %w", err)
	}
	return nil
}

func userErrorMessage(err error) string {
	switch {
	case errors.Is(err, parser.ErrEmptyText):
		return "Отправь непустой текст стендапа."
	case errors.Is(err, parser.ErrVoiceTooLong):
		return "Голосовое слишком длинное. Максимальная длительность — 2 минуты."
	case errors.Is(err, coretelegram.ErrVoiceFileTooLarge):
		return "Голосовой файл слишком большой. Запиши сообщение короче."
	case errors.Is(err, parser.ErrUserNotFound):
		return "Сначала зарегистрируйся через команду /start."
	case errors.Is(err, parser.ErrActiveTeamRequired), errors.Is(err, parser.ErrActiveTeamNotFound):
		return "Сначала выбери активную команду через её ссылку-приглашение."
	default:
		return "Возникла ошибка при обработке сообщения. Попробуй позднее."
	}
}

func (s *StandupTGBot) Wait() {
	<-s.done
}

func (s *StandupTGBot) Stop() {
	s.bot.StopReceivingUpdates()
}
