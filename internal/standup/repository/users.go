package repository

import (
	"context"
	"errors"

	"VoiceStandup.ai/internal/core/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CreateUser(ctx context.Context, user *domain.Users) error {
	row := r.db.QueryRow(ctx, `
		INSERT INTO users (telegram_user_id, username, display_name, state)
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), $4)
		RETURNING
			id,
			state,
			telegram_user_id,
			COALESCE(username, ''),
			COALESCE(display_name, ''),
			created_at,
			deleted_at`,
		user.TelegramUserID,
		user.Username,
		user.DisplayName,
		user.State,
	)

	created, err := scanUser(row)
	if err != nil {
		return wrapError("create user", err)
	}
	*user = *created
	return nil
}

func (r *Repository) GetUserByID(ctx context.Context, userID uuid.UUID) (*domain.Users, error) {
	user, err := scanUser(r.db.QueryRow(ctx, `
		SELECT
			id,
			state,
			telegram_user_id,
			COALESCE(username, ''),
			COALESCE(display_name, ''),
			created_at,
			deleted_at
		FROM users
		WHERE id = $1`, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, wrapError("get user by id", err)
	}
	return user, nil
}

func (r *Repository) GetActiveUserByTelegramID(ctx context.Context, telegramUserID int64) (*domain.Users, error) {
	user, err := scanUser(r.db.QueryRow(ctx, `
		SELECT
			id,
			state,
			telegram_user_id,
			COALESCE(username, ''),
			COALESCE(display_name, ''),
			created_at,
			deleted_at
		FROM users
		WHERE telegram_user_id = $1 AND deleted_at IS NULL`, telegramUserID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, wrapError("get active user by telegram id", err)
	}
	return user, nil
}

func (r *Repository) UpdateUser(ctx context.Context, user *domain.Users) error {
	row := r.db.QueryRow(ctx, `
		UPDATE users
		SET username = NULLIF($2, ''), display_name = NULLIF($3, '')
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING
			id,
			state,
			telegram_user_id,
			COALESCE(username, ''),
			COALESCE(display_name, ''),
			created_at,
			deleted_at`,
		user.ID,
		user.Username,
		user.DisplayName,
	)

	updated, err := scanUser(row)
	if err != nil {
		return wrapError("update user", err)
	}
	*user = *updated
	return nil
}

func (r *Repository) SetUserState(ctx context.Context, user *domain.Users, state string) error {
	commandTag, err := r.db.Exec(ctx, `
		UPDATE users
		SET state = $2
		WHERE id = $1 AND deleted_at IS NULL`, user.ID, state)
	if err != nil {
		return wrapError("set user state", err)
	}
	if err := ensureAffected("set user state", commandTag.RowsAffected()); err != nil {
		return err
	}
	user.State = state
	return nil
}

func (r *Repository) SoftDeleteUser(ctx context.Context, userID uuid.UUID) error {
	commandTag, err := r.db.Exec(ctx, `
		UPDATE users
		SET deleted_at = now()
		WHERE id = $1 AND deleted_at IS NULL`, userID)
	if err != nil {
		return wrapError("soft delete user", err)
	}
	return ensureAffected("soft delete user", commandTag.RowsAffected())
}

func scanUser(row rowScanner) (*domain.Users, error) {
	user := &domain.Users{}
	err := row.Scan(
		&user.ID,
		&user.State,
		&user.TelegramUserID,
		&user.Username,
		&user.DisplayName,
		&user.CreatedAt,
		&user.DeletedAt,
	)
	return user, err
}
