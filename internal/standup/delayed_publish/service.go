package delayed_publish

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const DefaultDelay = 2 * time.Minute

var (
	ErrInvalidSubmissionID = errors.New("submission ID is required")
	ErrNotScheduled        = errors.New("submission is not scheduled")
)

// TimerStore persists delayed-publication timers and must survive restarts.
type TimerStore interface {
	Schedule(ctx context.Context, submissionID uuid.UUID, delay time.Duration) (created bool, err error)
	Cancel(ctx context.Context, submissionID uuid.UUID) (cancelled bool, err error)
}

// ExpirationSubscriber supplies IDs from expired Redis timer keys.
type ExpirationSubscriber interface {
	SubscribeExpired(ctx context.Context) (<-chan uuid.UUID, func() error, error)
}

// RedisStore stores one TTL key per delayed submission. Redis must enable
// keyspace notifications with "notify-keyspace-events Ex".
type RedisStore struct {
	client redis.UniversalClient
	db     int
	prefix string
}

func NewRedisStore(client redis.UniversalClient, db int) (*RedisStore, error) {
	if client == nil {
		return nil, fmt.Errorf("redis client is required")
	}
	if db < 0 {
		return nil, fmt.Errorf("redis database number must not be negative")
	}
	return &RedisStore{client: client, db: db, prefix: "delayed_publish:"}, nil
}

func (s *RedisStore) Schedule(ctx context.Context, submissionID uuid.UUID, delay time.Duration) (bool, error) {
	result, err := s.client.SetArgs(ctx, s.key(submissionID), "1", redis.SetArgs{Mode: "NX", TTL: delay}).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("set delayed publish key: %w", err)
	}
	return result == "OK", nil
}

func (s *RedisStore) Cancel(ctx context.Context, submissionID uuid.UUID) (bool, error) {
	deleted, err := s.client.Del(ctx, s.key(submissionID)).Result()
	if err != nil {
		return false, fmt.Errorf("delete delayed publish key: %w", err)
	}
	return deleted == 1, nil
}

// SubscribeExpired converts Redis key-expiration messages to submission IDs.
func (s *RedisStore) SubscribeExpired(ctx context.Context) (<-chan uuid.UUID, func() error, error) {
	pubsub := s.client.PSubscribe(ctx, fmt.Sprintf("__keyevent@%d__:expired", s.db))
	messages := pubsub.Channel()
	ids := make(chan uuid.UUID)

	go func() {
		defer close(ids)
		for {
			select {
			case <-ctx.Done():
				return
			case message, ok := <-messages:
				if !ok {
					return
				}
				submissionID, ok := s.submissionIDFromKey(message.Payload)
				if !ok {
					continue
				}
				select {
				case ids <- submissionID:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ids, pubsub.Close, nil
}

func (s *RedisStore) key(submissionID uuid.UUID) string { return s.prefix + submissionID.String() }

func (s *RedisStore) submissionIDFromKey(key string) (uuid.UUID, bool) {
	value, found := strings.CutPrefix(key, s.prefix)
	if !found {
		return uuid.Nil, false
	}
	submissionID, err := uuid.Parse(value)
	return submissionID, err == nil
}

// Worker receives Redis expiration events and publishes their submissions.
type Worker struct {
	publisher SubmissionPublisher
	logger    *slog.Logger
}

func NewWorker(publisher SubmissionPublisher) (*Worker, error) {
	if publisher == nil {
		return nil, fmt.Errorf("submission publisher is required")
	}
	return &Worker{publisher: publisher, logger: slog.Default().With("component", "delayed_publish_worker")}, nil
}

// Run listens until ctx is cancelled. Failed publications are logged; a
// conditional pending-to-confirmed repository update prevents duplicates.
func (w *Worker) Run(ctx context.Context, subscriber ExpirationSubscriber) error {
	if subscriber == nil {
		return fmt.Errorf("expiration subscriber is required")
	}
	events, closeSubscription, err := subscriber.SubscribeExpired(ctx)
	if err != nil {
		return fmt.Errorf("subscribe to expiration events: %w", err)
	}
	defer func() { _ = closeSubscription() }()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case submissionID, ok := <-events:
			if !ok {
				return nil
			}
			if err := w.publisher.Publish(context.Background(), submissionID); err != nil {
				w.logger.Error("could not publish delayed submission", "submission_id", submissionID, "error", err)
			}
		}
	}
}

// Service manages Redis-backed delayed-publication keys.
type Service struct {
	timers    TimerStore
	canceller SubmissionCanceller
	publisher SubmissionPublisher
	delay     time.Duration
}

// NewService creates a service whose Redis timer keys expire after two minutes.
func NewService(timers TimerStore, canceller SubmissionCanceller, publisher SubmissionPublisher) (*Service, error) {
	if timers == nil {
		return nil, fmt.Errorf("timer store is required")
	}
	if canceller == nil {
		return nil, fmt.Errorf("submission canceller is required")
	}
	if publisher == nil {
		return nil, fmt.Errorf("submission publisher is required")
	}
	return &Service{timers: timers, canceller: canceller, publisher: publisher, delay: DefaultDelay}, nil
}

// Schedule writes a Redis key with a two-minute TTL.
func (s *Service) Schedule(ctx context.Context, submissionID uuid.UUID) error {
	if err := validateRequest(ctx, submissionID); err != nil {
		return err
	}
	_, err := s.timers.Schedule(ctx, submissionID, s.delay)
	if err != nil {
		return err
	}
	// Повторное планирование того же submission идемпотентно: существующий
	// таймер продолжит отсчёт, а репозиторий уже сохранил свежую версию отчёта.
	return nil
}

// Cancel deletes the Redis key and changes the pending submission to cancelled.
func (s *Service) Cancel(ctx context.Context, submissionID uuid.UUID) error {
	if err := validateRequest(ctx, submissionID); err != nil {
		return err
	}
	cancelled, err := s.timers.Cancel(ctx, submissionID)
	if err != nil {
		return err
	}
	if !cancelled {
		return fmt.Errorf("%w: %s", ErrNotScheduled, submissionID)
	}
	if err := s.canceller.DeleteSubmission(ctx, submissionID); err != nil {
		return fmt.Errorf("mark submission cancelled: %w", err)
	}
	return nil
}

// ConfirmNow deletes the Redis key and immediately publishes submissionID.
func (s *Service) ConfirmNow(ctx context.Context, submissionID uuid.UUID) error {
	if err := validateRequest(ctx, submissionID); err != nil {
		return err
	}
	cancelled, err := s.timers.Cancel(ctx, submissionID)
	if err != nil {
		return err
	}
	if !cancelled {
		return fmt.Errorf("%w: %s", ErrNotScheduled, submissionID)
	}
	return s.publisher.Publish(ctx, submissionID)
}

func validateRequest(ctx context.Context, submissionID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if submissionID == uuid.Nil {
		return ErrInvalidSubmissionID
	}
	return nil
}
