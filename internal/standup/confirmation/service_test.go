package confirmation

import (
	"context"
	"errors"
	"testing"

	"VoiceStandup.ai/internal/core/domain"
	"github.com/google/uuid"
)

func TestServiceConfirmNowForOwner(t *testing.T) {
	userID := uuid.New()
	submissionID := uuid.New()
	repository := &fakeRepository{
		user:       &domain.Users{ID: userID},
		submission: &domain.Submissions{ID: submissionID, UserID: userID},
	}
	lifecycle := &fakeLifecycle{}
	service := newTestService(t, repository, lifecycle)

	if err := service.ConfirmNow(context.Background(), 1001, submissionID); err != nil {
		t.Fatalf("ConfirmNow() error = %v", err)
	}
	if lifecycle.confirmed != submissionID {
		t.Errorf("confirmed ID = %s, want %s", lifecycle.confirmed, submissionID)
	}
}

func TestServiceRejectsAnotherUserSubmission(t *testing.T) {
	repository := &fakeRepository{
		user:       &domain.Users{ID: uuid.New()},
		submission: &domain.Submissions{ID: uuid.New(), UserID: uuid.New()},
	}
	lifecycle := &fakeLifecycle{}
	service := newTestService(t, repository, lifecycle)

	err := service.Cancel(context.Background(), 1001, repository.submission.ID)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("Cancel() error = %v, want %v", err, ErrForbidden)
	}
	if lifecycle.cancelled != uuid.Nil {
		t.Errorf("cancelled ID = %s, want nil", lifecycle.cancelled)
	}
}

func TestServiceRejectsMissingUserAndSubmission(t *testing.T) {
	tests := []struct {
		name       string
		repository *fakeRepository
		want       error
	}{
		{name: "user", repository: &fakeRepository{}, want: ErrUserNotFound},
		{name: "submission", repository: &fakeRepository{user: &domain.Users{ID: uuid.New()}}, want: ErrSubmissionNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := newTestService(t, test.repository, &fakeLifecycle{})
			err := service.ConfirmNow(context.Background(), 1001, uuid.New())
			if !errors.Is(err, test.want) {
				t.Fatalf("ConfirmNow() error = %v, want %v", err, test.want)
			}
		})
	}
}

func newTestService(t *testing.T, repository Repository, lifecycle Lifecycle) *Service {
	t.Helper()
	service, err := NewService(repository, lifecycle)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

type fakeRepository struct {
	user       *domain.Users
	submission *domain.Submissions
}

func (r *fakeRepository) GetActiveUserByTelegramID(context.Context, int64) (*domain.Users, error) {
	return r.user, nil
}
func (r *fakeRepository) GetSubmissionByID(context.Context, uuid.UUID) (*domain.Submissions, error) {
	return r.submission, nil
}

type fakeLifecycle struct {
	confirmed uuid.UUID
	cancelled uuid.UUID
}

func (l *fakeLifecycle) ConfirmNow(_ context.Context, submissionID uuid.UUID) error {
	l.confirmed = submissionID
	return nil
}
func (l *fakeLifecycle) Cancel(_ context.Context, submissionID uuid.UUID) error {
	l.cancelled = submissionID
	return nil
}
