package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"time"

	"VoiceStandup.ai/internal/core/domain"
	"VoiceStandup.ai/internal/standup/miniapp"
	"github.com/google/uuid"
)

const maxJSONBodySize = 64 << 10

type MiniAppService interface {
	GetProfile(ctx context.Context, telegramUserID int64) (*miniapp.Profile, error)
	ListTeams(ctx context.Context, telegramUserID int64) ([]domain.TeamMembership, error)
	CreateTeam(ctx context.Context, telegramUserID int64, input miniapp.CreateTeamInput) (*domain.TeamMembership, error)
	SelectActiveTeam(ctx context.Context, telegramUserID int64, teamID uuid.UUID) (*domain.TeamMembership, error)
	GetTeam(ctx context.Context, telegramUserID int64, teamID uuid.UUID) (*domain.TeamMembership, error)
	UpdateTeam(ctx context.Context, telegramUserID int64, teamID uuid.UUID, input miniapp.UpdateTeamInput) (*domain.TeamMembership, error)
	GetTeamMembers(ctx context.Context, telegramUserID int64, teamID uuid.UUID) ([]domain.TeamMemberStats, error)
	ListReports(ctx context.Context, telegramUserID int64) ([]domain.Submissions, error)
	GetReport(ctx context.Context, telegramUserID int64, submissionID uuid.UUID) (*domain.Submissions, error)
	ListUsers(ctx context.Context) ([]domain.Users, error)
	GetUserByID(ctx context.Context, userID uuid.UUID) (*domain.Users, error)
}

type API struct {
	service MiniAppService
	logger  *slog.Logger
}

func NewHandler(validator *InitDataValidator, service MiniAppService) (http.Handler, error) {
	if validator == nil {
		return nil, fmt.Errorf("HTTP API: Telegram validator is required")
	}
	if service == nil {
		return nil, fmt.Errorf("HTTP API: Mini App service is required")
	}

	api := &API{service: service, logger: slog.Default().With("component", "mini_app_http")}
	protected := http.NewServeMux()
	// Users
	protected.HandleFunc("GET /api/v1/me", api.getProfile)
	protected.HandleFunc("GET /api/v1/users", api.listUsers)
	protected.HandleFunc("GET /api/v1/users/{userID}", api.getUser)
	protected.HandleFunc("GET /api/v1/teams", api.listTeams)
	// Teams
	protected.HandleFunc("POST /api/v1/teams", api.createTeam)
	//protected.HandleFunc("PUT /api/v1/me/active-team", api.selectActiveTeam)
	protected.HandleFunc("GET /api/v1/teams/{teamID}", api.getTeam)
	protected.HandleFunc("PATCH /api/v1/teams/{teamID}", api.updateTeam)
	protected.HandleFunc("GET /api/v1/teams/{teamID}/members", api.getTeamMembers)
	// Reports
	protected.HandleFunc("GET /api/v1/reports", api.listReports)
	protected.HandleFunc("GET /api/v1/reports/{reportID}", api.getReport)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(response http.ResponseWriter, _ *http.Request) {
		writeJSON(response, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.Handle("/api/", AuthMiddleware(validator)(protected))
	return api.recoverPanics(mux), nil
}

func (a *API) getProfile(response http.ResponseWriter, request *http.Request) {
	telegramUser := mustTelegramUser(request)
	profile, err := a.service.GetProfile(request.Context(), telegramUser.ID)
	if err != nil {
		a.writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, profileResponseFromDomain(profile))
}

func (a *API) listTeams(response http.ResponseWriter, request *http.Request) {
	telegramUser := mustTelegramUser(request)
	memberships, err := a.service.ListTeams(request.Context(), telegramUser.ID)
	if err != nil {
		a.writeServiceError(response, err)
		return
	}

	teams := make([]teamResponse, 0, len(memberships))
	for _, membership := range memberships {
		teams = append(teams, teamResponseFromDomain(membership))
	}
	writeJSON(response, http.StatusOK, map[string]any{"teams": teams})
}

func (a *API) createTeam(response http.ResponseWriter, request *http.Request) {
	var input createTeamRequest
	if err := decodeJSON(response, request, &input); err != nil {
		writeError(response, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	publishTime, err := time.Parse("15:04", input.PublishLocalTime)
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "invalid_team", "publish_local_time должен быть в формате HH:MM")
		return
	}

	telegramUser := mustTelegramUser(request)
	membership, err := a.service.CreateTeam(request.Context(), telegramUser.ID, miniapp.CreateTeamInput{
		Name:             input.Name,
		TelegramChatID:   input.TelegramChatID,
		Timezone:         input.Timezone,
		PublishLocalTime: publishTime,
		Workdays:         input.Workdays,
		LatePolicy:       input.LatePolicy,
	})
	if err != nil {
		a.writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, teamResponseFromDomain(*membership))
}

func (a *API) selectActiveTeam(response http.ResponseWriter, request *http.Request) {
	var input selectActiveTeamRequest
	if err := decodeJSON(response, request, &input); err != nil {
		writeError(response, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	teamID, err := uuid.Parse(input.TeamID)
	if err != nil {
		writeError(response, http.StatusBadRequest, "invalid_team_id", "Некорректный team_id")
		return
	}

	telegramUser := mustTelegramUser(request)
	membership, err := a.service.SelectActiveTeam(request.Context(), telegramUser.ID, teamID)
	if err != nil {
		a.writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, teamResponseFromDomain(*membership))
}

func (a *API) listUsers(response http.ResponseWriter, request *http.Request) {
	users, err := a.service.ListUsers(request.Context())
	if err != nil {
		a.writeServiceError(response, err)
		return
	}

	result := make([]userResponse, 0, len(users))
	for _, user := range users {
		result = append(result, userResponseFromDomain(user))
	}
	writeJSON(response, http.StatusOK, map[string]any{"users": result})
}

func (a *API) getUser(response http.ResponseWriter, request *http.Request) {
	userID, err := uuid.Parse(request.PathValue("userID"))
	if err != nil {
		writeError(response, http.StatusBadRequest, "invalid_user_id", "Некорректный user ID")
		return
	}

	user, err := a.service.GetUserByID(request.Context(), userID)
	if err != nil {
		a.writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, userResponseFromDomain(*user))
}

func (a *API) getTeam(response http.ResponseWriter, request *http.Request) {
	teamID, err := uuid.Parse(request.PathValue("teamID"))
	if err != nil {
		writeError(response, http.StatusBadRequest, "invalid_team_id", "Некорректный team ID")
		return
	}

	telegramUser := mustTelegramUser(request)
	membership, err := a.service.GetTeam(request.Context(), telegramUser.ID, teamID)
	if err != nil {
		a.writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, teamResponseFromDomain(*membership))
}

func (a *API) updateTeam(response http.ResponseWriter, request *http.Request) {
	teamID, err := uuid.Parse(request.PathValue("teamID"))
	if err != nil {
		writeError(response, http.StatusBadRequest, "invalid_team_id", "Некорректный team ID")
		return
	}

	var input updateTeamRequest
	if err := decodeJSON(response, request, &input); err != nil {
		writeError(response, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	updateInput := miniapp.UpdateTeamInput{
		Name:       input.Name,
		Timezone:   input.Timezone,
		Workdays:   input.Workdays,
		LatePolicy: input.LatePolicy,
	}
	if input.PublishLocalTime != nil {
		publishTime, err := time.Parse("15:04", *input.PublishLocalTime)
		if err != nil {
			writeError(response, http.StatusUnprocessableEntity, "invalid_team", "publish_local_time должен быть в формате HH:MM")
			return
		}
		updateInput.PublishLocalTime = &publishTime
	}

	telegramUser := mustTelegramUser(request)
	membership, err := a.service.UpdateTeam(request.Context(), telegramUser.ID, teamID, updateInput)
	if err != nil {
		a.writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, teamResponseFromDomain(*membership))
}

func (a *API) listReports(response http.ResponseWriter, request *http.Request) {
	telegramUser := mustTelegramUser(request)
	reports, err := a.service.ListReports(request.Context(), telegramUser.ID)
	if err != nil {
		a.writeServiceError(response, err)
		return
	}

	result := make([]reportResponse, 0, len(reports))
	for _, report := range reports {
		result = append(result, reportResponseFromDomain(report))
	}
	writeJSON(response, http.StatusOK, map[string]any{"reports": result})
}

func (a *API) getReport(response http.ResponseWriter, request *http.Request) {
	reportID, err := uuid.Parse(request.PathValue("reportID"))
	if err != nil {
		writeError(response, http.StatusBadRequest, "invalid_report_id", "Некорректный report ID")
		return
	}

	telegramUser := mustTelegramUser(request)
	report, err := a.service.GetReport(request.Context(), telegramUser.ID, reportID)
	if err != nil {
		a.writeServiceError(response, err)
		return
	}
	if report == nil {
		writeError(response, http.StatusNotFound, "report_not_found", "Отчёт не найден")
		return
	}
	writeJSON(response, http.StatusOK, reportResponseFromDomain(*report))
}

func (a *API) getTeamMembers(response http.ResponseWriter, request *http.Request) {
	teamID, err := uuid.Parse(request.PathValue("teamID"))
	if err != nil {
		writeError(response, http.StatusBadRequest, "invalid_team_id", "Некорректный team ID")
		return
	}

	telegramUser := mustTelegramUser(request)
	members, err := a.service.GetTeamMembers(request.Context(), telegramUser.ID, teamID)
	if err != nil {
		a.writeServiceError(response, err)
		return
	}

	result := make([]teamMemberResponse, 0, len(members))
	for _, member := range members {
		result = append(result, teamMemberResponseFromDomain(member))
	}
	writeJSON(response, http.StatusOK, map[string]any{"members": result})
}

func (a *API) writeServiceError(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, miniapp.ErrUserNotFound):
		writeError(response, http.StatusNotFound, "user_not_found", "Пользователь не зарегистрирован в боте")
	case errors.Is(err, miniapp.ErrForbidden):
		writeError(response, http.StatusForbidden, "forbidden", "Нет доступа к этой команде")
	case errors.Is(err, miniapp.ErrInvalidTeam):
		writeError(response, http.StatusUnprocessableEntity, "invalid_team", "Некорректные настройки команды")
	case errors.Is(err, miniapp.ErrTeamChatUsed):
		writeError(response, http.StatusConflict, "team_chat_used", "Для этого Telegram-чата команда уже создана")
	default:
		a.logger.Error("Ошибка Mini App API", "error", err)
		writeError(response, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
	}
}

func (a *API) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				a.logger.Error("Паника в Mini App API", "method", request.Method, "path", request.URL.Path)
				writeError(response, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
			}
		}()
		next.ServeHTTP(response, request)
	})
}

func mustTelegramUser(request *http.Request) TelegramUser {
	user, ok := TelegramUserFromContext(request.Context())
	if !ok {
		panic("Telegram user is missing after auth middleware")
	}
	return user
}

func decodeJSON(response http.ResponseWriter, request *http.Request, target any) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return fmt.Errorf("Content-Type должен быть application/json")
	}
	request.Body = http.MaxBytesReader(response, request.Body, maxJSONBodySize)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("некорректный JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("тело должно содержать один JSON-объект")
	}
	return nil
}
