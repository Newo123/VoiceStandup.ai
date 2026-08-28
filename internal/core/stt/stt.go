package stt

import "context"

// Transcriber — интерфейс для распознавания речи в текст.
type Transcriber interface {
	Transcribe(ctx context.Context, audioBytes []byte) (string, error)
}
