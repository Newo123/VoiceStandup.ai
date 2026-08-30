package delayed_publish

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestServiceScheduleCreatesRedisTimer(t *testing.T) {
	timers := newFakeTimerStore()
	service := newTestService(t, timers, &recordingCanceller{}, newRecordingPublisher())
	id := uuid.New()
	if err := service.Schedule(context.Background(), id); err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	if got := timers.delay(id); got != DefaultDelay {
		t.Errorf("TTL = %v, want %v", got, DefaultDelay)
	}
	if err := service.Schedule(context.Background(), id); !errors.Is(err, ErrAlreadyScheduled) {
		t.Errorf("second Schedule() error = %v, want ErrAlreadyScheduled", err)
	}
}

func TestServiceCancelRemovesTimerAndCancelsSubmission(t *testing.T) {
	timers := newFakeTimerStore()
	canceller := &recordingCanceller{}
	service := newTestService(t, timers, canceller, newRecordingPublisher())
	id := uuid.New()
	if err := service.Schedule(context.Background(), id); err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	if err := service.Cancel(context.Background(), id); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if !canceller.wasCancelled(id) {
		t.Error("DeleteSubmission() was not called")
	}
	if err := service.Cancel(context.Background(), id); !errors.Is(err, ErrNotScheduled) {
		t.Errorf("second Cancel() error = %v, want ErrNotScheduled", err)
	}
}

func TestServiceConfirmNowRemovesTimerAndPublishes(t *testing.T) {
	timers := newFakeTimerStore()
	publisher := newRecordingPublisher()
	service := newTestService(t, timers, &recordingCanceller{}, publisher)
	id := uuid.New()
	if err := service.Schedule(context.Background(), id); err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	if err := service.ConfirmNow(context.Background(), id); err != nil {
		t.Fatalf("ConfirmNow() error = %v", err)
	}
	if !publisher.wasPublished(id) {
		t.Error("Publish() was not called")
	}
}

func TestWorkerPublishesExpirationEvents(t *testing.T) {
	publisher := newRecordingPublisher()
	worker, err := NewWorker(publisher)
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan uuid.UUID, 1)
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx, fakeSubscriber{events: events}) }()
	id := uuid.New()
	events <- id
	eventually(t, func() bool { return publisher.wasPublished(id) })
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("Worker.Run() error = %v, want context.Canceled", err)
	}
}

func TestWorkerUsesBoundedPublishContext(t *testing.T) {
	publisher := &contextRecordingPublisher{contexts: make(chan context.Context, 1)}
	worker, err := NewWorker(publisher)
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan uuid.UUID, 1)
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx, fakeSubscriber{events: events}) }()
	events <- uuid.New()

	publishCtx := <-publisher.contexts
	deadline, ok := publishCtx.Deadline()
	if !ok || time.Until(deadline) > PublishTimeout {
		t.Errorf("publish context deadline = %v, want a deadline within %v", deadline, PublishTimeout)
	}
	cancel()
	<-done
}

func TestWorkerReportsUnexpectedSubscriptionClosure(t *testing.T) {
	worker, err := NewWorker(newRecordingPublisher())
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}
	events := make(chan uuid.UUID)
	close(events)

	err = worker.Run(context.Background(), fakeSubscriber{events: events})
	if !errors.Is(err, ErrSubscriptionClosed) {
		t.Errorf("Worker.Run() error = %v, want ErrSubscriptionClosed", err)
	}
}

func TestRedisStoreParsesOnlyDelayedPublishKeys(t *testing.T) {
	store := &RedisStore{prefix: "delayed_publish:"}
	id := uuid.New()

	got, ok := store.submissionIDFromKey("delayed_publish:" + id.String())
	if !ok || got != id {
		t.Errorf("submissionIDFromKey() = (%s, %t), want (%s, true)", got, ok, id)
	}
	if _, ok := store.submissionIDFromKey("other:" + id.String()); ok {
		t.Error("submissionIDFromKey() accepted an unrelated Redis key")
	}
}

func TestPublisherConfirmsBeforeGamification(t *testing.T) {
	var order []string
	publisher, err := NewPublisher(
		repositoryFunc(func(context.Context, uuid.UUID) error { order = append(order, "confirm"); return nil }),
		gamificationFunc(func(context.Context, uuid.UUID) error { order = append(order, "gamification"); return nil }),
	)
	if err != nil {
		t.Fatalf("NewPublisher() error = %v", err)
	}
	if err := publisher.Publish(context.Background(), uuid.New()); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if len(order) != 2 || order[0] != "confirm" || order[1] != "gamification" {
		t.Errorf("call order = %v, want [confirm gamification]", order)
	}
}

func newTestService(t *testing.T, timers TimerStore, canceller SubmissionCanceller, publisher SubmissionPublisher) *Service {
	t.Helper()
	service, err := NewService(timers, canceller, publisher)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}

type fakeTimerStore struct {
	mu     sync.Mutex
	timers map[uuid.UUID]time.Duration
}

func newFakeTimerStore() *fakeTimerStore {
	return &fakeTimerStore{timers: make(map[uuid.UUID]time.Duration)}
}
func (s *fakeTimerStore) Schedule(_ context.Context, id uuid.UUID, delay time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.timers[id]; ok {
		return false, nil
	}
	s.timers[id] = delay
	return true, nil
}
func (s *fakeTimerStore) Cancel(_ context.Context, id uuid.UUID) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.timers[id]; !ok {
		return false, nil
	}
	delete(s.timers, id)
	return true, nil
}
func (s *fakeTimerStore) delay(id uuid.UUID) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.timers[id]
}

type recordingPublisher struct {
	mu        sync.Mutex
	published []uuid.UUID
}

type contextRecordingPublisher struct {
	contexts chan context.Context
}

func (p *contextRecordingPublisher) Publish(ctx context.Context, _ uuid.UUID) error {
	p.contexts <- ctx
	return nil
}

func newRecordingPublisher() *recordingPublisher { return &recordingPublisher{} }
func (p *recordingPublisher) Publish(_ context.Context, id uuid.UUID) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.published = append(p.published, id)
	return nil
}
func (p *recordingPublisher) wasPublished(id uuid.UUID) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, value := range p.published {
		if value == id {
			return true
		}
	}
	return false
}

type recordingCanceller struct {
	mu        sync.Mutex
	cancelled []uuid.UUID
}

func (c *recordingCanceller) DeleteSubmission(_ context.Context, id uuid.UUID) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cancelled = append(c.cancelled, id)
	return nil
}
func (c *recordingCanceller) wasCancelled(id uuid.UUID) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, value := range c.cancelled {
		if value == id {
			return true
		}
	}
	return false
}

type fakeSubscriber struct{ events <-chan uuid.UUID }

func (s fakeSubscriber) SubscribeExpired(context.Context) (<-chan uuid.UUID, func() error, error) {
	return s.events, func() error { return nil }, nil
}

type repositoryFunc func(context.Context, uuid.UUID) error

func (fn repositoryFunc) ConfirmSubmission(ctx context.Context, id uuid.UUID) error {
	return fn(ctx, id)
}

type gamificationFunc func(context.Context, uuid.UUID) error

func (fn gamificationFunc) ApplyConfirmedSubmission(ctx context.Context, id uuid.UUID) error {
	return fn(ctx, id)
}
