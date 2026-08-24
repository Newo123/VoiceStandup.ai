package domain

// StandupTGBot Request Response DTO
type StandupTGBotBaseChatIDRequestDTO struct {
	ChatID int64 // ID чата в Telegram
}

type StandupTGBotBaseRequestDTO struct {
	ChatID   int64  // ID чата в Telegram
	UserID   int64  // ID пользователя
	Username string // Имя пользователя
}

type StandupTGBotTextRequestDTO struct {
	StandupTGBotBaseRequestDTO
	Text string
}

type StandupTGBotTeamRequestDTO struct {
	StandupTGBotBaseRequestDTO
	TeamID string // номер к какой команде привязать человека
}

type StandupTGBotVoiceRequestDTO struct {
	StandupTGBotBaseRequestDTO
	VoiceFileID string
	Duration    int // seconds
}

type StandupTGBotResponseDTO struct {
	TargetChatID int64
	Text         string
}
