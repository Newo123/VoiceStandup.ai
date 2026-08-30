// Package miniapp содержит сценарии, доступные в Telegram Mini App.
package miniapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"VoiceStandup.ai/internal/core/domain"
	"github.com/google/uuid"
)

var (
	ErrUserNotFound = errors.New("mini app user not found")
	ErrForbidden    = errors.New("mini app team access forbidden")
	ErrInvalidTeam  = errors.New("invalid team data")
	ErrTeamChatUsed = errors.New("Telegram chat already belongs to a team")
)

type Repository interface {
	GetActiveUserByTelegramID(ctx context.Context, telegramUserID int64) (*domain.Users, error)
	GetUserStats(ctx context.Context, userID uuid.UUID) (*domain.UserStats, error)
	GetTeamByUUID(ctx context.Context, teamID uuid.UUID) (*domain.Teams, error)
	GetTeamByTelegramChatID(ctx context.Context, chatID int64) (*domain.Teams, error)
	GetTeamsByUserID(ctx context.Context, userID uuid.UUID) ([]domain.TeamMembership, error)
	GetTeamMembership(ctx context.Context, userID uuid.UUID, teamID uuid.UUID) (*domain.TeamMembership, error)
	GetTeamMemberStats(ctx context.Context, teamID uuid.UUID) ([]domain.TeamMemberStats, error)
	CreateTeamForOwner(ctx context.Context, owner *domain.Users, team *domain.Teams) error
	SetActiveTeam(ctx context.Context, user *domain.Users, teamID uuid.UUID) error
	UpdateTeam(ctx context.Context, team *domain.Teams) error
	GetSubmissionsByUserID(ctx context.Context, userID uuid.UUID) ([]domain.Submissions, error)
	GetSubmissionByID(ctx context.Context, submissionID uuid.UUID) (*domain.Submissions, error)
	GetAllUsers(ctx context.Context) ([]domain.Users, error)
	GetUserByID(ctx context.Context, userID uuid.UUID) (*domain.Users, error)
}

type Profile struct {
	User       domain.Users
	Stats      domain.UserStats
	ActiveTeam *domain.Teams
}

type CreateTeamInput struct {
	Name             string
	TelegramChatID   int64
	Timezone         string
	PublishLocalTime time.Time
	Workdays         []int
	LatePolicy       string
}

type UpdateTeamInput struct {
	Name             *string
	Timezone         *string
	PublishLocalTime *time.Time
	Workdays         *[]int
	LatePolicy       *string
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("mini app: repository is required")
	}
	return &Service{repository: repository}, nil
}

func (s *Service) GetProfile(ctx context.Context, telegramUserID int64) (*Profile, error) {
	user, err := s.getUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}

	stats, err := s.repository.GetUserStats(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("mini app: get user stats: %w", err)
	}
	if stats == nil {
		stats = &domain.UserStats{UserID: user.ID, Level: 1}
	}

	var activeTeam *domain.Teams
	if user.ActiveTeamID != nil {
		activeTeam, err = s.repository.GetTeamByUUID(ctx, *user.ActiveTeamID)
		if err != nil {
			return nil, fmt.Errorf("mini app: get active team: %w", err)
		}
	}
	return &Profile{User: *user, Stats: *stats, ActiveTeam: activeTeam}, nil
}

func (s *Service) ListTeams(ctx context.Context, telegramUserID int64) ([]domain.TeamMembership, error) {
	user, err := s.getUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	memberships, err := s.repository.GetTeamsByUserID(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("mini app: list teams: %w", err)
	}
	return memberships, nil
}

func (s *Service) CreateTeam(
	ctx context.Context,
	telegramUserID int64,
	input CreateTeamInput,
) (*domain.TeamMembership, error) {
	user, err := s.getUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	team, err := teamFromInput(input)
	if err != nil {
		return nil, err
	}
	existingTeam, err := s.repository.GetTeamByTelegramChatID(ctx, team.TelegramChatID)
	if err != nil {
		return nil, fmt.Errorf("mini app: check Telegram chat: %w", err)
	}
	if existingTeam != nil {
		return nil, ErrTeamChatUsed
	}
	if err := s.repository.CreateTeamForOwner(ctx, user, team); err != nil {
		return nil, fmt.Errorf("mini app: create team: %w", err)
	}
	return &domain.TeamMembership{Team: *team, IsOwner: true}, nil
}

func (s *Service) SelectActiveTeam(
	ctx context.Context,
	telegramUserID int64,
	teamID uuid.UUID,
) (*domain.TeamMembership, error) {
	user, err := s.getUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	membership, err := s.repository.GetTeamMembership(ctx, user.ID, teamID)
	if err != nil {
		return nil, fmt.Errorf("mini app: get team membership: %w", err)
	}
	if membership == nil {
		return nil, ErrForbidden
	}
	if err := s.repository.SetActiveTeam(ctx, user, teamID); err != nil {
		return nil, fmt.Errorf("mini app: select active team: %w", err)
	}
	return membership, nil
}

func (s *Service) ListUsers(ctx context.Context) ([]domain.Users, error) {
	users, err := s.repository.GetAllUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("mini app: list users: %w", err)
	}
	return users, nil
}

func (s *Service) GetUserByID(ctx context.Context, userID uuid.UUID) (*domain.Users, error) {
	user, err := s.repository.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("mini app: get user by id: %w", err)
	}
	if user == nil || user.DeletedAt != nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}

func (s *Service) ListReports(ctx context.Context, telegramUserID int64) ([]domain.Submissions, error) {
	user, err := s.getUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	submissions, err := s.repository.GetSubmissionsByUserID(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("mini app: list reports: %w", err)
	}
	return submissions, nil
}

func (s *Service) GetReport(ctx context.Context, telegramUserID int64, submissionID uuid.UUID) (*domain.Submissions, error) {
	user, err := s.getUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	submission, err := s.repository.GetSubmissionByID(ctx, submissionID)
	if err != nil {
		return nil, fmt.Errorf("mini app: get report: %w", err)
	}
	if submission == nil {
		return nil, nil
	}
	if submission.UserID != user.ID {
		return nil, ErrForbidden
	}
	return submission, nil
}

func (s *Service) GetTeam(ctx context.Context, telegramUserID int64, teamID uuid.UUID) (*domain.TeamMembership, error) {
	user, err := s.getUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	membership, err := s.repository.GetTeamMembership(ctx, user.ID, teamID)
	if err != nil {
		return nil, fmt.Errorf("mini app: get team: %w", err)
	}
	if membership == nil {
		return nil, ErrForbidden
	}
	return membership, nil
}

func (s *Service) UpdateTeam(
	ctx context.Context,
	telegramUserID int64,
	teamID uuid.UUID,
	input UpdateTeamInput,
) (*domain.TeamMembership, error) {
	user, err := s.getUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	membership, err := s.repository.GetTeamMembership(ctx, user.ID, teamID)
	if err != nil {
		return nil, fmt.Errorf("mini app: get team membership: %w", err)
	}
	if membership == nil {
		return nil, ErrForbidden
	}
	if !membership.IsOwner {
		return nil, ErrForbidden
	}

	team := &membership.Team
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 100 {
			return nil, fmt.Errorf("%w: team name must contain 2 to 100 characters", ErrInvalidTeam)
		}
		team.Name = name
	}
	if input.Timezone != nil {
		timezone := strings.TrimSpace(*input.Timezone)
		if _, err := time.LoadLocation(timezone); err != nil {
			return nil, fmt.Errorf("%w: invalid timezone", ErrInvalidTeam)
		}
		team.Timezone = timezone
	}
	if input.PublishLocalTime != nil {
		team.PublishLocalTime = *input.PublishLocalTime
	}
	if input.Workdays != nil {
		workdays, err := validateWorkdays(*input.Workdays)
		if err != nil {
			return nil, err
		}
		team.Workdays = workdays
	}
	if input.LatePolicy != nil {
		latePolicy := strings.TrimSpace(*input.LatePolicy)
		if latePolicy != domain.LatePolicyNextDigest && latePolicy != domain.LatePolicySeparateMessage {
			return nil, fmt.Errorf("%w: unsupported late policy", ErrInvalidTeam)
		}
		team.LatePolicy = latePolicy
	}

	if err := s.repository.UpdateTeam(ctx, team); err != nil {
		return nil, fmt.Errorf("mini app: update team: %w", err)
	}
	return membership, nil
}

func (s *Service) GetTeamMembers(
	ctx context.Context,
	telegramUserID int64,
	teamID uuid.UUID,
) ([]domain.TeamMemberStats, error) {
	user, err := s.getUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	membership, err := s.repository.GetTeamMembership(ctx, user.ID, teamID)
	if err != nil {
		return nil, fmt.Errorf("mini app: get team membership: %w", err)
	}
	if membership == nil {
		return nil, ErrForbidden
	}
	members, err := s.repository.GetTeamMemberStats(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("mini app: get team members: %w", err)
	}
	return members, nil
}

func (s *Service) getUser(ctx context.Context, telegramUserID int64) (*domain.Users, error) {
	user, err := s.repository.GetActiveUserByTelegramID(ctx, telegramUserID)
	if err != nil {
		return nil, fmt.Errorf("mini app: get user: %w", err)
	}
	if user == nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}

func teamFromInput(input CreateTeamInput) (*domain.Teams, error) {
	name := strings.TrimSpace(input.Name)
	if utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 100 {
		return nil, fmt.Errorf("%w: team name must contain 2 to 100 characters", ErrInvalidTeam)
	}
	if input.TelegramChatID == 0 {
		return nil, fmt.Errorf("%w: Telegram chat ID is required", ErrInvalidTeam)
	}
	timezone := strings.TrimSpace(input.Timezone)
	if timezone == "" {
		timezone = "Europe/Moscow"
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return nil, fmt.Errorf("%w: invalid timezone", ErrInvalidTeam)
	}
	workdays, err := validateWorkdays(input.Workdays)
	if err != nil {
		return nil, err
	}
	latePolicy := strings.TrimSpace(input.LatePolicy)
	if latePolicy == "" {
		latePolicy = domain.LatePolicyNextDigest
	}
	if latePolicy != domain.LatePolicyNextDigest && latePolicy != domain.LatePolicySeparateMessage {
		return nil, fmt.Errorf("%w: unsupported late policy", ErrInvalidTeam)
	}

	return &domain.Teams{
		Name:             name,
		TelegramChatID:   input.TelegramChatID,
		Timezone:         timezone,
		PublishLocalTime: input.PublishLocalTime,
		Workdays:         workdays,
		LatePolicy:       latePolicy,
	}, nil
}

func validateWorkdays(workdays []int) ([]int, error) {
	if len(workdays) == 0 {
		return []int{1, 2, 3, 4, 5}, nil
	}
	seen := make(map[int]struct{}, len(workdays))
	result := make([]int, 0, len(workdays))
	for _, workday := range workdays {
		if workday < 1 || workday > 7 {
			return nil, fmt.Errorf("%w: workday must be between 1 and 7", ErrInvalidTeam)
		}
		if _, duplicate := seen[workday]; duplicate {
			return nil, fmt.Errorf("%w: duplicate workday", ErrInvalidTeam)
		}
		seen[workday] = struct{}{}
		result = append(result, workday)
	}
	return result, nil
}
