package bot

import (
	"context"
	"strings"
	"testing"

	"VoiceStandup.ai/internal/core/domain"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

func TestRouteIgnoresStandupMessagesFromGroup(t *testing.T) {
	standupBot := &StandupTGBot{}
	update := tgbotapi.Update{Message: &tgbotapi.Message{
		From: &tgbotapi.User{ID: 1001},
		Chat: &tgbotapi.Chat{ID: -1001, Type: "group"},
		Text: "групповой текст",
	}}
	if err := standupBot.route(context.Background(), update); err != nil {
		t.Fatalf("route() error = %v", err)
	}
}

func TestFormatStandupPreviewEscapesUserContent(t *testing.T) {
	message := formatStandupPreview(&domain.StandupPreview{
		Done:     "Исправил <handler>",
		Plans:    "Буду править <handler>",
		Blockers: "API & токен",
	})
	if !strings.Contains(message, "Исправил &lt;handler&gt;") || !strings.Contains(message, "API &amp; токен") {
		t.Errorf("message does not escape HTML: %q", message)
	}
	if !strings.Contains(message, "<b>Что в планах:</b>\n—") {
		t.Errorf("message does not render empty section: %q", message)
	}
}

func TestParseStandupCallback(t *testing.T) {
	submissionID := uuid.New()
	data := callbackData(confirmCallbackAction, submissionID)
	action, parsedID, err := parseStandupCallback(data)
	if err != nil {
		t.Fatalf("parseStandupCallback() error = %v", err)
	}
	if action != confirmCallbackAction || parsedID != submissionID {
		t.Errorf("action/ID = %q/%s", action, parsedID)
	}
	if len(data) > 64 {
		t.Errorf("callback data length = %d, Telegram limit is 64", len(data))
	}
}

func TestParseStandupCallbackRejectsInvalidData(t *testing.T) {
	for _, data := range []string{"", "other:" + uuid.NewString(), confirmCallbackAction + ":bad-id"} {
		if _, _, err := parseStandupCallback(data); err == nil {
			t.Errorf("parseStandupCallback(%q) error = nil", data)
		}
	}
}
