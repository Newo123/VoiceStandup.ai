package bot

import (
	"context"
	"fmt"

	domain "VoiceStandup.ai/internal/core/domain"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (s *StandupTGBot) Route(ctx context.Context, update tgbotapi.Update) error {

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
	// Получаем текущее состояние пользователя из вашего кэша/БД
	userState, err := s.service.GetUserState(ctx, userID)
	if err != nil {
		return err
	}
	switch userState {
	case StateAwaitingRole:
		// Если бот ждал от пользователя роль (например, после Onboarding)
		return s.handleSetUserRole(ctx, msg)
	default:
		if msg.Voice != nil {
			return s.handleVoiceStandup(ctx, msg)
		} else {
			return s.handleTextStandup(ctx, msg)
		}
	}
}

func (s *StandupTGBot) handleBotAddedToGroup(ctx context.Context, chatMember *tgbotapi.ChatMemberUpdated) error {

	s.logger.Info("Бот добавлен в чат", "Title", chatMember.Chat.Title, "Chat ID", chatMember.Chat.ID)

	resp, err := s.service.BotAddedToGroup(ctx, chatMember.Chat.ID)
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

// Обработка inline-кнопки
func (s *StandupTGBot) handleInlineClicked(ctx context.Context, callback *tgbotapi.CallbackQuery) error {

	req := &domain.StandupTGBotBaseRequestDTO{
		UserID:   callback.Message.From.ID,
		ChatID:   callback.Message.Chat.ID,
		Username: callback.Message.From.UserName,
	}

	// СРАЗУ отвечаем Telegram, что мы получили нажатие (чтобы убрать "часики" на кнопке)
	callbackAnswer := tgbotapi.NewCallback(callback.ID, "")
	if _, err := s.bot.Request(callbackAnswer); err != nil {
		s.logger.Error("Ошибка ответа на callback", "error", err)
	}

	s.logger.Info("Нажата inline клавиша", "callback.Data", callback.Data, "username", callback.From.UserName)

	switch callback.Data {
	// «Отправить»
	case "send_button":
		resp, err := s.service.SaveReport(ctx, req)
		if err != nil {
			return err
		}
		if resp == nil {
			return fmt.Errorf("Пришел пустой response")
		}
		s.answerToTG(resp)
		return nil
	// «Отменить»
	case "cancel_button":
		resp, err := s.service.CancelReport(ctx, req)
		if err != nil {
			return err
		}
		if resp == nil {
			return fmt.Errorf("Пришел пустой response")
		}
		s.answerToTG(resp)
		return nil
	default:
		s.logger.Warn("Неизвестная кнопка", "button", callback.Data)
		return nil
	}
}

func (s *StandupTGBot) handleStart(ctx context.Context, msg *tgbotapi.Message) error {
	req := &domain.StandupTGBotBaseRequestDTO{
		UserID:   msg.From.ID,
		Username: msg.From.UserName,
		ChatID:   msg.Chat.ID,
	}

	resp, err := s.service.StartOnboarding(ctx, req)
	if err != nil {
		return err
	}
	if resp == nil {
		return fmt.Errorf("Пришел пустой response")
	}
	s.answerToTG(resp)
	return nil
}

func (s *StandupTGBot) handleVoiceStandup(ctx context.Context, msg *tgbotapi.Message) error {
	req := &domain.StandupTGBotVoiceRequestDTO{
		VoiceFileID:                msg.Voice.FileID,
		Duration:                   msg.Voice.Duration,
		StandupTGBotBaseRequestDTO: baseReqFromMsg(msg),
	}

	resp, err := s.service.ProcessVoiceStandup(ctx, req)
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

	resp, err := s.service.SetUserRole(ctx, req)
	if err != nil {
		return err
	}
	if resp == nil {
		return fmt.Errorf("Пришел пустой response")
	}
	s.answerToTG(resp)
	return nil
}

func (s *StandupTGBot) handleTextStandup(ctx context.Context, msg *tgbotapi.Message) error {
	req := &domain.StandupTGBotTextRequestDTO{
		Text:                       msg.Text,
		StandupTGBotBaseRequestDTO: baseReqFromMsg(msg),
	}

	resp, err := s.service.ProcessTextStandup(ctx, req)
	if err != nil {
		return err
	}
	if resp == nil {
		return fmt.Errorf("Пришел пустой response")
	}
	s.answerToTG(resp)
	return nil
}

func baseReqFromMsg(msg *tgbotapi.Message) domain.StandupTGBotBaseRequestDTO {
	return domain.StandupTGBotBaseRequestDTO{
		UserID:   msg.From.ID,
		ChatID:   msg.Chat.ID,
		Username: msg.From.UserName,
	}
}
