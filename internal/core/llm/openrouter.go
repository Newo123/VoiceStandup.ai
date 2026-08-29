package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultBaseURL — базовый URL OpenRouter API.
const DefaultBaseURL = "https://openrouter.ai/api/v1"

// chatCompletionRequest — тело JSON-запроса к OpenRouter API.
type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

// chatMessage — одно сообщение в запросе chat completion.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatCompletionResponse — ответ OpenRouter API.
type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Client — низкоуровневый HTTP-клиент для OpenRouter chat completions API.
type Client struct {
	httpClient *http.Client
	apiKey     string
	model      string
	baseURL    string
}

// NewOpenRouterClient создаёт новый Client.
// model и timeout должны быть переданы явно (берутся из конфигурации).
func NewOpenRouterClient(apiKey, model string, timeout time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		apiKey:     apiKey,
		model:      model,
		baseURL:    DefaultBaseURL,
	}
}

// Complete отправляет запрос chat completion в OpenRouter и возвращает
// текст ответа ассистента.
func (c *Client) Complete(ctx context.Context, systemPrompt, userContent string) (string, error) {
	body := chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userContent},
		},
		Temperature: 0.2,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("llm: ошибка маршалинга запроса: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("llm: ошибка создания запроса: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm: ошибка выполнения запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("llm: неожиданный статус %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("llm: ошибка декодирования ответа: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("llm: пустой список choices в ответе")
	}

	return chatResp.Choices[0].Message.Content, nil
}
