package bot

import (
	"context"
	"fmt"

	domain "VoiceStandup.ai/internal/core/domain"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (s *StandupTGBot) handleStart(ctx context.Context, msg *tgbotapi.Message) error {
	req := &domain.StandupTGBotTeamRequestDTO{
		TeamID:                     msg.CommandArguments(),
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
	req := &domain.StandupTGBotVoiceRequestDTO{
		VoiceFileID:                msg.Voice.FileID,
		Duration:                   msg.Voice.Duration,
		StandupTGBotBaseRequestDTO: baseReqFromMsg(msg),
	}

	resp, err := s.standup.ProcessVoiceStandup(ctx, req)
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

	resp, err := s.standup.ProcessTextStandup(ctx, req)
	if err != nil {
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
		resp, err := s.confirm.SaveReport(ctx, req)
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
		resp, err := s.confirm.CancelReport(ctx, req)
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
