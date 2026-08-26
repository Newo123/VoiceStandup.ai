// Package delayed_publish postpones standup publication to allow a user to
// cancel or confirm it immediately.
package delayed_publish

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// SubmissionRepository confirms a pending submission in persistent storage.
// Implementations must update only records with status "pending" and return an
// error when the submission is no longer pending.
type SubmissionRepository interface {
	ConfirmPending(ctx context.Context, submissionID uuid.UUID) error
}

// Gamification awards XP and updates streaks for a confirmed submission.
type Gamification interface {
	ApplyConfirmedSubmission(ctx context.Context, submissionID uuid.UUID) error
}

// SubmissionPublisher is used by Worker to publish a submission.
type SubmissionPublisher interface {
	Publish(ctx context.Context, submissionID uuid.UUID) error
}

// Publisher confirms a submission and then applies its gamification effects.
type Publisher struct {
	repository   SubmissionRepository
	gamification Gamification
}

func NewPublisher(repository SubmissionRepository, gamification Gamification) (*Publisher, error) {
	if repository == nil {
		return nil, fmt.Errorf("submission repository is required")
	}
	if gamification == nil {
		return nil, fmt.Errorf("gamification service is required")
	}

	return &Publisher{
		repository:   repository,
		gamification: gamification,
	}, nil
}

// Publish atomically moves a submission from pending to confirmed through the
// repository, then calculates its XP and streak effects.
func (p *Publisher) Publish(ctx context.Context, submissionID uuid.UUID) error {
	if err := p.repository.ConfirmPending(ctx, submissionID); err != nil {
		return fmt.Errorf("confirm pending submission: %w", err)
	}
	if err := p.gamification.ApplyConfirmedSubmission(ctx, submissionID); err != nil {
		return fmt.Errorf("apply gamification: %w", err)
	}

	return nil
}
