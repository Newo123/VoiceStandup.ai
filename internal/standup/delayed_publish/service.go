package delayed_publish

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

const DefaultDelay = 2 * time.Minute

var (
	ErrInvalidSubmissionID = errors.New("submission ID is required")
	ErrAlreadyScheduled    = errors.New("submission is already scheduled")
	ErrNotScheduled        = errors.New("submission is not scheduled")
)

// Worker receives timer-expiration events and publishes their submissions.
type Worker struct {
	publisher SubmissionPublisher
	logger    *slog.Logger
}

func NewWorker(publisher SubmissionPublisher) (*Worker, error) {
	if publisher == nil {
		return nil, fmt.Errorf("submission publisher is required")
	}

	return &Worker{
		publisher: publisher,
		logger:    slog.Default().With("component", "delayed_publish_worker"),
	}, nil
}

// Process publishes one submission. It is called by Service in a background
// goroutine when the scheduled delay expires.
func (w *Worker) Process(ctx context.Context, submissionID uuid.UUID) error {
	return w.publisher.Publish(ctx, submissionID)
}

// Service manages delayed-publication timers. It is safe for concurrent use.
type Service struct {
	worker *Worker
	delay  time.Duration

	mu     sync.Mutex
	timers map[uuid.UUID]*time.Timer
}

// NewService creates a service whose schedules expire after two minutes.
func NewService(worker *Worker) (*Service, error) {
	return newService(worker, DefaultDelay)
}

func newService(worker *Worker, delay time.Duration) (*Service, error) {
	if worker == nil {
		return nil, fmt.Errorf("delayed publish worker is required")
	}
	if delay < 0 {
		return nil, fmt.Errorf("delay must not be negative")
	}

	return &Service{
		worker: worker,
		delay:  delay,
		timers: make(map[uuid.UUID]*time.Timer),
	}, nil
}

// Schedule starts a two-minute delay before publishing submissionID.
func (s *Service) Schedule(ctx context.Context, submissionID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if submissionID == uuid.Nil {
		return ErrInvalidSubmissionID
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.timers[submissionID]; exists {
		return fmt.Errorf("%w: %s", ErrAlreadyScheduled, submissionID)
	}

	timer := time.AfterFunc(s.delay, func() { s.expire(submissionID) })
	s.timers[submissionID] = timer
	return nil
}

// Cancel removes a scheduled publication. It returns ErrNotScheduled when the
// delay has already expired or the submission was cancelled before.
func (s *Service) Cancel(ctx context.Context, submissionID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if submissionID == uuid.Nil {
		return ErrInvalidSubmissionID
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopScheduled(submissionID)
}

// ConfirmNow cancels a scheduled delay and immediately publishes submissionID.
func (s *Service) ConfirmNow(ctx context.Context, submissionID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if submissionID == uuid.Nil {
		return ErrInvalidSubmissionID
	}

	s.mu.Lock()
	if err := s.stopScheduled(submissionID); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()

	return s.worker.Process(ctx, submissionID)
}

func (s *Service) stopScheduled(submissionID uuid.UUID) error {
	timer, exists := s.timers[submissionID]
	if !exists || !timer.Stop() {
		return fmt.Errorf("%w: %s", ErrNotScheduled, submissionID)
	}

	delete(s.timers, submissionID)
	return nil
}

func (s *Service) expire(submissionID uuid.UUID) {
	s.mu.Lock()
	if _, exists := s.timers[submissionID]; !exists {
		s.mu.Unlock()
		return
	}
	delete(s.timers, submissionID)
	s.mu.Unlock()

	if err := s.worker.Process(context.Background(), submissionID); err != nil {
		s.worker.logger.Error("could not publish delayed submission", "submission_id", submissionID, "error", err)
	}
}
