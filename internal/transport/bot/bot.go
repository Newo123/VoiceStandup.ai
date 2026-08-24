// 4.1. internal/transport/telegram — Обработка Telegram Bot API:

// - Выдача Chat ID при добавлении бота в чат.
// - Хэндлеры /start, регистрационных ответов, приема голоса и текста в ЛС.
// - Обработка inline-кнопки «Отменить» или «Отправить» в предпросмотре.

// TODO
// 1. название кнопок callback.Data
// 2. Stop() вызываем тут или в main?
// 3. StateNone StateAwaitingRole соотнести с сервисом

package bot

import (
	"context"
	"log/slog"
	"time"

	domain "VoiceStandup.ai/internal/core/domain"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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
	// Голосовой отчет -> транскрибация -> LLM -> Превью
	ProcessVoiceStandup(ctx context.Context, req *domain.StandupTGBotVoiceRequestDTO) (*domain.StandupTGBotResponseDTO, error)

	// Текстовый отчет -> LLM -> Превью
	ProcessTextStandup(ctx context.Context, req *domain.StandupTGBotTextRequestDTO) (*domain.StandupTGBotResponseDTO, error)
}
type StandupConfirmationService interface {
	// Обработка inline-кнопки «Отправить» в предпросмотре.
	SaveReport(ctx context.Context, req *domain.StandupTGBotBaseRequestDTO) (*domain.StandupTGBotResponseDTO, error)

	// Обработка inline-кнопки «Отменить» в предпросмотре.
	CancelReport(ctx context.Context, req *domain.StandupTGBotBaseRequestDTO) (*domain.StandupTGBotResponseDTO, error)
}

func NewStandupTGBot(
	bot *tgbotapi.BotAPI,
	onboard OnboardingService,
	standup StandupIngestionService,
	confirm StandupConfirmationService) *StandupTGBot {
	return &StandupTGBot{
		onboard: onboard,
		standup: standup,
		confirm: confirm,
		bot:     bot,
		done:    make(chan struct{}),
		logger:  slog.Default().With("component", "standup_tg_bot"),
	}
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
				} else if update.CallbackQuery != nil {
					chatID = update.CallbackQuery.Message.Chat.ID
				}
				s.answerToTGError(err, chatID)
			}
		}
	}
}

// обрабатывает response от сервиса для формирования ответа юзеру
func (s *StandupTGBot) answerToTG(resp *domain.StandupTGBotResponseDTO) {
	msg := tgbotapi.NewMessage(resp.TargetChatID, resp.Text)
	msg.ParseMode = tgbotapi.ModeMarkdownV2

	if _, err := s.bot.Send(msg); err != nil {
		s.logger.Error("send failed", "error", err, "chat_id", resp.TargetChatID)
	}
}

// в случае если сервис отдает ошибку. Возвращаем типовой ответ
func (s *StandupTGBot) answerToTGError(err error, chatId int64) {

	s.logger.Error("Ошибка обработки запроса", "error", err, "chat_id", chatId)

	text := "Возникла ошибка при обработке вашего сообщения. Попробуйте позднее!"
	msg := tgbotapi.NewMessage(chatId, text)

	if _, err := s.bot.Send(msg); err != nil {
		s.logger.Error("send failed", "error", err, "chat_id", chatId)
	}
}

func (s *StandupTGBot) Wait() {
	<-s.done
}

func (s *StandupTGBot) Stop() {
	s.bot.StopReceivingUpdates()
}
