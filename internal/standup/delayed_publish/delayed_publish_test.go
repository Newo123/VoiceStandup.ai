package delayed_publish

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestServicePublishesAfterDelay(t *testing.T) {
	publisher := newRecordingPublisher()
	service := newTestService(t, publisher, time.Millisecond)
	submissionID := uuid.New()

	if err := service.Schedule(context.Background(), submissionID); err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}

	select {
	case got := <-publisher.published:
		if got != submissionID {
			t.Errorf("published ID = %s, want %s", got, submissionID)
		}
	case <-time.After(time.Second):
		t.Fatal("scheduled submission was not published")
	}

	if err := service.Cancel(context.Background(), submissionID); !errors.Is(err, ErrNotScheduled) {
		t.Errorf("Cancel() error = %v, want ErrNotScheduled", err)
	}
}

func TestServiceCancelPreventsPublication(t *testing.T) {
	publisher := newRecordingPublisher()
	service := newTestService(t, publisher, 50*time.Millisecond)
	submissionID := uuid.New()

	if err := service.Schedule(context.Background(), submissionID); err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	if err := service.Cancel(context.Background(), submissionID); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if err := service.Cancel(context.Background(), submissionID); !errors.Is(err, ErrNotScheduled) {
		t.Errorf("second Cancel() error = %v, want ErrNotScheduled", err)
	}

	select {
	case got := <-publisher.published:
		t.Fatalf("unexpected publication of %s", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestServiceConfirmNowPublishesImmediately(t *testing.T) {
	publisher := newRecordingPublisher()
	service := newTestService(t, publisher, time.Hour)
	submissionID := uuid.New()

	if err := service.Schedule(context.Background(), submissionID); err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	if err := service.ConfirmNow(context.Background(), submissionID); err != nil {
		t.Fatalf("ConfirmNow() error = %v", err)
	}

	select {
	case got := <-publisher.published:
		if got != submissionID {
			t.Errorf("published ID = %s, want %s", got, submissionID)
		}
	default:
		t.Fatal("ConfirmNow() did not publish the submission")
	}
}

func TestServiceRejectsDuplicateSchedule(t *testing.T) {
	publisher := newRecordingPublisher()
	service := newTestService(t, publisher, time.Hour)
	submissionID := uuid.New()

	if err := service.Schedule(context.Background(), submissionID); err != nil {
		t.Fatalf("first Schedule() error = %v", err)
	}
	t.Cleanup(func() { _ = service.Cancel(context.Background(), submissionID) })

	if err := service.Schedule(context.Background(), submissionID); !errors.Is(err, ErrAlreadyScheduled) {
		t.Errorf("second Schedule() error = %v, want ErrAlreadyScheduled", err)
	}
}

func TestPublisherConfirmsBeforeGamification(t *testing.T) {
	var order []string
	publisher, err := NewPublisher(
		repositoryFunc(func(context.Context, uuid.UUID) error {
			order = append(order, "confirm")
			return nil
		}),
		gamificationFunc(func(context.Context, uuid.UUID) error {
			order = append(order, "gamification")
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("NewPublisher() error = %v", err)
	}

	if err := publisher.Publish(context.Background(), uuid.New()); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if got, want := len(order), 2; got != want || order[0] != "confirm" || order[1] != "gamification" {
		t.Errorf("call order = %v, want [confirm gamification]", order)
	}
}

func newTestService(t *testing.T, publisher SubmissionPublisher, delay time.Duration) *Service {
	t.Helper()
	worker, err := NewWorker(publisher)
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}
	service, err := newService(worker, delay)
	if err != nil {
		t.Fatalf("newService() error = %v", err)
	}
	return service
}

type recordingPublisher struct {
	published chan uuid.UUID
}

func newRecordingPublisher() *recordingPublisher {
	return &recordingPublisher{published: make(chan uuid.UUID, 1)}
}

func (p *recordingPublisher) Publish(_ context.Context, submissionID uuid.UUID) error {
	p.published <- submissionID
	return nil
}

type repositoryFunc func(context.Context, uuid.UUID) error

func (fn repositoryFunc) ConfirmPending(ctx context.Context, submissionID uuid.UUID) error {
	return fn(ctx, submissionID)
}

type gamificationFunc func(context.Context, uuid.UUID) error

func (fn gamificationFunc) ApplyConfirmedSubmission(ctx context.Context, submissionID uuid.UUID) error {
	return fn(ctx, submissionID)
}

var _ SubmissionPublisher = (*recordingPublisher)(nil)
var _ SubmissionRepository = repositoryFunc(nil)
var _ Gamification = gamificationFunc(nil)
