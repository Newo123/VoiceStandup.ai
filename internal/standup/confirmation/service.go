// Package confirmation проверяет владельца стендапа перед подтверждением или отменой.
package confirmation

import (
	"context"
	"errors"
	"fmt"

	"VoiceStandup.ai/internal/core/domain"
	"github.com/google/uuid"
)

var (
	ErrUserNotFound       = errors.New("confirmation user not found")
	ErrSubmissionNotFound = errors.New("submission not found")
	ErrForbidden          = errors.New("submission belongs to another user")
)

type Repository interface {
	GetActiveUserByTelegramID(ctx context.Context, telegramUserID int64) (*domain.Users, error)
	GetSubmissionByID(ctx context.Context, submissionID uuid.UUID) (*domain.Submissions, error)
}

type Lifecycle interface {
	ConfirmNow(ctx context.Context, submissionID uuid.UUID) (*domain.UserStats, error)
	Cancel(ctx context.Context, submissionID uuid.UUID) error
}

type Service struct {
	repository Repository
	lifecycle  Lifecycle
}

func NewService(repository Repository, lifecycle Lifecycle) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("confirmation: repository is required")
	}
	if lifecycle == nil {
		return nil, fmt.Errorf("confirmation: lifecycle service is required")
	}
	return &Service{repository: repository, lifecycle: lifecycle}, nil
}

func (s *Service) ConfirmNow(ctx context.Context, telegramUserID int64, submissionID uuid.UUID) (*domain.UserStats, error) {
	if err := s.authorize(ctx, telegramUserID, submissionID); err != nil {
		return nil, err
	}
	stats, err := s.lifecycle.ConfirmNow(ctx, submissionID)
	if err != nil {
		return nil, fmt.Errorf("confirmation: confirm submission: %w", err)
	}
	return stats, nil
}

func (s *Service) Cancel(ctx context.Context, telegramUserID int64, submissionID uuid.UUID) error {
	if err := s.authorize(ctx, telegramUserID, submissionID); err != nil {
		return err
	}
	if err := s.lifecycle.Cancel(ctx, submissionID); err != nil {
		return fmt.Errorf("confirmation: cancel submission: %w", err)
	}
	return nil
}

func (s *Service) authorize(ctx context.Context, telegramUserID int64, submissionID uuid.UUID) error {
	user, err := s.repository.GetActiveUserByTelegramID(ctx, telegramUserID)
	if err != nil {
		return fmt.Errorf("confirmation: get user: %w", err)
	}
	if user == nil {
		return ErrUserNotFound
	}

	submission, err := s.repository.GetSubmissionByID(ctx, submissionID)
	if err != nil {
		return fmt.Errorf("confirmation: get submission: %w", err)
	}
	if submission == nil {
		return ErrSubmissionNotFound
	}
	if submission.UserID != user.ID {
		return ErrForbidden
	}
	return nil
}
