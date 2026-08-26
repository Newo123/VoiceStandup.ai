package domain

import (
	"time"

	uuid "github.com/google/uuid"
)

// const
const (
	StateNone               = ""
	StateOnboarded          = "onboarded"
	StatePrefixAwaitingRole = "awaiting_role" // Пользователь сейчас должен выбрать/ввести роль. awaiting_role:<uuid>
)

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
	TeamID int64 // ChatID команды
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

// DB
type Users struct {
	ID             uuid.UUID
	State          string
	TelegramUserID int64
	Username       string
	DisplayName    string
	CreatedAt      time.Time
	DeletedAt      *time.Time
}

type Teams struct {
	ID                       uuid.UUID
	Name                     string
	TelegramChatID           int64
	Timezone                 string
	PublishLocalTime         time.Time
	Workdays                 []int
	LatePolicy               string
	LastPublishedStandupDate time.Time
	CreatedAt                time.Time
	DeletedAt                *time.Time
}

type TeamMembers struct {
	TeamID    uuid.UUID
	UserID    uuid.UUID
	Role      string
	IsOwner   bool
	FullName  string
	Status    string
	CreatedAt time.Time
	DeletedAt *time.Time
}

type TelegramUpdates struct {
	UpdateID   int64
	ReceivedAt time.Time
}

type Submissions struct {
	ID           uuid.UUID
	TeamID       uuid.UUID
	UserID       uuid.UUID
	StandupDate  time.Time
	Status       string
	DoneText     *string
	PlansText    *string
	BlockersText *string
	ConfirmedAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    *time.Time
}
