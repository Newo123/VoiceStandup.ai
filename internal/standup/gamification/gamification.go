package gamification

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"

	"VoiceStandup.ai/internal/core/domain"
)

// ---------- Константы ----------

const (
	// XP за время сдачи
	xpOnTime     = 30 // сдано до 10:00 local
	xpInWindow   = 15 // сдано 10:00 – 11:30 local
	xpBeforeLate = 10 // сдано 11:30 – 11:58 local

	// XP за формат
	xpVoice = 10 // голосовое сообщение
	xpText  = 0  // текстовый формат

	// Пороги множителя стрика
	streakThreshold10 = 10 // x2.0
	streakThreshold5  = 5  // x1.5
	streakThreshold3  = 3  // x1.2

	multiplier10 = 2.0
	multiplier5  = 1.5
	multiplier3  = 1.2
	multiplier1  = 1.0

	// Пороги уровней
	level2Threshold = 100
	level3Threshold = 300
	level4Threshold = 600
	level5Threshold = 1000
	level6Threshold = 1500

	level6PlusStep = 500
)

// ---------- Границы времени (local time) ----------

var (
	timeOnTimeEnd = parseTimeOnly("10:00")
	timeWindowEnd = parseTimeOnly("11:30")
	timeLateEnd   = parseTimeOnly("11:58")
)

func parseTimeOnly(s string) time.Time {
	t, err := time.Parse("15:04", s)
	if err != nil {
		panic("gamification: неверная константа времени: " + s)
	}
	return t
}

// ---------- Типы данных ----------

// UserStatsData — состояние геймификации одного пользователя.
type UserStatsData struct {
	XP              int64
	Level           int
	CurrentStreak   int
	BestStreak      int
	LastStandupDate *time.Time // только дата, nil если ни разу не сдавал
}

// ---------- Интерфейсы ----------

// GamificationRepo читает и обновляет данные геймификации в PostgreSQL.
type GamificationRepo interface {
	GetSubmission(ctx context.Context, submissionID uuid.UUID) (domain.Submissions, error)
	GetTeamTimezone(ctx context.Context, teamID uuid.UUID) (string, error)
	GetUserStats(ctx context.Context, userID uuid.UUID) (UserStatsData, error)
	UpdateUserStats(ctx context.Context, userID uuid.UUID, stats UserStatsData) error
}

// Clock — обёртка над time.Now() для тестирования.
type Clock interface {
	Now() time.Time
}

// RealClock возвращает реальное системное время.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

// ---------- Сервис ----------

type GamificationService struct {
	repo   GamificationRepo
	clock  Clock
	logger *slog.Logger
}

// NewGamificationService создаёт сервис геймификации.
// Если clock == nil, используется RealClock.
func NewGamificationService(repo GamificationRepo, clock Clock) *GamificationService {
	if clock == nil {
		clock = RealClock{}
	}
	return &GamificationService{
		repo:   repo,
		clock:  clock,
		logger: slog.Default().With("component", "gamification_service"),
	}
}

// ApplyConfirmedSubmission реализует контракт delayed_publish.Gamification.
// Загружает подтверждённую сдачу, рассчитывает XP, стрики и уровень, сохраняет результат.
func (g *GamificationService) ApplyConfirmedSubmission(ctx context.Context, submissionID uuid.UUID) error {
	// ---- Загрузка данных ----
	submission, err := g.repo.GetSubmission(ctx, submissionID)
	if err != nil {
		return fmt.Errorf("ошибка загрузки сдачи %s: %w", submissionID, err)
	}

	timezone, err := g.repo.GetTeamTimezone(ctx, submission.TeamID)
	if err != nil {
		return fmt.Errorf("ошибка загрузки часового пояса команды %s: %w", submission.TeamID, err)
	}

	stats, err := g.repo.GetUserStats(ctx, submission.UserID)
	if err != nil {
		return fmt.Errorf("ошибка загрузки статистики пользователя %s: %w", submission.UserID, err)
	}

	// ---- Шаг 1: Базовый XP (время + формат) ----
	baseXP := calculateTimeXP(submission.CreatedAt, timezone)
	formatXP := calculateFormatXP(submission.Format)
	rawXP := baseXP + formatXP

	// ---- Шаг 2: Система стриков ----
	today := todayInTimezone(g.clock.Now(), timezone)

	streak := stats.CurrentStreak
	lastDate := stats.LastStandupDate

	if lastDate != nil && isSameDay(*lastDate, today) {
		// Повторная сдача в тот же день — стрик не меняем.
	} else {
		if lastDate != nil && isYesterday(*lastDate, today) {
			streak++
		} else {
			streak = 1
		}
	}

	multiplier := streakMultiplier(streak)
	finalXP := int64(math.Round(float64(rawXP) * multiplier))

	// ---- Обновление рекорда ----
	bestStreak := stats.BestStreak
	if streak > bestStreak {
		bestStreak = streak
	}

	// ---- Шаг 3: Расчёт нового уровня ----
	newTotalXP := stats.XP + finalXP
	newLevel := calculateLevel(newTotalXP)

	// ---- Сохранение ----
	updatedStats := UserStatsData{
		XP:              newTotalXP,
		Level:           newLevel,
		CurrentStreak:   streak,
		BestStreak:      bestStreak,
		LastStandupDate: &today,
	}

	if err := g.repo.UpdateUserStats(ctx, submission.UserID, updatedStats); err != nil {
		return fmt.Errorf("ошибка обновления статистики пользователя %s: %w", submission.UserID, err)
	}

	g.logger.Debug("геймификация применена",
		"user_id", submission.UserID,
		"submission_id", submissionID,
		"base_xp", rawXP,
		"multiplier", multiplier,
		"final_xp", finalXP,
		"new_total_xp", newTotalXP,
		"new_level", newLevel,
		"streak", streak,
	)

	return nil
}

// ---------- Шаг 1: XP за время сдачи ----------

// calculateTimeXP возвращает XP в зависимости от времени сдачи в часовом поясе команды.
func calculateTimeXP(createdAt time.Time, timezone string) int {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	localTime := createdAt.In(loc)
	localClock := time.Date(0, 1, 1, localTime.Hour(), localTime.Minute(), localTime.Second(), 0, time.UTC)

	onTimeEnd := timeOnTimeEnd
	windowEnd := timeWindowEnd
	lateEnd := timeLateEnd

	switch {
	case localClock.Before(onTimeEnd) || localClock.Equal(onTimeEnd):
		return xpOnTime
	case localClock.Before(windowEnd) || localClock.Equal(windowEnd):
		return xpInWindow
	case localClock.Before(lateEnd) || localClock.Equal(lateEnd):
		return xpBeforeLate
	default:
		// После 11:58 — 0 XP.
		return 0
	}
}

// ---------- Шаг 1: XP за формат ----------

func calculateFormatXP(format string) int {
	if format == "voice" {
		return xpVoice
	}
	return xpText
}

// ---------- Шаг 2: Множитель стрика ----------

func streakMultiplier(streak int) float64 {
	switch {
	case streak >= streakThreshold10:
		return multiplier10
	case streak >= streakThreshold5:
		return multiplier5
	case streak >= streakThreshold3:
		return multiplier3
	default:
		return multiplier1
	}
}

// ---------- Шаг 3: Расчёт уровня ----------

func calculateLevel(totalXP int64) int {
	switch {
	case totalXP < level2Threshold:
		return 1
	case totalXP < level3Threshold:
		return 2
	case totalXP < level4Threshold:
		return 3
	case totalXP < level5Threshold:
		return 4
	case totalXP < level6Threshold:
		return 5
	default:
		// Уровень 6+: 5 + floor((xp - 1500) / 500) + 1
		return 5 + int((totalXP-level6Threshold)/level6PlusStep) + 1
	}
}

// ---------- Вспомогательные функции ----------

// todayInTimezone возвращает сегодняшнюю дату (без времени) в указанном часовом поясе.
func todayInTimezone(now time.Time, timezone string) time.Time {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	nowLocal := now.In(loc)
	return time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, time.UTC)
}

// isSameDay возвращает true, если обе даты приходятся на один календарный день.
func isSameDay(a, b time.Time) bool {
	ya, ma, da := a.Date()
	yb, mb, db := b.Date()
	return ya == yb && ma == mb && da == db
}

// isYesterday возвращает true, если a ровно на один календарный день раньше b.
func isYesterday(a, b time.Time) bool {
	ya, ma, da := a.Date()
	yb, mb, db := b.Date()

	daysA := daysSinceEpoch(ya, ma, da)
	daysB := daysSinceEpoch(yb, mb, db)
	return daysB-daysA == 1
}

// daysSinceEpoch возвращает количество дней с начала эпохи (год 1) для указанной даты.
func daysSinceEpoch(year int, month time.Month, day int) int {
	return int(time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Unix() / 86400)
}
