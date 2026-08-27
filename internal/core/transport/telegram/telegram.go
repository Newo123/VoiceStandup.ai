package telegram

import (
	"context"
	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Client — клиент для отправки сообщений в Telegram.
type Client struct {
	bot    *tgbotapi.BotAPI
	logger *slog.Logger
}

// NewClient создаёт новый клиент для отправки сообщений.
func NewClient(bot *tgbotapi.BotAPI) *Client {
	return &Client{
		bot:    bot,
		logger: slog.Default().With("component", "telegram_client"),
	}
}

// SendMessage отправляет HTML-сообщение в указанный чат.
func (c *Client) SendMessage(ctx context.Context, chatID int64, html string) error {
	msg := tgbotapi.NewMessage(chatID, html)
	msg.ParseMode = tgbotapi.ModeHTML

	_, err := c.bot.Send(msg)
	if err != nil {
		c.logger.Error("отправка сообщения в Telegram",
			"chat_id", chatID,
			"error", err,
		)
		return err
	}

	c.logger.Debug("сообщение отправлено",
		"chat_id", chatID,
	)
	return nil
}
