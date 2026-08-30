package miniapp

import (
	"context"
	"errors"
	"testing"
	"time"

	"VoiceStandup.ai/internal/core/domain"
	"github.com/google/uuid"
)

func TestServiceGetProfile(t *testing.T) {
	repository, user := readyRepository()
	teamID := *user.ActiveTeamID
	repository.team = &domain.Teams{ID: teamID, Name: "Backend"}
	repository.stats = &domain.UserStats{UserID: user.ID, XP: 120, Level: 2}
	service := newTestService(t, repository)

	profile, err := service.GetProfile(context.Background(), user.TelegramUserID)
	if err != nil {
		t.Fatalf("GetProfile() error = %v", err)
	}
	if profile.User.ID != user.ID || profile.Stats.XP != 120 || profile.ActiveTeam.ID != teamID {
		t.Errorf("profile = %+v", profile)
	}
}

func TestServiceCreateTeam(t *testing.T) {
	repository, user := readyRepository()
	service := newTestService(t, repository)

	membership, err := service.CreateTeam(context.Background(), user.TelegramUserID, CreateTeamInput{
		Name:             "  Platform  ",
		TelegramChatID:   -1001,
		Timezone:         "Europe/Moscow",
		PublishLocalTime: time.Date(0, 1, 1, 10, 30, 0, 0, time.UTC),
		Workdays:         []int{1, 2, 3, 4, 5},
	})
	if err != nil {
		t.Fatalf("CreateTeam() error = %v", err)
	}
	if repository.created == nil || repository.created.Name != "Platform" {
		t.Fatalf("created team = %+v", repository.created)
	}
	if !membership.IsOwner || membership.Team.ID == uuid.Nil {
		t.Errorf("membership = %+v", membership)
	}
}

func TestServiceRejectsInvalidTeam(t *testing.T) {
	repository, user := readyRepository()
	service := newTestService(t, repository)
	tests := []CreateTeamInput{
		{Name: "x", TelegramChatID: -1, Timezone: "Europe/Moscow"},
		{Name: "Team", TelegramChatID: 0, Timezone: "Europe/Moscow"},
		{Name: "Team", TelegramChatID: -1, Timezone: "Mars/Olympus"},
		{Name: "Team", TelegramChatID: -1, Timezone: "Europe/Moscow", Workdays: []int{1, 1}},
	}
	for _, input := range tests {
		if _, err := service.CreateTeam(context.Background(), user.TelegramUserID, input); !errors.Is(err, ErrInvalidTeam) {
			t.Errorf("CreateTeam(%+v) error = %v", input, err)
		}
	}
}

func TestServiceChecksMembershipBeforeTeamAccess(t *testing.T) {
	repository, user := readyRepository()
	service := newTestService(t, repository)
	teamID := uuid.New()

	if _, err := service.SelectActiveTeam(context.Background(), user.TelegramUserID, teamID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("SelectActiveTeam() error = %v, want %v", err, ErrForbidden)
	}
	if _, err := service.GetTeam(context.Background(), user.TelegramUserID, teamID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("GetTeam() error = %v, want %v", err, ErrForbidden)
	}
	if _, err := service.GetTeamMembers(context.Background(), user.TelegramUserID, teamID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("GetTeamMembers() error = %v, want %v", err, ErrForbidden)
	}
	if repository.activeTeam != uuid.Nil || repository.memberStatsCalls != 0 {
		t.Error("protected repository operation was called")
	}
}

func TestServiceSelectsMemberTeam(t *testing.T) {
	repository, user := readyRepository()
	teamID := uuid.New()
	repository.membership = &domain.TeamMembership{Team: domain.Teams{ID: teamID}}
	service := newTestService(t, repository)

	if _, err := service.SelectActiveTeam(context.Background(), user.TelegramUserID, teamID); err != nil {
		t.Fatalf("SelectActiveTeam() error = %v", err)
	}
	if repository.activeTeam != teamID {
		t.Errorf("active team = %s, want %s", repository.activeTeam, teamID)
	}
}

func TestServiceGetTeam(t *testing.T) {
	repository, user := readyRepository()
	teamID := uuid.New()
	repository.membership = &domain.TeamMembership{Team: domain.Teams{ID: teamID, Name: "Backend"}}
	service := newTestService(t, repository)

	membership, err := service.GetTeam(context.Background(), user.TelegramUserID, teamID)
	if err != nil {
		t.Fatalf("GetTeam() error = %v", err)
	}
	if membership.Team.ID != teamID {
		t.Errorf("team ID = %s, want %s", membership.Team.ID, teamID)
	}
}

func newTestService(t *testing.T, repository Repository) *Service {
	t.Helper()
	service, err := NewService(repository)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func readyRepository() (*fakeRepository, *domain.Users) {
	teamID := uuid.New()
	user := &domain.Users{ID: uuid.New(), TelegramUserID: 1001, ActiveTeamID: &teamID}
	return &fakeRepository{user: user}, user
}

type fakeRepository struct {
	user             *domain.Users
	stats            *domain.UserStats
	team             *domain.Teams
	memberships      []domain.TeamMembership
	membership       *domain.TeamMembership
	members          []domain.TeamMemberStats
	created          *domain.Teams
	activeTeam       uuid.UUID
	memberStatsCalls int
}

func (r *fakeRepository) GetActiveUserByTelegramID(context.Context, int64) (*domain.Users, error) {
	return r.user, nil
}
func (r *fakeRepository) GetUserStats(context.Context, uuid.UUID) (*domain.UserStats, error) {
	return r.stats, nil
}
func (r *fakeRepository) GetTeamByUUID(context.Context, uuid.UUID) (*domain.Teams, error) {
	return r.team, nil
}
func (r *fakeRepository) GetTeamByTelegramChatID(context.Context, int64) (*domain.Teams, error) {
	return nil, nil
}
func (r *fakeRepository) GetTeamsByUserID(context.Context, uuid.UUID) ([]domain.TeamMembership, error) {
	return r.memberships, nil
}
func (r *fakeRepository) GetTeamMembership(context.Context, uuid.UUID, uuid.UUID) (*domain.TeamMembership, error) {
	return r.membership, nil
}
func (r *fakeRepository) GetTeamMemberStats(context.Context, uuid.UUID) ([]domain.TeamMemberStats, error) {
	r.memberStatsCalls++
	return r.members, nil
}
func (r *fakeRepository) CreateTeamForOwner(_ context.Context, owner *domain.Users, team *domain.Teams) error {
	team.ID = uuid.New()
	owner.ActiveTeamID = &team.ID
	copy := *team
	r.created = &copy
	return nil
}
func (r *fakeRepository) SetActiveTeam(_ context.Context, user *domain.Users, teamID uuid.UUID) error {
	user.ActiveTeamID = &teamID
	r.activeTeam = teamID
	return nil
}
func (r *fakeRepository) UpdateTeam(context.Context, *domain.Teams) error {
	return nil
}
func (r *fakeRepository) GetSubmissionsByUserID(context.Context, uuid.UUID) ([]domain.Submissions, error) {
	return nil, nil
}
func (r *fakeRepository) GetSubmissionByID(context.Context, uuid.UUID) (*domain.Submissions, error) {
	return nil, nil
}
func (r *fakeRepository) GetAllUsers(context.Context) ([]domain.Users, error) {
	return nil, nil
}
func (r *fakeRepository) GetUserByID(context.Context, uuid.UUID) (*domain.Users, error) {
	return nil, nil
}
