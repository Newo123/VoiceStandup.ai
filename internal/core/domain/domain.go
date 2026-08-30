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

	SubmissionStatusProcessing           = "processing"
	SubmissionStatusAwaitingConfirmation = "awaiting_confirmation"
	SubmissionStatusConfirmed            = "confirmed"
	SubmissionStatusFailed               = "failed"

	SubmissionFormatText  = "text"
	SubmissionFormatVoice = "voice"

	LatePolicyNextDigest      = "NEXT_DIGEST"
	LatePolicySeparateMessage = "SEPARATE_MESSAGE"
)

// StandupResponse — DTO структурированного стендап-отчёта, полученного от LLM.
type StandupResponse struct {
	Done     string `json:"done"`
	Plans    string `json:"plans"`
	Blockers string `json:"blockers"`
}

// StandupPreview — подготовленный отчёт, который пользователь должен подтвердить.
type StandupPreview struct {
	SubmissionID uuid.UUID
	TeamID       uuid.UUID
	StandupDate  time.Time
	Format       string
	Done         string
	Plans        string
	Blockers     string
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

type StandupTGBotResponseDTO struct {
	TargetChatID int64
	Text         string
}

// DB
type Users struct {
	ID             uuid.UUID
	State          string
	ActiveTeamID   *uuid.UUID
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

type TeamMembership struct {
	Team    Teams
	Role    string
	IsOwner bool
}

type TeamMemberStats struct {
	UserID        uuid.UUID
	Username      string
	DisplayName   string
	Role          string
	IsOwner       bool
	XP            int
	Level         int
	CurrentStreak int
	BestStreak    int
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
	Format       string // "voice" or "text"
	DoneText     *string
	PlansText    *string
	BlockersText *string
	ConfirmedAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    *time.Time
}

type UserStats struct {
	UserID          uuid.UUID
	XP              int
	Level           int
	CurrentStreak   int
	BestStreak      int
	LastStandupDate *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
