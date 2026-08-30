package repository

import (
	"context"
	"errors"
	"time"

	"VoiceStandup.ai/internal/core/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SaveSubmission создаёт стендап или заменяет текущий стендап для той же
// команды, пользователя и локальной даты.
func (r *Repository) SaveSubmission(ctx context.Context, submission *domain.Submissions) error {
	if submission.Status == "" {
		submission.Status = domain.SubmissionStatusProcessing
	}
	if submission.Format == "" {
		submission.Format = domain.SubmissionFormatText
	}

	row := r.db.QueryRow(ctx, `
		INSERT INTO submissions (
			team_id, user_id, standup_date, status, format,
			done_text, plans_text, blockers_text, confirmed_at
		)
		VALUES ($1, $2, $3::date, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (team_id, user_id, standup_date) DO UPDATE
		SET status = EXCLUDED.status,
			format = EXCLUDED.format,
			done_text = EXCLUDED.done_text,
			plans_text = EXCLUDED.plans_text,
			blockers_text = EXCLUDED.blockers_text,
			confirmed_at = EXCLUDED.confirmed_at,
			updated_at = now()
		RETURNING
			id,
			team_id,
			user_id,
			standup_date,
			status,
			format,
			done_text,
			plans_text,
			blockers_text,
			confirmed_at,
			created_at,
			updated_at`,
		submission.TeamID,
		submission.UserID,
		submission.StandupDate.Format(time.DateOnly),
		submission.Status,
		submission.Format,
		submission.DoneText,
		submission.PlansText,
		submission.BlockersText,
		submission.ConfirmedAt,
	)

	upserted, err := scanSubmission(row)
	if err != nil {
		return wrapError("save submission", err)
	}
	*submission = *upserted
	return nil
}

func (r *Repository) GetSubmissionByID(
	ctx context.Context,
	submissionID uuid.UUID,
) (*domain.Submissions, error) {
	submission, err := scanSubmission(r.db.QueryRow(ctx, `
		SELECT
			id,
			team_id,
			user_id,
			standup_date,
			status,
			format,
			done_text,
			plans_text,
			blockers_text,
			confirmed_at,
			created_at,
			updated_at
		FROM submissions
		WHERE id = $1`, submissionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, wrapError("get submission by id", err)
	}
	return submission, nil
}

func (r *Repository) GetSubmissionByTeamUserAndDate(
	ctx context.Context,
	teamID uuid.UUID,
	userID uuid.UUID,
	standupDate time.Time,
) (*domain.Submissions, error) {
	submission, err := scanSubmission(r.db.QueryRow(ctx, `
		SELECT
			id,
			team_id,
			user_id,
			standup_date,
			status,
			format,
			done_text,
			plans_text,
			blockers_text,
			confirmed_at,
			created_at,
			updated_at
		FROM submissions
		WHERE team_id = $1 AND user_id = $2 AND standup_date = $3::date`,
		teamID,
		userID,
		standupDate.Format(time.DateOnly),
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, wrapError("get submission by team, user and date", err)
	}
	return submission, nil
}

// GetSubmissionsByTeamAndDate возвращает только подтверждённые стендапы,
// поскольку метод используется сервисом публикации дайджеста.
func (r *Repository) GetSubmissionsByTeamAndDate(
	ctx context.Context,
	teamID uuid.UUID,
	standupDate time.Time,
) ([]domain.Submissions, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			id,
			team_id,
			user_id,
			standup_date,
			status,
			format,
			done_text,
			plans_text,
			blockers_text,
			confirmed_at,
			created_at,
			updated_at
		FROM submissions
		WHERE team_id = $1
			AND standup_date = $2::date
			AND status = $3
			AND confirmed_at IS NOT NULL
		ORDER BY created_at`, teamID, standupDate.Format(time.DateOnly), domain.SubmissionStatusConfirmed)
	if err != nil {
		return nil, wrapError("get submissions by team and date", err)
	}
	defer rows.Close()

	submissions := make([]domain.Submissions, 0)
	for rows.Next() {
		submission, scanErr := scanSubmission(rows)
		if scanErr != nil {
			return nil, wrapError("scan submission", scanErr)
		}
		submissions = append(submissions, *submission)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapError("iterate submissions", err)
	}
	return submissions, nil
}

func (r *Repository) ConfirmSubmission(ctx context.Context, submissionID uuid.UUID) error {
	commandTag, err := r.db.Exec(ctx, `
		UPDATE submissions
		SET status = $2, confirmed_at = now(), updated_at = now()
		WHERE id = $1 AND status = $3`,
		submissionID,
		domain.SubmissionStatusConfirmed,
		domain.SubmissionStatusAwaitingConfirmation,
	)
	if err != nil {
		return wrapError("confirm submission", err)
	}
	return ensureAffected("confirm submission", commandTag.RowsAffected())
}

func (r *Repository) GetSubmissionsByUserID(
	ctx context.Context,
	userID uuid.UUID,
) ([]domain.Submissions, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			id,
			team_id,
			user_id,
			standup_date,
			status,
			format,
			done_text,
			plans_text,
			blockers_text,
			confirmed_at,
			created_at,
			updated_at
		FROM submissions
		WHERE user_id = $1
		ORDER BY standup_date DESC, created_at DESC`, userID)
	if err != nil {
		return nil, wrapError("get submissions by user", err)
	}
	defer rows.Close()

	submissions := make([]domain.Submissions, 0)
	for rows.Next() {
		submission, scanErr := scanSubmission(rows)
		if scanErr != nil {
			return nil, wrapError("scan submission", scanErr)
		}
		submissions = append(submissions, *submission)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapError("iterate submissions", err)
	}
	return submissions, nil
}

func (r *Repository) DeleteSubmission(ctx context.Context, submissionID uuid.UUID) error {
	commandTag, err := r.db.Exec(ctx, `
		DELETE FROM submissions
		WHERE id = $1 AND status = $2`, submissionID, domain.SubmissionStatusAwaitingConfirmation)
	if err != nil {
		return wrapError("delete submission", err)
	}
	return ensureAffected("delete submission", commandTag.RowsAffected())
}

func scanSubmission(row rowScanner) (*domain.Submissions, error) {
	submission := &domain.Submissions{}
	err := row.Scan(
		&submission.ID,
		&submission.TeamID,
		&submission.UserID,
		&submission.StandupDate,
		&submission.Status,
		&submission.Format,
		&submission.DoneText,
		&submission.PlansText,
		&submission.BlockersText,
		&submission.ConfirmedAt,
		&submission.CreatedAt,
		&submission.UpdatedAt,
	)
	return submission, err
}
