package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"VoiceStandup.ai/internal/core/domain"
	"VoiceStandup.ai/internal/core/stt"
)

// VoiceProcessor обрабатывает голосовые сообщения: распознавание речи (STT)
// с последующей коррекцией и структурированием через LLM.
type VoiceProcessor struct {
	stt stt.Transcriber
	llm *Client
}

// NewVoiceProcessor создаёт новый VoiceProcessor. Возвращает ошибку, если
// любая из зависимостей равна nil.
func NewVoiceProcessor(stt stt.Transcriber, llm *Client) (*VoiceProcessor, error) {
	if stt == nil {
		return nil, fmt.Errorf("llm: stt-транскрайбер равен nil")
	}
	if llm == nil {
		return nil, fmt.Errorf("llm: llm-клиент равен nil")
	}
	return &VoiceProcessor{stt: stt, llm: llm}, nil
}

// ProcessVoice транскрибирует аудио через STT, затем отправляет сырой текст
// в LLM для коррекции и структурирования. Возвращает распарсенный StandupResponse.
func (v *VoiceProcessor) ProcessVoice(ctx context.Context, audioBytes []byte) (*domain.StandupResponse, error) {
	rawText, err := v.stt.Transcribe(ctx, audioBytes)
	if err != nil {
		return nil, fmt.Errorf("llm: ошибка stt-транскрибации: %w", err)
	}

	systemPrompt := `Ты — ассистент для стендап-отчетов. Пользователь надиктовал отчет голосом. Исправь галлюцинации STT, артефакты речи, пунктуацию и структурируй текст. Верни СТРОГО JSON без markdown-разметки: {"done": "что сделано", "in_progress": "что в работе", "blockers": "блокеры или нет"}`

	result, err := v.llm.Complete(ctx, systemPrompt, rawText)
	if err != nil {
		return nil, fmt.Errorf("llm: ошибка llm-обработки голоса: %w", err)
	}

	var resp domain.StandupResponse
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return nil, fmt.Errorf("llm: ошибка парсинга ответа голосового процессора: %w", err)
	}

	return &resp, nil
}
