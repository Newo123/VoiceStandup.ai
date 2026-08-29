package stt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// DefaultSTTBaseURL — базовый URL OpenRouter API для аудио.
const DefaultSTTBaseURL = "https://openrouter.ai/api/v1"

// whisperResponse — ответ OpenRouter API на запрос транскрибации.
type whisperResponse struct {
	Text string `json:"text"`
}

// WhisperClient — клиент для транскрибации аудио через OpenRouter Whisper API.
type WhisperClient struct {
	httpClient *http.Client
	apiKey     string
	model      string
	baseURL    string
}

// NewWhisperClient создаёт новый WhisperClient.
// model и timeout должны быть переданы явно (берутся из конфигурации).
func NewWhisperClient(apiKey, model string, timeout time.Duration) *WhisperClient {
	return &WhisperClient{
		httpClient: &http.Client{Timeout: timeout},
		apiKey:     apiKey,
		model:      model,
		baseURL:    DefaultSTTBaseURL,
	}
}

// Transcribe отправляет аудио-файл в OpenRouter Whisper API и возвращает
// распознанный текст.
func (c *WhisperClient) Transcribe(ctx context.Context, audioBytes []byte) (string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Поле model
	if err := writer.WriteField("model", c.model); err != nil {
		return "", fmt.Errorf("stt: ошибка записи поля model: %w", err)
	}

	// Поле file — аудио-данные
	part, err := writer.CreateFormFile("file", "audio.ogg")
	if err != nil {
		return "", fmt.Errorf("stt: ошибка создания form-file: %w", err)
	}
	if _, err := part.Write(audioBytes); err != nil {
		return "", fmt.Errorf("stt: ошибка записи аудио-данных: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("stt: ошибка закрытия multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/audio/transcriptions", body)
	if err != nil {
		return "", fmt.Errorf("stt: ошибка создания запроса: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("stt: ошибка выполнения запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("stt: неожиданный статус %d: %s", resp.StatusCode, string(respBody))
	}

	var wResp whisperResponse
	if err := json.NewDecoder(resp.Body).Decode(&wResp); err != nil {
		return "", fmt.Errorf("stt: ошибка декодирования ответа: %w", err)
	}

	if wResp.Text == "" {
		return "", fmt.Errorf("stt: пустой текст в ответе Whisper")
	}

	return wResp.Text, nil
}
