package httptransport

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

const testBotToken = "123456:test-token"

func TestInitDataValidatorAcceptsSignedData(t *testing.T) {
	now := time.Date(2026, time.August, 29, 12, 0, 0, 0, time.UTC)
	validator := newTestValidator(t, now)
	raw := signedInitData(testBotToken, now, `{"id":1001,"first_name":"Иван","username":"ivan"}`)

	user, err := validator.Validate(raw)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if user.ID != 1001 || user.Username != "ivan" || user.FirstName != "Иван" {
		t.Errorf("user = %+v", user)
	}
}

func TestInitDataValidatorRejectsTamperedData(t *testing.T) {
	now := time.Date(2026, time.August, 29, 12, 0, 0, 0, time.UTC)
	validator := newTestValidator(t, now)
	raw := signedInitData(testBotToken, now, `{"id":1001,"first_name":"Иван"}`)
	raw += "&query_id=changed"

	if _, err := validator.Validate(raw); err == nil {
		t.Fatal("Validate() error = nil")
	}
}

func TestInitDataValidatorRejectsExpiredData(t *testing.T) {
	now := time.Date(2026, time.August, 29, 12, 0, 0, 0, time.UTC)
	validator := newTestValidator(t, now)
	raw := signedInitData(testBotToken, now.Add(-6*time.Minute), `{"id":1001,"first_name":"Иван"}`)

	if _, err := validator.Validate(raw); err != ErrExpiredInitData {
		t.Fatalf("Validate() error = %v, want %v", err, ErrExpiredInitData)
	}
}

func TestAuthMiddlewareAddsTelegramUserToContext(t *testing.T) {
	now := time.Date(2026, time.August, 29, 12, 0, 0, 0, time.UTC)
	validator := newTestValidator(t, now)
	raw := signedInitData(testBotToken, now, `{"id":1001,"first_name":"Иван"}`)

	handler := AuthMiddleware(validator)(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		user, ok := TelegramUserFromContext(request.Context())
		if !ok || user.ID != 1001 {
			t.Fatalf("context user = %+v, ok = %t", user, ok)
		}
		response.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	request.Header.Set("Authorization", "tma "+raw)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestAuthMiddlewareRejectsMissingHeader(t *testing.T) {
	validator := newTestValidator(t, time.Now())
	handler := AuthMiddleware(validator)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler was called")
	}))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func newTestValidator(t *testing.T, now time.Time) *InitDataValidator {
	t.Helper()
	validator, err := NewInitDataValidator(testBotToken, 5*time.Minute)
	if err != nil {
		t.Fatalf("NewInitDataValidator() error = %v", err)
	}
	validator.now = func() time.Time { return now }
	return validator
}

func signedInitData(botToken string, authTime time.Time, userJSON string) string {
	values := url.Values{
		"auth_date": {strconv.FormatInt(authTime.Unix(), 10)},
		"query_id":  {"query-1"},
		"user":      {userJSON},
	}
	values.Set("hash", hex.EncodeToString(initDataHash(botToken, dataCheckString(values))))
	return values.Encode()
}

func Example() {
	fmt.Println("Authorization: tma <Telegram.WebApp.initData>")
	// Output: Authorization: tma <Telegram.WebApp.initData>
}
