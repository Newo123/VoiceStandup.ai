// 3.6. internal/standup/digest — Крон-сервис (12:00):
// сборка единого дайджеста команды, вычисление списка участников без отчёта,
// формирования поста в общий чат.

package digest

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"VoiceStandup.ai/internal/core/domain"
	"github.com/google/uuid"
)

type DigestService struct {
	repo     DigestRepo
	sender   DigestSender
	interval time.Duration
	done     chan struct{}
	logger   *slog.Logger
}

func NewDigestService(repo DigestRepo, sender DigestSender, interval time.Duration) *DigestService {
	return &DigestService{
		repo:     repo,
		sender:   sender,
		interval: interval,
		done:     make(chan struct{}),
		logger:   slog.Default().With("component", "digest_service"),
	}
}

// DigestRepo — интерфейс доступа к данным для формирования дайджеста.
type DigestRepo interface {

	// GetAllTeams возвращает все активные (не удалённые) команды.
	GetAllTeams(ctx context.Context) ([]domain.Teams, error)

	// GetSubmissionsByTeamAndDate возвращает отчёты команды за указанную дату.
	GetSubmissionsByTeamAndDate(ctx context.Context, teamID uuid.UUID, standupDate time.Time) ([]domain.Submissions, error)

	// GetTeamMembers возвращает всех активных участников команды.
	GetTeamMembers(ctx context.Context, teamID uuid.UUID) ([]domain.TeamMembers, error)

	// MarkDigestPublished проставляет last_published_standup_date команде на указанную дату.
	MarkDigestPublished(ctx context.Context, teamID uuid.UUID, date time.Time) error
}

// DigestSender — интерфейс отправки готового дайджеста в Telegram.
type DigestSender interface {
	SendMessage(ctx context.Context, chatID int64, html string) error
}

func (w *DigestService) Start(ctx context.Context) error {
	defer close(w.done)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Воркер получил сигнал остановки из контекста. Выходим из цикла...")
			return ctx.Err()
		case <-ticker.C:
			if err := w.process(ctx); err != nil {
				w.logger.Error("Ошибка", "error", err)
			}
		}
	}
}

func (w *DigestService) process(ctx context.Context) error {
	dCtx, dCancel := context.WithCancel(ctx)
	defer dCancel()

	// 1. Загружаем все активные команды
	teams, err := w.repo.GetAllTeams(dCtx)
	if err != nil {
		return fmt.Errorf("загрузка списка команд: %w", err)
	}

	for _, team := range teams {
		select {
		case <-dCtx.Done():
			w.logger.Info("Контекст воркера digest service завершен")
			return dCtx.Err()
		default:
		}

		if err := w.processTeam(dCtx, team); err != nil {
			w.logger.Error("Ошибка обработки команды",
				"team_id", team.ID,
				"team_name", team.Name,
				"error", err,
			)
		}
	}

	return nil
}

// processTeam проверяет, готова ли команда к публикации дайджеста.
// Если да — собирает, отправляет и помечает опубликованным.
func (w *DigestService) processTeam(ctx context.Context, team domain.Teams) error {
	loc, err := time.LoadLocation(team.Timezone)
	if err != nil {
		return fmt.Errorf("загрузка часового пояса %s: %w", team.Timezone, err)
	}

	now := time.Now().In(loc)
	today := now.Truncate(24 * time.Hour)

	// --- 1. Проверка рабочего дня ---
	// Преобразуем Go Weekday (0=Вс … 6=Сб) в ISO (1=Пн … 7=Вс).
	goWeekday := int(now.Weekday())
	if goWeekday == 0 {
		goWeekday = 7
	}

	isWorkday := false
	for _, wd := range team.Workdays {
		if wd == goWeekday {
			isWorkday = true
			break
		}
	}
	if !isWorkday {
		// w.logger.Debug("Сегодня не рабочий день, пропускаем",
		// 	"team_id", team.ID, "weekday", goWeekday)
		return nil
	}

	// --- 2. Проверка, опубликован ли уже дайджест сегодня ---
	lastPub := team.LastPublishedStandupDate
	if !lastPub.IsZero() {
		lastPubDate := lastPub.In(loc).Truncate(24 * time.Hour)
		if lastPubDate.Equal(today) {
			// w.logger.Debug("Дайджест уже опубликован сегодня",
			// 	"team_id", team.ID)
			return nil
		}
	}

	// --- 3. Проверка, наступило ли время публикации ---
	pubTime := team.PublishLocalTime
	publishDateTime := time.Date(
		now.Year(), now.Month(), now.Day(),
		pubTime.Hour(), pubTime.Minute(), pubTime.Second(), 0,
		loc,
	)

	if now.Before(publishDateTime) {
		// w.logger.Debug("Время публикации ещё не наступило",
		// 	"team_id", team.ID,
		// 	"publish_at", publishDateTime.Format("15:04"),
		// 	"now", now.Format("15:04"))
		return nil
	}

	// --- 4. Загружаем отчёты за сегодня ---
	submissions, err := w.repo.GetSubmissionsByTeamAndDate(ctx, team.ID, today)
	if err != nil {
		return fmt.Errorf("загрузка отчётов команды %s: %w", team.ID, err)
	}

	// --- 5. Загружаем всех участников команды ---
	members, err := w.repo.GetTeamMembers(ctx, team.ID)
	if err != nil {
		return fmt.Errorf("загрузка участников команды %s: %w", team.ID, err)
	}

	// --- 6. Собираем HTML дайджеста ---
	html := w.buildDigestMessage(team, submissions, members, today)

	// --- 7. Отправляем в Telegram ---
	if err := w.sender.SendMessage(ctx, team.TelegramChatID, html); err != nil {
		return fmt.Errorf("отправка дайджеста команде %s: %w", team.ID, err)
	}

	// --- 8. Помечаем как опубликованный ---
	if err := w.repo.MarkDigestPublished(ctx, team.ID, today); err != nil {
		return fmt.Errorf("отметка публикации дайджеста команде %s: %w", team.ID, err)
	}

	w.logger.Info("✅ Дайджест отправлен",
		"team_id", team.ID,
		"team_name", team.Name,
		"date", today.Format("2006-01-02"),
		"submissions", len(submissions),
		"members", len(members),
	)

	return nil
}
