package bot

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	domain "VoiceStandup.ai/internal/core/domain"
	"VoiceStandup.ai/internal/standup/parser"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

const (
	confirmCallbackAction = "standup_confirm"
	cancelCallbackAction  = "standup_cancel"
)

func (s *StandupTGBot) handleStart(ctx context.Context, msg *tgbotapi.Message) error {

	var teamID int64
	args := msg.CommandArguments()
	if args != "" {
		parsed, err := strconv.ParseInt(args, 10, 64)
		if err != nil {
			s.answerToTG(&domain.StandupTGBotResponseDTO{
				TargetChatID: msg.Chat.ID,
				Text:         "❌ Неверная ссылка-приглашение. Попроси новую у администратора.",
			})
			return nil // ← не fatal, просто ответ пользователю
		}
		teamID = parsed
	}

	req := &domain.StandupTGBotTeamRequestDTO{
		TeamID:                     teamID,
		StandupTGBotBaseRequestDTO: baseReqFromMsg(msg),
	}

	resp, err := s.onboard.StartOnboarding(ctx, req)
	if err != nil {
		return err
	}
	if resp == nil {
		return fmt.Errorf("Пришел пустой response")
	}
	s.answerToTG(resp)
	return nil
}

func (s *StandupTGBot) handleSetUserRole(ctx context.Context, msg *tgbotapi.Message) error {
	req := &domain.StandupTGBotTextRequestDTO{
		Text:                       msg.Text,
		StandupTGBotBaseRequestDTO: baseReqFromMsg(msg),
	}

	resp, err := s.onboard.SetUserRole(ctx, req)
	if err != nil {
		return err
	}
	if resp == nil {
		return fmt.Errorf("Пришел пустой response")
	}
	s.answerToTG(resp)
	return nil
}

func (s *StandupTGBot) handleBotAddedToGroup(ctx context.Context, chatMember *tgbotapi.ChatMemberUpdated) error {

	s.logger.Info("Бот добавлен в чат", "Title", chatMember.Chat.Title, "Chat ID", chatMember.Chat.ID)

	resp, err := s.onboard.BotAddedToGroup(ctx, chatMember.Chat.ID)
	if err != nil {
		s.logger.Error("Ошибка обработки добавления бота", "error", err)
		return err
	}
	if resp == nil {
		return fmt.Errorf("Пришел пустой response")
	}
	s.answerToTG(resp)
	return nil
}

func (s *StandupTGBot) handleVoiceStandup(ctx context.Context, msg *tgbotapi.Message) error {
	preview, err := s.standup.ProcessVoice(ctx, parser.VoiceInput{
		TelegramUserID: msg.From.ID,
		FileID:         msg.Voice.FileID,
		Duration:       time.Duration(msg.Voice.Duration) * time.Second,
	})
	if err != nil {
		return err
	}
	return s.answerPreviewToTG(msg.Chat.ID, preview)
}

func (s *StandupTGBot) handleTextStandup(ctx context.Context, msg *tgbotapi.Message) error {
	preview, err := s.standup.ProcessText(ctx, parser.TextInput{
		TelegramUserID: msg.From.ID,
		Text:           msg.Text,
	})
	if err != nil {
		return err
	}
	return s.answerPreviewToTG(msg.Chat.ID, preview)
}

// Обработка inline-кнопки
func (s *StandupTGBot) handleInlineClicked(ctx context.Context, callback *tgbotapi.CallbackQuery) error {
	// СРАЗУ отвечаем Telegram, что мы получили нажатие (чтобы убрать "часики" на кнопке)
	callbackAnswer := tgbotapi.NewCallback(callback.ID, "")
	if _, err := s.bot.Request(callbackAnswer); err != nil {
		s.logger.Error("Ошибка ответа на callback", "error", err)
	}

	if callback.Message == nil || callback.From == nil {
		return fmt.Errorf("callback message or sender is missing")
	}
	action, submissionID, err := parseStandupCallback(callback.Data)
	if err != nil {
		return err
	}

	var responseText string
	switch action {
	case confirmCallbackAction:
		stats, err := s.confirm.ConfirmNow(ctx, callback.From.ID, submissionID)
		if err != nil {
			return err
		}
		responseText = fmt.Sprintf(
			"✅ Стендап подтверждён!\n\n<b>🎯 Очки:</b> %d XP\n<b>⭐ Уровень:</b> %d\n<b>🔥 Стрик:</b> %d %s",
			stats.XP, stats.Level, stats.CurrentStreak,
			streakEmoji(stats.CurrentStreak),
		)
	case cancelCallbackAction:
		if err := s.confirm.Cancel(ctx, callback.From.ID, submissionID); err != nil {
			return err
		}
		responseText = "Стендап отменён."
	}

	markup := tgbotapi.NewEditMessageReplyMarkup(
		callback.Message.Chat.ID,
		callback.Message.MessageID,
		tgbotapi.InlineKeyboardMarkup{InlineKeyboard: make([][]tgbotapi.InlineKeyboardButton, 0)},
	)
	if _, err := s.bot.Request(markup); err != nil {
		s.logger.Warn("Не удалось убрать inline-кнопки", "error", err)
	}
	s.answerToTG(&domain.StandupTGBotResponseDTO{
		TargetChatID: callback.Message.Chat.ID,
		Text:         responseText,
	})
	return nil
}

func (s *StandupTGBot) answerPreviewToTG(chatID int64, preview *domain.StandupPreview) error {
	if preview == nil || preview.SubmissionID == uuid.Nil {
		return fmt.Errorf("standup preview is empty")
	}

	message := tgbotapi.NewMessage(chatID, formatStandupPreview(preview))
	message.ParseMode = tgbotapi.ModeHTML
	message.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("✅ Отправить", callbackData(confirmCallbackAction, preview.SubmissionID)),
		tgbotapi.NewInlineKeyboardButtonData("❌ Отменить", callbackData(cancelCallbackAction, preview.SubmissionID)),
	))
	if _, err := s.bot.Send(message); err != nil {
		return fmt.Errorf("send standup preview: %w", err)
	}
	return nil
}

func formatStandupPreview(preview *domain.StandupPreview) string {
	return fmt.Sprintf(
		"<b>Предпросмотр стендапа</b>\n\n"+
			"<b>😎Что сделано:</b>\n%s\n\n"+
			"<b>🧐Что в планах:</b>\n%s\n\n"+
			"<b>💥Блокеры:</b>\n%s",
		previewValue(preview.Done),
		previewValue(preview.Plans),
		previewValue(preview.Blockers),
	)
}

func previewValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "—"
	}
	return html.EscapeString(value)
}

func streakEmoji(streak int) string {
	switch {
	case streak >= 10:
		return "🔥🔥🔥"
	case streak >= 5:
		return "🔥🔥"
	case streak >= 3:
		return "🔥"
	default:
		return "🌱"
	}
}

func callbackData(action string, submissionID uuid.UUID) string {
	return action + ":" + submissionID.String()
}

func parseStandupCallback(data string) (string, uuid.UUID, error) {
	action, rawID, found := strings.Cut(data, ":")
	if !found || (action != confirmCallbackAction && action != cancelCallbackAction) {
		return "", uuid.Nil, fmt.Errorf("unknown standup callback")
	}
	submissionID, err := uuid.Parse(rawID)
	if err != nil {
		return "", uuid.Nil, fmt.Errorf("invalid submission ID in callback: %w", err)
	}
	return action, submissionID, nil
}
