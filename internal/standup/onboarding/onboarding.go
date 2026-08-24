// 3.2. internal/standup/onboarding — Логика регистрации:
// обработка перехода по инвайт-ссылке,
// текстовый опрос роли пользователя без WebApp и привязка к team_id.

package onboarding

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	uuid "github.com/google/uuid"

	domain "VoiceStandup.ai/internal/core/domain"
)

type OnboardingService struct {
	repo   OnboardingRepo
	logger *slog.Logger
}

func NewOnboardingService(repo OnboardingRepo) *OnboardingService {
	return &OnboardingService{
		repo:   repo,
		logger: slog.Default().With("component", "onboard_service"),
	}
}

type OnboardingRepo interface {
	CreateUser(ctx context.Context, user *domain.Users) error
	UpdateUser(ctx context.Context, user *domain.Users) error

	SaveUserInTeam(ctx context.Context, user *domain.Users, teamID int64) error
	SaveUserRoleInTeam(ctx context.Context, role string, team *domain.Teams) error
	GetUserTeam(ctx context.Context, userID uuid.UUID) (*domain.Teams, error)

	GetUserState(ctx context.Context, user *domain.Users) (string, error)
	GetActiveUserByTelegramID(ctx context.Context, userID int64) (*domain.Users, error)
}

func (s *OnboardingService) StartOnboarding(ctx context.Context, req *domain.StandupTGBotTeamRequestDTO) (*domain.StandupTGBotResponseDTO, error) {

	user, err := s.repo.GetActiveUserByTelegramID(ctx, req.UserID)
	if err != nil {
		return nil, err
	}

	if user == nil {
		user = &domain.Users{
			TelegramUserID: req.UserID,
			Username:       req.Username,
			DisplayName:    "TODO " + req.Username,
			CreatedAt:      time.Now(),
		}
		if err := s.repo.CreateUser(ctx, user); err != nil {
			return nil, err
		}
	} else {
		if err := s.repo.UpdateUser(ctx, user); err != nil {
			return nil, err
		}
	}

	if req.TeamID != 0 {
		err = s.repo.SaveUserInTeam(ctx, user, req.TeamID)
		if err != nil {
			return nil, err
		}

		return s.generateResponse(req.ChatID, s.askRoleMessage(user)), nil
	} else {
		// написать что-то типа приветствия и как пользовать (что можно создать команду и тд)
		return s.generateResponse(req.ChatID, s.welcomeNoTeamMessage(user)), nil
	}

}

func (s *OnboardingService) SetUserRole(ctx context.Context, req *domain.StandupTGBotTextRequestDTO) (*domain.StandupTGBotResponseDTO, error) {

	user, err := s.repo.GetActiveUserByTelegramID(ctx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("не удалось найти пользователя: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("пользователь не найден")
	}

	team, err := s.repo.GetUserTeam(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("не удалось найти команду: %w", err)
	}
	if team == nil {
		return nil, fmt.Errorf("ты не состоишь ни в одной команде")
	}

	if err := s.repo.SaveUserRoleInTeam(ctx, req.Text, team); err != nil {
		return nil, err
	}

	return s.generateResponse(req.ChatID, s.roleSetSuccessMessage(user, team, req.Text)), nil
}

func (s *OnboardingService) GetUserState(ctx context.Context, userID int64) (string, error) {

	user, err := s.repo.GetActiveUserByTelegramID(ctx, userID)
	if err != nil {
		return "", err
	}

	if user == nil {
		return "", fmt.Errorf("Нет такого активного юзера в БД")
	}

	return s.repo.GetUserState(ctx, user)

}

func (s *OnboardingService) BotAddedToGroup(ctx context.Context, chatID int64) (*domain.StandupTGBotResponseDTO, error) {
	return s.generateResponse(chatID, s.botAddedToGroupMessage(chatID)), nil
}

func (s *OnboardingService) generateResponse(targetChatID int64, text string) *domain.StandupTGBotResponseDTO {
	return &domain.StandupTGBotResponseDTO{
		TargetChatID: targetChatID,
		Text:         text,
	}
}
