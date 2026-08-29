// Package parser преобразует входящий текст или голос в черновик стендапа.
package parser

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"VoiceStandup.ai/internal/core/domain"
	"github.com/google/uuid"
)

const (
	DefaultMaxVoiceDuration = 2 * time.Minute
	rollbackTimeout         = 5 * time.Second
)

var (
	ErrEmptyText          = errors.New("standup text is empty")
	ErrVoiceFileRequired  = errors.New("voice file is required")
	ErrInvalidVoiceLength = errors.New("voice duration must be positive")
	ErrVoiceTooLong       = errors.New("voice message is too long")
	ErrUserNotFound       = errors.New("standup user not found")
	ErrActiveTeamRequired = errors.New("active team is required")
	ErrActiveTeamNotFound = errors.New("active team not found")
	ErrEmptyStandup       = errors.New("standup response is empty")
)

type Repository interface {
	GetActiveUserByTelegramID(ctx context.Context, telegramUserID int64) (*domain.Users, error)
	GetTeamByUUID(ctx context.Context, teamID uuid.UUID) (*domain.Teams, error)
	SaveSubmission(ctx context.Context, submission *domain.Submissions) error
	DeleteSubmission(ctx context.Context, submissionID uuid.UUID) error
}

type TextProcessor interface {
	ProcessText(ctx context.Context, rawText string) (*domain.StandupResponse, error)
}

type VoiceProcessor interface {
	ProcessVoice(ctx context.Context, audio []byte) (*domain.StandupResponse, error)
}

type VoiceLoader interface {
	DownloadVoice(ctx context.Context, fileID string) ([]byte, error)
}

type Scheduler interface {
	Schedule(ctx context.Context, submissionID uuid.UUID) error
}

type TextInput struct {
	TelegramUserID int64
	Text           string
}

type VoiceInput struct {
	TelegramUserID int64
	FileID         string
	Duration       time.Duration
}

type Service struct {
	repository       Repository
	textProcessor    TextProcessor
	voiceProcessor   VoiceProcessor
	voiceLoader      VoiceLoader
	scheduler        Scheduler
	maxVoiceDuration time.Duration
	now              func() time.Time
}

func NewService(
	repository Repository,
	textProcessor TextProcessor,
	voiceProcessor VoiceProcessor,
	voiceLoader VoiceLoader,
	scheduler Scheduler,
) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("parser: repository is required")
	}
	if textProcessor == nil {
		return nil, fmt.Errorf("parser: text processor is required")
	}
	if voiceProcessor == nil {
		return nil, fmt.Errorf("parser: voice processor is required")
	}
	if voiceLoader == nil {
		return nil, fmt.Errorf("parser: voice loader is required")
	}
	if scheduler == nil {
		return nil, fmt.Errorf("parser: scheduler is required")
	}

	return &Service{
		repository:       repository,
		textProcessor:    textProcessor,
		voiceProcessor:   voiceProcessor,
		voiceLoader:      voiceLoader,
		scheduler:        scheduler,
		maxVoiceDuration: DefaultMaxVoiceDuration,
		now:              time.Now,
	}, nil
}

func (s *Service) ProcessText(ctx context.Context, input TextInput) (*domain.StandupPreview, error) {
	rawText := strings.TrimSpace(input.Text)
	if rawText == "" {
		return nil, ErrEmptyText
	}

	user, team, err := s.resolveUserAndTeam(ctx, input.TelegramUserID)
	if err != nil {
		return nil, err
	}

	response, err := s.textProcessor.ProcessText(ctx, rawText)
	if err != nil {
		return nil, fmt.Errorf("parser: process text: %w", err)
	}
	return s.savePreview(ctx, user, team, domain.SubmissionFormatText, response)
}

func (s *Service) ProcessVoice(ctx context.Context, input VoiceInput) (*domain.StandupPreview, error) {
	fileID := strings.TrimSpace(input.FileID)
	if fileID == "" {
		return nil, ErrVoiceFileRequired
	}
	if input.Duration <= 0 {
		return nil, ErrInvalidVoiceLength
	}
	if input.Duration > s.maxVoiceDuration {
		return nil, fmt.Errorf("%w: maximum duration is %s", ErrVoiceTooLong, s.maxVoiceDuration)
	}

	user, team, err := s.resolveUserAndTeam(ctx, input.TelegramUserID)
	if err != nil {
		return nil, err
	}

	audio, err := s.voiceLoader.DownloadVoice(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("parser: download voice: %w", err)
	}
	if len(audio) == 0 {
		return nil, fmt.Errorf("parser: download voice: empty audio")
	}

	response, err := s.voiceProcessor.ProcessVoice(ctx, audio)
	if err != nil {
		return nil, fmt.Errorf("parser: process voice: %w", err)
	}
	return s.savePreview(ctx, user, team, domain.SubmissionFormatVoice, response)
}

func (s *Service) resolveUserAndTeam(
	ctx context.Context,
	telegramUserID int64,
) (*domain.Users, *domain.Teams, error) {
	user, err := s.repository.GetActiveUserByTelegramID(ctx, telegramUserID)
	if err != nil {
		return nil, nil, fmt.Errorf("parser: get user: %w", err)
	}
	if user == nil {
		return nil, nil, ErrUserNotFound
	}
	if user.ActiveTeamID == nil {
		return nil, nil, ErrActiveTeamRequired
	}

	team, err := s.repository.GetTeamByUUID(ctx, *user.ActiveTeamID)
	if err != nil {
		return nil, nil, fmt.Errorf("parser: get active team: %w", err)
	}
	if team == nil {
		return nil, nil, ErrActiveTeamNotFound
	}
	return user, team, nil
}

func (s *Service) savePreview(
	ctx context.Context,
	user *domain.Users,
	team *domain.Teams,
	format string,
	response *domain.StandupResponse,
) (*domain.StandupPreview, error) {
	if response == nil {
		return nil, ErrEmptyStandup
	}

	done := strings.TrimSpace(response.Done)
	plans := strings.TrimSpace(response.Plans)
	blockers := strings.TrimSpace(response.Blockers)
	if done == "" && plans == "" && blockers == "" {
		return nil, ErrEmptyStandup
	}

	standupDate, err := localStandupDate(s.now(), team.Timezone)
	if err != nil {
		return nil, err
	}

	submission := &domain.Submissions{
		TeamID:       team.ID,
		UserID:       user.ID,
		StandupDate:  standupDate,
		Status:       domain.SubmissionStatusAwaitingConfirmation,
		Format:       format,
		DoneText:     stringPointer(done),
		PlansText:    stringPointer(plans),
		BlockersText: stringPointer(blockers),
	}
	if err := s.repository.SaveSubmission(ctx, submission); err != nil {
		return nil, fmt.Errorf("parser: save submission: %w", err)
	}
	if err := s.scheduler.Schedule(ctx, submission.ID); err != nil {
		return nil, s.rollbackSubmission(ctx, submission.ID, err)
	}

	return &domain.StandupPreview{
		SubmissionID: submission.ID,
		TeamID:       submission.TeamID,
		StandupDate:  submission.StandupDate,
		Format:       submission.Format,
		Done:         done,
		Plans:        plans,
		Blockers:     blockers,
	}, nil
}

func (s *Service) rollbackSubmission(ctx context.Context, submissionID uuid.UUID, scheduleErr error) error {
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
	defer cancel()

	if err := s.repository.DeleteSubmission(rollbackCtx, submissionID); err != nil {
		return fmt.Errorf("parser: schedule submission: %w; delete unscheduled submission: %v", scheduleErr, err)
	}
	return fmt.Errorf("parser: schedule submission: %w", scheduleErr)
}

func localStandupDate(now time.Time, timezone string) (time.Time, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, fmt.Errorf("parser: invalid team timezone %q: %w", timezone, err)
	}
	localTime := now.In(location)
	return time.Date(localTime.Year(), localTime.Month(), localTime.Day(), 0, 0, 0, 0, time.UTC), nil
}

func stringPointer(value string) *string {
	return &value
}
