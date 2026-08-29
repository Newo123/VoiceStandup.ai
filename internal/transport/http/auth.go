package httptransport

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	authorizationScheme = "tma"
	maxInitDataLength   = 16 << 10
	allowedClockSkew    = 30 * time.Second
)

var (
	ErrInvalidInitData = errors.New("invalid Telegram init data")
	ErrExpiredInitData = errors.New("Telegram init data expired")
	ErrUnauthorized    = errors.New("Telegram authorization is required")
)

type TelegramUser struct {
	ID           int64  `json:"id"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name,omitempty"`
	Username     string `json:"username,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
	IsBot        bool   `json:"is_bot,omitempty"`
}

type InitDataValidator struct {
	botToken string
	maxAge   time.Duration
	now      func() time.Time
}

func NewInitDataValidator(botToken string, maxAge time.Duration) (*InitDataValidator, error) {
	if strings.TrimSpace(botToken) == "" {
		return nil, fmt.Errorf("Telegram bot token is required")
	}
	if maxAge <= 0 {
		return nil, fmt.Errorf("Telegram auth max age must be positive")
	}
	return &InitDataValidator{botToken: botToken, maxAge: maxAge, now: time.Now}, nil
}

func (v *InitDataValidator) Validate(rawInitData string) (TelegramUser, error) {
	if rawInitData == "" || len(rawInitData) > maxInitDataLength {
		return TelegramUser{}, ErrInvalidInitData
	}

	values, err := url.ParseQuery(rawInitData)
	if err != nil {
		return TelegramUser{}, fmt.Errorf("%w: parse query", ErrInvalidInitData)
	}
	if err := requireSingleValues(values); err != nil {
		return TelegramUser{}, err
	}

	providedHash, err := hex.DecodeString(values.Get("hash"))
	if err != nil || len(providedHash) != sha256.Size {
		return TelegramUser{}, fmt.Errorf("%w: invalid hash", ErrInvalidInitData)
	}
	expectedHash := initDataHash(v.botToken, dataCheckString(values))
	if !hmac.Equal(providedHash, expectedHash) {
		return TelegramUser{}, fmt.Errorf("%w: hash mismatch", ErrInvalidInitData)
	}

	authTimestamp, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
	if err != nil {
		return TelegramUser{}, fmt.Errorf("%w: invalid auth_date", ErrInvalidInitData)
	}
	authTime := time.Unix(authTimestamp, 0)
	now := v.now()
	if authTime.After(now.Add(allowedClockSkew)) {
		return TelegramUser{}, fmt.Errorf("%w: auth_date is in the future", ErrInvalidInitData)
	}
	if now.Sub(authTime) > v.maxAge {
		return TelegramUser{}, ErrExpiredInitData
	}

	var user TelegramUser
	if err := json.Unmarshal([]byte(values.Get("user")), &user); err != nil {
		return TelegramUser{}, fmt.Errorf("%w: invalid user", ErrInvalidInitData)
	}
	if user.ID == 0 || user.IsBot {
		return TelegramUser{}, fmt.Errorf("%w: invalid user ID", ErrInvalidInitData)
	}
	return user, nil
}

func requireSingleValues(values url.Values) error {
	for key, entries := range values {
		if key == "" || len(entries) != 1 {
			return fmt.Errorf("%w: duplicate or empty field", ErrInvalidInitData)
		}
	}
	for _, key := range []string{"auth_date", "hash", "user"} {
		if values.Get(key) == "" {
			return fmt.Errorf("%w: missing %s", ErrInvalidInitData, key)
		}
	}
	return nil
}

func dataCheckString(values url.Values) string {
	keys := make([]string, 0, len(values)-1)
	for key := range values {
		if key != "hash" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	fields := make([]string, 0, len(keys))
	for _, key := range keys {
		fields = append(fields, key+"="+values.Get(key))
	}
	return strings.Join(fields, "\n")
}

func initDataHash(botToken, checkString string) []byte {
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	_, _ = secret.Write([]byte(botToken))

	hash := hmac.New(sha256.New, secret.Sum(nil))
	_, _ = hash.Write([]byte(checkString))
	return hash.Sum(nil)
}

type authContextKey struct{}

func TelegramUserFromContext(ctx context.Context) (TelegramUser, bool) {
	user, ok := ctx.Value(authContextKey{}).(TelegramUser)
	return user, ok
}

func AuthMiddleware(validator *InitDataValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			if validator == nil {
				writeError(response, http.StatusInternalServerError, "internal_error", "Сервис авторизации не настроен")
				return
			}

			scheme, rawInitData, found := strings.Cut(strings.TrimSpace(request.Header.Get("Authorization")), " ")
			if !found || !strings.EqualFold(scheme, authorizationScheme) || rawInitData == "" {
				writeError(response, http.StatusUnauthorized, "unauthorized", ErrUnauthorized.Error())
				return
			}
			user, err := validator.Validate(rawInitData)
			if err != nil {
				writeError(response, http.StatusUnauthorized, "unauthorized", "Некорректные или устаревшие данные Telegram")
				return
			}

			ctx := context.WithValue(request.Context(), authContextKey{}, user)
			next.ServeHTTP(response, request.WithContext(ctx))
		})
	}
}
