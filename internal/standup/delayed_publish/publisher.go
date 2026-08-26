// Package delayed_publish postpones standup publication to allow a user to
// cancel or confirm it immediately.
package delayed_publish

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// PendingSubmissionConfirmer confirms a pending submission in persistent
// storage. Implementations must update only records with status "pending".
type PendingSubmissionConfirmer interface {
	ConfirmPending(ctx context.Context, submissionID uuid.UUID) error
}

// SubmissionCanceller marks a pending submission as cancelled in persistent
// storage. Implementations must update only records with status "pending".
type SubmissionCanceller interface {
	CancelPending(ctx context.Context, submissionID uuid.UUID) error
}

// SubmissionRepository is the Postgres-facing contract used by delayed
// publication. CancelPending should set status to "cancelled".
type SubmissionRepository interface {
	PendingSubmissionConfirmer
	SubmissionCanceller
}

// Gamification awards XP and updates streaks for a confirmed submission.
type Gamification interface {
	ApplyConfirmedSubmission(ctx context.Context, submissionID uuid.UUID) error
}

// SubmissionPublisher is used by Worker and Service to publish a submission.
type SubmissionPublisher interface {
	Publish(ctx context.Context, submissionID uuid.UUID) error
}

// Publisher confirms a submission and then applies its gamification effects.
type Publisher struct {
	repository   PendingSubmissionConfirmer
	gamification Gamification
}

func NewPublisher(repository PendingSubmissionConfirmer, gamification Gamification) (*Publisher, error) {
	if repository == nil {
		return nil, fmt.Errorf("submission repository is required")
	}
	if gamification == nil {
		return nil, fmt.Errorf("gamification service is required")
	}

	return &Publisher{repository: repository, gamification: gamification}, nil
}

// Publish moves a submission from pending to confirmed, then calculates XP and
// streak effects.
func (p *Publisher) Publish(ctx context.Context, submissionID uuid.UUID) error {
	if err := p.repository.ConfirmPending(ctx, submissionID); err != nil {
		return fmt.Errorf("confirm pending submission: %w", err)
	}
	if err := p.gamification.ApplyConfirmedSubmission(ctx, submissionID); err != nil {
		return fmt.Errorf("apply gamification: %w", err)
	}
	return nil
}
