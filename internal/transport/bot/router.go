package bot

import (
	"context"
	"strings"

	domain "VoiceStandup.ai/internal/core/domain"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (s *StandupTGBot) route(ctx context.Context, update tgbotapi.Update) error {

	// 1. сценарий: бот добавлен в группу
	if update.MyChatMember != nil {
		chatMember := update.MyChatMember
		// Проверяем старый статус, чтобы исключить простое переключение прав
		if chatMember.OldChatMember.Status == "left" || chatMember.OldChatMember.Status == "kicked" {
			if chatMember.NewChatMember.Status != "left" && chatMember.NewChatMember.Status != "kicked" {
				return s.handleBotAddedToGroup(ctx, chatMember)
			}
		}
	}

	// 2. сценарий: нажали на inline кнопку
	// проверяем, является ли апдейт нажатием на кнопку
	if update.CallbackQuery != nil {
		callback := update.CallbackQuery
		return s.handleInlineClicked(ctx, callback)
	}

	// Обрабатываем только текстовые и медиа-сообщения
	if update.Message == nil {
		return nil
	}

	msg := update.Message
	userID := msg.From.ID

	// 3. сценарий: ОТДЕЛЯЕМ КОМАНДЫ (например, /start)
	if msg.IsCommand() {
		switch msg.Command() {
		case "start":
			return s.handleStart(ctx, msg)
		}
	}

	// 4. сценарий: ГОЛОСОВОЕ/ОБЫЧНЫЙ ТЕКСТ
	userState, err := s.onboard.GetUserState(ctx, userID)
	if err != nil {
		return err
	}

	if strings.HasPrefix(userState, domain.StatePrefixAwaitingRole) {
		return s.handleSetUserRole(ctx, msg)
	}

	// default: Voice стендап
	if msg.Voice != nil {
		return s.handleVoiceStandup(ctx, msg)
	}
	return s.handleTextStandup(ctx, msg)
}

func baseReqFromMsg(msg *tgbotapi.Message) domain.StandupTGBotBaseRequestDTO {
	return domain.StandupTGBotBaseRequestDTO{
		UserID:   msg.From.ID,
		ChatID:   msg.Chat.ID,
		Username: msg.From.UserName,
	}
}
