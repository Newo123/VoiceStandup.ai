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
	GetTeamMembers(ctx context.Context, telegramUserID int64, teamID uuid.UUID) ([]domain.TeamMemberStats, error)
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
	protected.HandleFunc("GET /api/v1/me", api.getProfile)
	protected.HandleFunc("GET /api/v1/teams", api.listTeams)
	protected.HandleFunc("POST /api/v1/teams", api.createTeam)
	protected.HandleFunc("PUT /api/v1/me/active-team", api.selectActiveTeam)
	protected.HandleFunc("GET /api/v1/teams/{teamID}/members", api.getTeamMembers)

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
