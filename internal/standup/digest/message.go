package digest

import (
	"fmt"
	"strings"
	"time"

	"VoiceStandup.ai/internal/core/domain"
	"github.com/google/uuid"
)

// buildDigestMessage собирает HTML-сообщение дайджеста для отправки в Telegram.
func (w *DigestService) buildDigestMessage(
	team domain.Teams,
	submissions []domain.Submissions,
	members []domain.TeamMembers,
	date time.Time,
) string {
	var sb strings.Builder

	// --- Заголовок ---
	sb.WriteString("<b>📋 Ежедневный стендап</b>\n")
	sb.WriteString(fmt.Sprintf("<b>Команда:</b> %s\n", escapeHTML(team.Name)))
	sb.WriteString(fmt.Sprintf("<b>Дата:</b> %s\n\n", date.Format("02.01.2006")))

	// Индексы для быстрого поиска
	submittedUserIDs := make(map[uuid.UUID]struct{}, len(submissions))
	for _, s := range submissions {
		submittedUserIDs[s.UserID] = struct{}{}
	}

	memberByUserID := make(map[uuid.UUID]domain.TeamMembers, len(members))
	for _, m := range members {
		memberByUserID[m.UserID] = m
	}

	// --- Блок отчётов ---
	sb.WriteString("━━━━━━━━━━━━━━━━\n")
	if len(submissions) > 0 {
		sb.WriteString("<b>✅ Отчеты участников:</b>\n\n")

		for _, sub := range submissions {
			member, ok := memberByUserID[sub.UserID]
			name := "Участник"
			if ok && member.FullName != "" {
				name = member.FullName
			}

			sb.WriteString(fmt.Sprintf("<b>👤 %s</b>\n", escapeHTML(name)))

			if sub.DoneText != nil && *sub.DoneText != "" {
				sb.WriteString(fmt.Sprintf("  <b>✅ Сделано:</b> %s\n", escapeHTML(*sub.DoneText)))
			}
			if sub.PlansText != nil && *sub.PlansText != "" {
				sb.WriteString(fmt.Sprintf("  <b>📋 Планы:</b> %s\n", escapeHTML(*sub.PlansText)))
			}
			if sub.BlockersText != nil && *sub.BlockersText != "" {
				sb.WriteString(fmt.Sprintf("  <b>🚫 Блокеры:</b> %s\n", escapeHTML(*sub.BlockersText)))
			}
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("<b>❌ Отчетов сегодня нет</b>\n\n")
	}

	// --- Блок молчунов ---
	var silentMembers []domain.TeamMembers
	for _, m := range members {
		if _, ok := submittedUserIDs[m.UserID]; !ok {
			silentMembers = append(silentMembers, m)
		}
	}

	if len(silentMembers) > 0 {
		sb.WriteString("━━━━━━━━━━━━━━━━\n")
		sb.WriteString("<b>🔇 Молчуны (штраф -20 XP):</b>\n")
		for _, m := range silentMembers {
			name := m.FullName
			if name == "" {
				name = "Участник"
			}
			sb.WriteString(fmt.Sprintf("  • %s\n", escapeHTML(name)))
		}
		sb.WriteString("\n")
	}

	// --- Подвал ---
	sb.WriteString("━━━━━━━━━━━━━━━━\n")
	sb.WriteString("<i>Стендап-бот 🤖</i>")

	return sb.String()
}

// escapeHTML экранирует спецсимволы для HTML-режима Telegram.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&")
	s = strings.ReplaceAll(s, "<", "<")
	s = strings.ReplaceAll(s, ">", ">")
	return s
}
