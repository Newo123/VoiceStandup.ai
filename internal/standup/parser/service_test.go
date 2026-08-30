package parser

import (
	"context"
	"errors"
	"testing"
	"time"

	"VoiceStandup.ai/internal/core/domain"
	"github.com/google/uuid"
)

func TestServiceProcessText(t *testing.T) {
	repository, user, team := readyRepository()
	textProcessor := &fakeTextProcessor{response: &domain.StandupResponse{
		Done: "  Сделал parser  ", Plans: "Подключить бота", Blockers: "Нет",
	}}
	scheduler := &fakeScheduler{}
	service := newTestService(t, repository, textProcessor, &fakeVoiceProcessor{}, &fakeVoiceLoader{}, scheduler)
	service.now = func() time.Time {
		return time.Date(2026, time.August, 29, 21, 30, 0, 0, time.UTC)
	}

	preview, err := service.ProcessText(context.Background(), TextInput{
		TelegramUserID: user.TelegramUserID,
		Text:           "  вчера сделал parser  ",
	})
	if err != nil {
		t.Fatalf("ProcessText() error = %v", err)
	}
	if textProcessor.input != "вчера сделал parser" {
		t.Errorf("text processor input = %q", textProcessor.input)
	}
	if repository.saved == nil {
		t.Fatal("submission was not saved")
	}
	if repository.saved.TeamID != team.ID || repository.saved.UserID != user.ID {
		t.Errorf("saved submission owner = (%s, %s)", repository.saved.TeamID, repository.saved.UserID)
	}
	if repository.saved.Format != domain.SubmissionFormatText || repository.saved.Status != domain.SubmissionStatusAwaitingConfirmation {
		t.Errorf("saved format/status = %q/%q", repository.saved.Format, repository.saved.Status)
	}
	if got := repository.saved.StandupDate.Format(time.DateOnly); got != "2026-08-30" {
		t.Errorf("standup date = %q, want 2026-08-30", got)
	}
	if scheduler.scheduled != repository.saved.ID {
		t.Errorf("scheduled ID = %s, want %s", scheduler.scheduled, repository.saved.ID)
	}
	if preview.SubmissionID != repository.saved.ID || preview.Done != "Сделал parser" {
		t.Errorf("preview = %+v", preview)
	}
}

func TestServiceProcessVoice(t *testing.T) {
	repository, user, _ := readyRepository()
	voiceLoader := &fakeVoiceLoader{audio: []byte("ogg-data")}
	voiceProcessor := &fakeVoiceProcessor{response: &domain.StandupResponse{Plans: "Ревью"}}
	service := newTestService(t, repository, &fakeTextProcessor{}, voiceProcessor, voiceLoader, &fakeScheduler{})

	preview, err := service.ProcessVoice(context.Background(), VoiceInput{
		TelegramUserID: user.TelegramUserID,
		FileID:         " voice-file ",
		Duration:       42 * time.Second,
	})
	if err != nil {
		t.Fatalf("ProcessVoice() error = %v", err)
	}
	if voiceLoader.fileID != "voice-file" {
		t.Errorf("voice file ID = %q", voiceLoader.fileID)
	}
	if string(voiceProcessor.audio) != "ogg-data" {
		t.Errorf("voice processor audio = %q", voiceProcessor.audio)
	}
	if preview.Format != domain.SubmissionFormatVoice || repository.saved.Format != domain.SubmissionFormatVoice {
		t.Errorf("preview/saved format = %q/%q", preview.Format, repository.saved.Format)
	}
}

func TestServiceRejectsInvalidInputBeforeDependencies(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Service) error
		want error
	}{
		{
			name: "empty text",
			run: func(service *Service) error {
				_, err := service.ProcessText(context.Background(), TextInput{Text: "  "})
				return err
			},
			want: ErrEmptyText,
		},
		{
			name: "missing voice file",
			run: func(service *Service) error {
				_, err := service.ProcessVoice(context.Background(), VoiceInput{Duration: time.Second})
				return err
			},
			want: ErrVoiceFileRequired,
		},
		{
			name: "zero voice duration",
			run: func(service *Service) error {
				_, err := service.ProcessVoice(context.Background(), VoiceInput{FileID: "voice"})
				return err
			},
			want: ErrInvalidVoiceLength,
		},
		{
			name: "voice too long",
			run: func(service *Service) error {
				_, err := service.ProcessVoice(context.Background(), VoiceInput{FileID: "voice", Duration: DefaultMaxVoiceDuration + time.Second})
				return err
			},
			want: ErrVoiceTooLong,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeRepository{}
			service := newTestService(t, repository, &fakeTextProcessor{}, &fakeVoiceProcessor{}, &fakeVoiceLoader{}, &fakeScheduler{})
			if err := test.run(service); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if repository.userCalls != 0 {
				t.Errorf("repository calls = %d, want 0", repository.userCalls)
			}
		})
	}
}

func TestServiceRequiresUserAndActiveTeam(t *testing.T) {
	teamID := uuid.New()
	tests := []struct {
		name       string
		repository *fakeRepository
		want       error
	}{
		{name: "unknown user", repository: &fakeRepository{}, want: ErrUserNotFound},
		{name: "no active team", repository: &fakeRepository{user: &domain.Users{ID: uuid.New()}}, want: ErrActiveTeamRequired},
		{
			name: "missing active team",
			repository: &fakeRepository{user: &domain.Users{
				ID: uuid.New(), ActiveTeamID: &teamID,
			}},
			want: ErrActiveTeamNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := newTestService(t, test.repository, &fakeTextProcessor{}, &fakeVoiceProcessor{}, &fakeVoiceLoader{}, &fakeScheduler{})
			_, err := service.ProcessText(context.Background(), TextInput{TelegramUserID: 1, Text: "статус"})
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestServiceDoesNotSaveEmptyProcessorResponse(t *testing.T) {
	repository, user, _ := readyRepository()
	service := newTestService(
		t,
		repository,
		&fakeTextProcessor{response: &domain.StandupResponse{}},
		&fakeVoiceProcessor{},
		&fakeVoiceLoader{},
		&fakeScheduler{},
	)

	_, err := service.ProcessText(context.Background(), TextInput{TelegramUserID: user.TelegramUserID, Text: "статус"})
	if !errors.Is(err, ErrEmptyStandup) {
		t.Fatalf("error = %v, want %v", err, ErrEmptyStandup)
	}
	if repository.saved != nil {
		t.Error("empty standup was saved")
	}
}

func TestServiceRollsBackSubmissionWhenSchedulingFails(t *testing.T) {
	repository, user, _ := readyRepository()
	scheduleErr := errors.New("redis unavailable")
	service := newTestService(
		t,
		repository,
		&fakeTextProcessor{response: &domain.StandupResponse{Done: "Готово"}},
		&fakeVoiceProcessor{},
		&fakeVoiceLoader{},
		&fakeScheduler{err: scheduleErr},
	)

	_, err := service.ProcessText(context.Background(), TextInput{TelegramUserID: user.TelegramUserID, Text: "статус"})
	if !errors.Is(err, scheduleErr) {
		t.Fatalf("error = %v, want wrapped %v", err, scheduleErr)
	}
	if repository.saved == nil || repository.deleted != repository.saved.ID {
		t.Errorf("deleted ID = %s, saved = %+v", repository.deleted, repository.saved)
	}
}

func TestLocalStandupDateRejectsInvalidTimezone(t *testing.T) {
	_, err := localStandupDate(time.Now(), "Mars/Olympus")
	if err == nil {
		t.Fatal("localStandupDate() error = nil")
	}
}

func newTestService(
	t *testing.T,
	repository Repository,
	textProcessor TextProcessor,
	voiceProcessor VoiceProcessor,
	voiceLoader VoiceLoader,
	scheduler Scheduler,
) *Service {
	t.Helper()
	service, err := NewService(repository, textProcessor, voiceProcessor, voiceLoader, scheduler)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func readyRepository() (*fakeRepository, *domain.Users, *domain.Teams) {
	teamID := uuid.New()
	user := &domain.Users{ID: uuid.New(), ActiveTeamID: &teamID, TelegramUserID: 1001}
	team := &domain.Teams{ID: teamID, Timezone: "Europe/Moscow"}
	return &fakeRepository{user: user, team: team}, user, team
}

type fakeRepository struct {
	user      *domain.Users
	team      *domain.Teams
	saved     *domain.Submissions
	deleted   uuid.UUID
	userCalls int
}

func (r *fakeRepository) GetActiveUserByTelegramID(context.Context, int64) (*domain.Users, error) {
	r.userCalls++
	return r.user, nil
}
func (r *fakeRepository) GetTeamByUUID(context.Context, uuid.UUID) (*domain.Teams, error) {
	return r.team, nil
}
func (r *fakeRepository) SaveSubmission(_ context.Context, submission *domain.Submissions) error {
	if submission.ID == uuid.Nil {
		submission.ID = uuid.New()
	}
	copy := *submission
	r.saved = &copy
	return nil
}
func (r *fakeRepository) DeleteSubmission(_ context.Context, submissionID uuid.UUID) error {
	r.deleted = submissionID
	return nil
}

type fakeTextProcessor struct {
	response *domain.StandupResponse
	err      error
	input    string
}

func (p *fakeTextProcessor) ProcessText(_ context.Context, input string) (*domain.StandupResponse, error) {
	p.input = input
	return p.response, p.err
}

type fakeVoiceProcessor struct {
	response *domain.StandupResponse
	err      error
	audio    []byte
}

func (p *fakeVoiceProcessor) ProcessVoice(_ context.Context, audio []byte) (*domain.StandupResponse, error) {
	p.audio = audio
	return p.response, p.err
}

type fakeVoiceLoader struct {
	audio  []byte
	err    error
	fileID string
}

func (l *fakeVoiceLoader) DownloadVoice(_ context.Context, fileID string) ([]byte, error) {
	l.fileID = fileID
	return l.audio, l.err
}

type fakeScheduler struct {
	err       error
	scheduled uuid.UUID
}

func (s *fakeScheduler) Schedule(_ context.Context, submissionID uuid.UUID) error {
	s.scheduled = submissionID
	return s.err
}
