package delayed_publish

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

var ErrSubmissionNotPending = errors.New("submission is not pending")

const (
	confirmPendingQuery = `UPDATE submissions
SET status = 'confirmed', confirmed_at = now(), updated_at = now()
WHERE id = $1 AND status = 'pending'`
	cancelPendingQuery = `UPDATE submissions
SET status = 'cancelled', updated_at = now()
WHERE id = $1 AND status = 'pending'`
)

// SQLExecutor is satisfied by *sql.DB and *sql.Tx.
type SQLExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// PostgresRepository updates submission status with conditional SQL queries.
type PostgresRepository struct {
	db SQLExecutor
}

func NewPostgresRepository(db SQLExecutor) (*PostgresRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("SQL executor is required")
	}
	return &PostgresRepository{db: db}, nil
}

// ConfirmPending changes exactly one submission from pending to confirmed.
func (r *PostgresRepository) ConfirmPending(ctx context.Context, submissionID uuid.UUID) error {
	return r.updatePendingStatus(ctx, confirmPendingQuery, submissionID)
}

// CancelPending changes exactly one submission from pending to cancelled.
func (r *PostgresRepository) CancelPending(ctx context.Context, submissionID uuid.UUID) error {
	return r.updatePendingStatus(ctx, cancelPendingQuery, submissionID)
}

func (r *PostgresRepository) updatePendingStatus(ctx context.Context, query string, submissionID uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, query, submissionID)
	if err != nil {
		return fmt.Errorf("update submission status: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read updated row count: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("%w: %s", ErrSubmissionNotPending, submissionID)
	}
	return nil
}
