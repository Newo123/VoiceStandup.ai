package telegram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	maxVoiceFileSize     = 20 << 20
	voiceDownloadTimeout = 30 * time.Second
)

var ErrVoiceFileTooLarge = errors.New("telegram voice file is too large")

type fileURLProvider interface {
	GetFileDirectURL(fileID string) (string, error)
}

// Client — клиент для отправки сообщений в Telegram.
type Client struct {
	bot              *tgbotapi.BotAPI
	files            fileURLProvider
	httpClient       *http.Client
	maxVoiceFileSize int64
	logger           *slog.Logger
}

// NewClient создаёт новый клиент для отправки сообщений.
func NewClient(bot *tgbotapi.BotAPI) *Client {
	return &Client{
		bot:              bot,
		files:            bot,
		httpClient:       &http.Client{Timeout: voiceDownloadTimeout},
		maxVoiceFileSize: maxVoiceFileSize,
		logger:           slog.Default().With("component", "telegram_client"),
	}
}

// DownloadVoice скачивает голосовое сообщение Telegram и ограничивает его
// размер, чтобы не загружать произвольный объём данных в память.
func (c *Client) DownloadVoice(ctx context.Context, fileID string) ([]byte, error) {
	if strings.TrimSpace(fileID) == "" {
		return nil, fmt.Errorf("telegram: voice file ID is required")
	}
	if c.files == nil || c.httpClient == nil || c.maxVoiceFileSize <= 0 {
		return nil, fmt.Errorf("telegram: voice downloader is not configured")
	}

	fileURL, err := c.files.GetFileDirectURL(fileID)
	if err != nil {
		return nil, fmt.Errorf("telegram: get voice file URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("telegram: create voice download request: %w", err)
	}

	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram: download voice: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram: download voice: unexpected status %d", response.StatusCode)
	}

	audio, err := io.ReadAll(io.LimitReader(response.Body, c.maxVoiceFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("telegram: read voice: %w", err)
	}
	if int64(len(audio)) > c.maxVoiceFileSize {
		return nil, fmt.Errorf("%w: maximum size is %d bytes", ErrVoiceFileTooLarge, c.maxVoiceFileSize)
	}
	if len(audio) == 0 {
		return nil, fmt.Errorf("telegram: downloaded voice file is empty")
	}
	return audio, nil
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
