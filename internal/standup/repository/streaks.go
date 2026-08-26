package repository

import (
	"context"
	"errors"
	"time"

	"VoiceStandup.ai/internal/core/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SaveStreak создаёт стрик или сохраняет его текущее состояние.
func (r *Repository) SaveStreak(ctx context.Context, streak *domain.Streaks) error {
	row := r.db.QueryRow(ctx, `
		INSERT INTO streaks (
			team_id,
			user_id,
			current_count,
			best_count,
			last_standup_date
		)
		VALUES ($1, $2, $3, $4, $5::date)
		ON CONFLICT (team_id, user_id) DO UPDATE
		SET current_count = EXCLUDED.current_count,
			best_count = EXCLUDED.best_count,
			last_standup_date = EXCLUDED.last_standup_date,
			updated_at = now()
		RETURNING
			team_id,
			user_id,
			current_count,
			best_count,
			last_standup_date,
			created_at,
			updated_at`,
		streak.TeamID,
		streak.UserID,
		streak.CurrentCount,
		streak.BestCount,
		nullableDatePointer(streak.LastStandupDate),
	)

	saved, err := scanStreak(row)
	if err != nil {
		return wrapError("save streak", err)
	}
	*streak = *saved
	return nil
}

func (r *Repository) GetStreak(
	ctx context.Context,
	teamID uuid.UUID,
	userID uuid.UUID,
) (*domain.Streaks, error) {
	streak, err := scanStreak(r.db.QueryRow(ctx, `
		SELECT
			team_id,
			user_id,
			current_count,
			best_count,
			last_standup_date,
			created_at,
			updated_at
		FROM streaks
		WHERE team_id = $1 AND user_id = $2`, teamID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, wrapError("get streak", err)
	}
	return streak, nil
}

func (r *Repository) ListTeamStreaks(ctx context.Context, teamID uuid.UUID) ([]domain.Streaks, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			team_id,
			user_id,
			current_count,
			best_count,
			last_standup_date,
			created_at,
			updated_at
		FROM streaks
		WHERE team_id = $1
		ORDER BY current_count DESC, best_count DESC, created_at`, teamID)
	if err != nil {
		return nil, wrapError("list team streaks", err)
	}
	defer rows.Close()

	streaks := make([]domain.Streaks, 0)
	for rows.Next() {
		streak, scanErr := scanStreak(rows)
		if scanErr != nil {
			return nil, wrapError("scan streak", scanErr)
		}
		streaks = append(streaks, *streak)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapError("iterate streaks", err)
	}
	return streaks, nil
}

func (r *Repository) DeleteStreak(
	ctx context.Context,
	teamID uuid.UUID,
	userID uuid.UUID,
) error {
	commandTag, err := r.db.Exec(ctx, `
		DELETE FROM streaks
		WHERE team_id = $1 AND user_id = $2`, teamID, userID)
	if err != nil {
		return wrapError("delete streak", err)
	}
	return ensureAffected("delete streak", commandTag.RowsAffected())
}

func scanStreak(row rowScanner) (*domain.Streaks, error) {
	streak := &domain.Streaks{}
	err := row.Scan(
		&streak.TeamID,
		&streak.UserID,
		&streak.CurrentCount,
		&streak.BestCount,
		&streak.LastStandupDate,
		&streak.CreatedAt,
		&streak.UpdatedAt,
	)
	return streak, err
}

func nullableDatePointer(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.Format(time.DateOnly)
}
