package repository

import (
	"context"
	"errors"
	"time"

	"VoiceStandup.ai/internal/core/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SaveUserStats создаёт статистику пользователя или сохраняет её текущее состояние.
func (r *Repository) SaveUserStats(ctx context.Context, stats *domain.UserStats) error {
	saved, err := scanUserStats(r.db.QueryRow(ctx, `
		INSERT INTO user_stats (
			user_id,
			xp,
			level,
			current_streak,
			best_streak,
			last_standup_date
		)
		VALUES ($1, $2, $3, $4, $5, $6::date)
		ON CONFLICT (user_id) DO UPDATE
		SET xp = EXCLUDED.xp,
			level = EXCLUDED.level,
			current_streak = EXCLUDED.current_streak,
			best_streak = EXCLUDED.best_streak,
			last_standup_date = EXCLUDED.last_standup_date,
			updated_at = now()
		RETURNING
			user_id,
			xp,
			level,
			current_streak,
			best_streak,
			last_standup_date,
			created_at,
			updated_at`,
		stats.UserID,
		stats.XP,
		stats.Level,
		stats.CurrentStreak,
		stats.BestStreak,
		nullableDatePointer(stats.LastStandupDate),
	))
	if err != nil {
		return wrapError("save user stats", err)
	}
	*stats = *saved
	return nil
}

func (r *Repository) GetUserStats(ctx context.Context, userID uuid.UUID) (*domain.UserStats, error) {
	stats, err := scanUserStats(r.db.QueryRow(ctx, `
		SELECT
			user_id,
			xp,
			level,
			current_streak,
			best_streak,
			last_standup_date,
			created_at,
			updated_at
		FROM user_stats
		WHERE user_id = $1`, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, wrapError("get user stats", err)
	}
	return stats, nil
}

func (r *Repository) DeleteUserStats(ctx context.Context, userID uuid.UUID) error {
	commandTag, err := r.db.Exec(ctx, `
		DELETE FROM user_stats
		WHERE user_id = $1`, userID)
	if err != nil {
		return wrapError("delete user stats", err)
	}
	return ensureAffected("delete user stats", commandTag.RowsAffected())
}

func scanUserStats(row rowScanner) (*domain.UserStats, error) {
	stats := &domain.UserStats{}
	err := row.Scan(
		&stats.UserID,
		&stats.XP,
		&stats.Level,
		&stats.CurrentStreak,
		&stats.BestStreak,
		&stats.LastStandupDate,
		&stats.CreatedAt,
		&stats.UpdatedAt,
	)
	return stats, err
}

func nullableDatePointer(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.Format(time.DateOnly)
}
