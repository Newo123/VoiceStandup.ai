// 3.2. internal/standup/onboarding — Логика регистрации:
// обработка перехода по инвайт-ссылке,
// текстовый опрос роли пользователя без WebApp и привязка к team_id.

// TODO
// 1. DisplayName
// 2. статус добавить в бд — State пока хранится в коде, перенести в БД

package onboarding

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	uuid "github.com/google/uuid"

	domain "VoiceStandup.ai/internal/core/domain"
)

// OnboardingService — сервис онбординга новых пользователей.
// Отвечает за регистрацию, привязку к команде и опрос роли.
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

// OnboardingRepo — интерфейс хранилища для онбординга.
type OnboardingRepo interface {
	// CreateUser — создаёт нового пользователя в БД.
	CreateUser(ctx context.Context, user *domain.Users) error

	// UpdateUser — обновляет существующего пользователя (username, display_name).
	// Обновление происходит по ID пользователя.
	UpdateUser(ctx context.Context, user *domain.Users) error

	// GetActiveUserByTelegramID — возвращает активного (не удалённого) пользователя
	// по его TelegramUserID. Если пользователь не найден — возвращает (nil, nil).
	GetActiveUserByTelegramID(ctx context.Context, userID int64) (*domain.Users, error)

	// SaveUserInTeamByChatID — добавляет пользователя в команду.
	// Создаёт запись в team_members со статусом "active" и is_owner = false.
	// Если пользователь уже в команде — возвращает ошибку.
	SaveUserInTeamByChatID(ctx context.Context, userID uuid.UUID, teamID uuid.UUID) error

	// SaveUserRoleInTeam — сохраняет/обновляет роль пользователя в команде.
	SaveUserRoleInTeam(ctx context.Context, userID uuid.UUID, teamID uuid.UUID, role string) error

	// GetTeamByUUID — возвращает команду по её UUID.
	// Если команда не найдена — возвращает (nil, nil).
	GetTeamByUUID(ctx context.Context, teamID uuid.UUID) (*domain.Teams, error)

	// GetTeamByTelegramChatID — возвращает команду по Telegram ChatID группы.
	// Если команда не найдена — возвращает (nil, nil).
	GetTeamByTelegramChatID(ctx context.Context, chatID int64) (*domain.Teams, error)

	// SaveUserStats — создаёт или обновляет статистику пользователя (XP, level, streak).
	SaveUserStats(ctx context.Context, stats *domain.UserStats) error

	// SetUserState — устанавливает состояние пользователя (FSM).
	// Состояние хранится в поле state таблицы users.
	// Возможные значения: "" (StateNone), "onboarded" (StateOnboarded),
	// "awaiting_role:<uuid>" (StatePrefixAwaitingRole).
	SetUserState(ctx context.Context, user *domain.Users, state string) error
}

// StartOnboarding — обработка /start.
// Если TeamID != 0 — пользователь перешёл по инвайт-ссылке:
// создаём/обновляем пользователя, привязываем к команде, устанавливаем состояние awaiting_role.
// Если TeamID == 0 — просто приветствуем без команды.
func (s *OnboardingService) StartOnboarding(ctx context.Context, req *domain.StandupTGBotTeamRequestDTO) (*domain.StandupTGBotResponseDTO, error) {

	user, err := s.repo.GetActiveUserByTelegramID(ctx, req.UserID)
	if err != nil {
		s.logger.Error("ошибка получения пользователя", "userID", req.UserID, "error", err)
		return nil, err
	}

	if user == nil {
		// Новый пользователь — создаём запись
		user = &domain.Users{
			TelegramUserID: req.UserID,
			Username:       req.Username,
			DisplayName:    req.Username, // TODO: дать возможность сменить
			CreatedAt:      time.Now(),
		}
		if err := s.repo.CreateUser(ctx, user); err != nil {
			s.logger.Error("ошибка создания пользователя", "userID", req.UserID, "error", err)
			return nil, err
		}

		// Создаём начальную статистику для нового пользователя
		stats := &domain.UserStats{
			UserID: user.ID,
			XP:     0,
			Level:  1,
		}
		if err := s.repo.SaveUserStats(ctx, stats); err != nil {
			s.logger.Error("ошибка создания статистики пользователя", "userID", req.UserID, "error", err)
			return nil, err
		}
	} else {
		user.Username = req.Username
		user.DisplayName = req.Username

		// Существующий пользователь — обновляем username
		if err := s.repo.UpdateUser(ctx, user); err != nil {
			s.logger.Error("ошибка обновления пользователя", "userID", req.UserID, "error", err)
			return nil, err
		}
	}

	if req.TeamID != 0 {
		// Получаем команду, чтобы узнать её UUID для состояния
		team, err := s.repo.GetTeamByTelegramChatID(ctx, req.TeamID)
		if err != nil {
			s.logger.Error("ошибка получения команды по chatID", "chatID", req.TeamID, "error", err)
			return nil, err
		}
		if team == nil {
			s.logger.Error("команда не найдена по chatID", "chatID", req.TeamID)
			return nil, fmt.Errorf("команда с chatID %d не найдена", req.TeamID)
		}

		// Привязываем пользователя к команде по Telegram ChatID
		if err := s.repo.SaveUserInTeamByChatID(ctx, user.ID, team.ID); err != nil {
			s.logger.Error("ошибка сохранения пользователя в команду", "userID", req.UserID, "teamChatID", req.TeamID, "error", err)
			return nil, err
		}

		// Устанавливаем состояние "ждём роль для этой команды"
		if err := s.repo.SetUserState(ctx, user, domain.StatePrefixAwaitingRole+":"+team.ID.String()); err != nil {
			s.logger.Error("ошибка установки состояния awaiting_role", "userID", req.UserID, "error", err)
			return nil, err
		}

		return s.generateResponse(req.ChatID, askRoleMessage(user)), nil
	}

	// Пользователь без команды — показываем приветствие
	return s.generateResponse(req.ChatID, welcomeNoTeamMessage(user)), nil
}

// SetUserRole — сохраняет роль пользователя в команде.
// Вызывается, когда пользователь в состоянии awaiting_role:<uuid> пишет текст.
// Парсит UUID команды из состояния, сохраняет роль и переводит состояние в onboarded.
func (s *OnboardingService) SetUserRole(ctx context.Context, req *domain.StandupTGBotTextRequestDTO) (*domain.StandupTGBotResponseDTO, error) {

	if len(strings.TrimSpace(req.Text)) == 0 {
		return s.generateResponse(req.ChatID,
			"Пожалуйста, напиши свою роль. Например: разработчик, дизайнер, продакт-менеджер."), nil
	}

	user, err := s.repo.GetActiveUserByTelegramID(ctx, req.UserID)
	if err != nil {
		s.logger.Error("ошибка получения пользователя", "userID", req.UserID, "error", err)
		return nil, err
	}
	if user == nil {
		// Пользователь не найден — нет ошибки репы, просто нет записи
		return nil, fmt.Errorf("пользователь не найден")
	}

	// Парсим teamUUID из состояния "awaiting_role:<uuid>"
	teamUUID, err := parseTeamUUIDFromState(user.State)
	if err != nil {
		s.logger.Error("неверный формат состояния", "state", user.State, "userID", req.UserID, "error", err)
		return nil, err
	}

	// Получаем команду по UUID
	team, err := s.repo.GetTeamByUUID(ctx, teamUUID)
	if err != nil {
		s.logger.Error("ошибка получения команды по UUID", "teamUUID", teamUUID, "error", err)
		return nil, err
	}
	if team == nil {
		s.logger.Error("команда не найдена по UUID", "teamUUID", teamUUID)
		return nil, fmt.Errorf("команда с UUID %s не найдена", teamUUID)
	}

	// Сохраняем роль в team_members
	if err := s.repo.SaveUserRoleInTeam(ctx, user.ID, team.ID, req.Text); err != nil {
		s.logger.Error("ошибка сохранения роли", "userID", req.UserID, "role", req.Text, "teamID", team.ID, "error", err)
		return nil, err
	}

	// Онбординг завершён — переводим состояние в onboarded
	if err := s.repo.SetUserState(ctx, user, domain.StateOnboarded); err != nil {
		s.logger.Error("ошибка установки состояния onboarded", "userID", req.UserID, "error", err)
		return nil, err
	}

	return s.generateResponse(req.ChatID, roleSetSuccessMessage(user, team, req.Text)), nil
}

// GetUserState — возвращает текущее состояние пользователя для роутера.
// Используется в route() для выбора следующего хэндлера.
func (s *OnboardingService) GetUserState(ctx context.Context, userID int64) (string, error) {

	user, err := s.repo.GetActiveUserByTelegramID(ctx, userID)
	if err != nil {
		s.logger.Error("ошибка получения пользователя для состояния", "userID", userID, "error", err)
		return "", err
	}

	if user == nil {
		return "", fmt.Errorf("нет такого активного пользователя в БД")
	}

	return user.State, nil
}

// BotAddedToGroup — формирует приветственное сообщение при добавлении бота в групповой чат.
// Возвращает ChatID, который нужен для создания команды.
func (s *OnboardingService) BotAddedToGroup(ctx context.Context, chatID int64) (*domain.StandupTGBotResponseDTO, error) {
	return s.generateResponse(chatID, botAddedToGroupMessage(chatID)), nil
}

// generateResponse — утилита для создания ответного DTO.
func (s *OnboardingService) generateResponse(targetChatID int64, text string) *domain.StandupTGBotResponseDTO {
	return &domain.StandupTGBotResponseDTO{
		TargetChatID: targetChatID,
		Text:         text,
	}
}

// parseTeamUUIDFromState извлекает UUID из строки "awaiting_role:550e8400-e29b-41d4-a716-446655440000".
func parseTeamUUIDFromState(state string) (uuid.UUID, error) {
	parts := strings.SplitN(state, ":", 2)
	if len(parts) != 2 || parts[0] != domain.StatePrefixAwaitingRole {
		return uuid.Nil, fmt.Errorf("неверный формат состояния: %s", state)
	}
	return uuid.Parse(parts[1])
}
