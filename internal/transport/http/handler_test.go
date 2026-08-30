package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"VoiceStandup.ai/internal/core/domain"
	"VoiceStandup.ai/internal/standup/miniapp"
	"github.com/google/uuid"
)

func TestHandlerHealthDoesNotRequireAuthorization(t *testing.T) {
	handler := newTestHandler(t, &fakeMiniAppService{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestHandlerReturnsProfile(t *testing.T) {
	userID := uuid.New()
	service := &fakeMiniAppService{profile: &miniapp.Profile{
		User:  domain.Users{ID: userID, TelegramUserID: 1001, Username: "ivan"},
		Stats: domain.UserStats{XP: 120, Level: 2},
	}}
	handler := newTestHandler(t, service)
	response := performAuthorizedRequest(t, handler, http.MethodGet, "/api/v1/me", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload profileResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.User.ID != userID.String() || payload.Stats.XP != 120 || service.telegramUserID != 1001 {
		t.Errorf("payload = %+v, Telegram user ID = %d", payload, service.telegramUserID)
	}
}

func TestHandlerCreatesTeam(t *testing.T) {
	teamID := uuid.New()
	service := &fakeMiniAppService{created: &domain.TeamMembership{
		Team:    domain.Teams{ID: teamID, Name: "Backend", PublishLocalTime: time.Date(0, 1, 1, 10, 30, 0, 0, time.UTC)},
		IsOwner: true,
	}}
	handler := newTestHandler(t, service)
	body := bytes.NewBufferString(`{
		"name":"Backend",
		"telegram_chat_id":-1001,
		"timezone":"Europe/Moscow",
		"publish_local_time":"10:30",
		"workdays":[1,2,3,4,5],
		"late_policy":"NEXT_DIGEST"
	}`)
	response := performAuthorizedRequest(t, handler, http.MethodPost, "/api/v1/teams", body)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if service.createInput.Name != "Backend" || service.createInput.PublishLocalTime.Hour() != 10 {
		t.Errorf("create input = %+v", service.createInput)
	}
}

func TestHandlerRejectsUnknownJSONField(t *testing.T) {
	handler := newTestHandler(t, &fakeMiniAppService{})
	body := bytes.NewBufferString(`{"team_id":"` + uuid.NewString() + `","unexpected":true}`)
	response := performAuthorizedRequest(t, handler, http.MethodPut, "/api/v1/me/active-team", body)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestHandlerMapsForbiddenError(t *testing.T) {
	service := &fakeMiniAppService{membersErr: miniapp.ErrForbidden}
	handler := newTestHandler(t, service)
	response := performAuthorizedRequest(
		t,
		handler,
		http.MethodGet,
		"/api/v1/teams/"+uuid.NewString()+"/members",
		nil,
	)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestHandlerRequiresTelegramAuthorization(t *testing.T) {
	handler := newTestHandler(t, &fakeMiniAppService{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func newTestHandler(t *testing.T, service MiniAppService) http.Handler {
	t.Helper()
	now := time.Date(2026, time.August, 29, 12, 0, 0, 0, time.UTC)
	validator := newTestValidator(t, now)
	handler, err := NewHandler(validator, service)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	return handler
}

func performAuthorizedRequest(
	t *testing.T,
	handler http.Handler,
	method string,
	path string,
	body *bytes.Buffer,
) *httptest.ResponseRecorder {
	t.Helper()
	if body == nil {
		body = &bytes.Buffer{}
	}
	request := httptest.NewRequest(method, path, body)
	if method == http.MethodPost || method == http.MethodPut {
		request.Header.Set("Content-Type", "application/json")
	}
	authTime := time.Date(2026, time.August, 29, 12, 0, 0, 0, time.UTC)
	request.Header.Set("Authorization", "tma "+signedInitData(testBotToken, authTime, `{"id":1001,"first_name":"Иван"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

type fakeMiniAppService struct {
	profile        *miniapp.Profile
	profileErr     error
	teams          []domain.TeamMembership
	created        *domain.TeamMembership
	createInput    miniapp.CreateTeamInput
	selected       *domain.TeamMembership
	members        []domain.TeamMemberStats
	membersErr     error
	telegramUserID int64
}

func (s *fakeMiniAppService) GetProfile(_ context.Context, telegramUserID int64) (*miniapp.Profile, error) {
	s.telegramUserID = telegramUserID
	return s.profile, s.profileErr
}
func (s *fakeMiniAppService) ListTeams(context.Context, int64) ([]domain.TeamMembership, error) {
	return s.teams, nil
}
func (s *fakeMiniAppService) CreateTeam(
	_ context.Context,
	_ int64,
	input miniapp.CreateTeamInput,
) (*domain.TeamMembership, error) {
	s.createInput = input
	return s.created, nil
}
func (s *fakeMiniAppService) SelectActiveTeam(context.Context, int64, uuid.UUID) (*domain.TeamMembership, error) {
	return s.selected, nil
}
func (s *fakeMiniAppService) GetTeamMembers(context.Context, int64, uuid.UUID) ([]domain.TeamMemberStats, error) {
	return s.members, s.membersErr
}

var _ MiniAppService = (*fakeMiniAppService)(nil)
