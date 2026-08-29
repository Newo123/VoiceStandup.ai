package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"VoiceStandup.ai/internal/core/domain"
)

// TextProcessor обрабатывает текстовые сообщения: отправляет сырой текст
// пользователя в LLM для структурирования в стендап-отчёт.
type TextProcessor struct {
	llm *Client
}

// NewTextProcessor создаёт новый TextProcessor. Возвращает ошибку, если
// llm-клиент равен nil.
func NewTextProcessor(llm *Client) (*TextProcessor, error) {
	if llm == nil {
		return nil, fmt.Errorf("llm: llm-клиент равен nil")
	}
	return &TextProcessor{llm: llm}, nil
}

// ProcessText отправляет сырой текст в LLM для анализа и структурирования
// в стендап-отчёт. Возвращает распарсенный StandupResponse.
func (t *TextProcessor) ProcessText(ctx context.Context, rawText string) (*domain.StandupResponse, error) {
	if strings.TrimSpace(rawText) == "" {
		return nil, fmt.Errorf("llm: сырой текст пуст")
	}

	systemPrompt := `Ты — ассистент для стендап-отчетов. Проанализируй текст отчета пользователя и разбей его по категориям. Верни СТРОГО JSON без markdown-разметки: {"done": "что сделано", "plans": "что в планах", "blockers": "блокеры или нет"}`

	result, err := t.llm.Complete(ctx, systemPrompt, rawText)
	if err != nil {
		return nil, fmt.Errorf("llm: ошибка llm-обработки текста: %w", err)
	}

	var resp domain.StandupResponse
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return nil, fmt.Errorf("llm: ошибка парсинга ответа текстового процессора: %w", err)
	}

	return &resp, nil
}
