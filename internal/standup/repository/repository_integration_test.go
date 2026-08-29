package repository

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"VoiceStandup.ai/internal/core/domain"
	corepostgres "VoiceStandup.ai/internal/core/repository/postgres"
	"github.com/joho/godotenv"
)

func TestRepositoryCRUDIntegration(t *testing.T) {
	databaseURL := integrationDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := corepostgres.New(ctx, databaseURL, 5*time.Second)
	if err != nil {
		t.Fatalf("connect to postgres: %v", err)
	}
	defer pool.Close()

	repo := New(pool)
	uniqueID := time.Now().UnixNano()
	ownerTeamChatID := -uniqueID - 1
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()

		queries := []struct {
			name string
			sql  string
			args []any
		}{
			{"user stats", `DELETE FROM user_stats WHERE user_id IN (SELECT id FROM users WHERE telegram_user_id = $1)`, []any{uniqueID}},
			{"submissions", `DELETE FROM submissions WHERE user_id IN (SELECT id FROM users WHERE telegram_user_id = $1)`, []any{uniqueID}},
			{"team members", `DELETE FROM team_members WHERE user_id IN (SELECT id FROM users WHERE telegram_user_id = $1)`, []any{uniqueID}},
			{"teams", `DELETE FROM teams WHERE telegram_chat_id IN ($1, $2)`, []any{-uniqueID, ownerTeamChatID}},
			{"user", `DELETE FROM users WHERE telegram_user_id = $1`, []any{uniqueID}},
		}
		for _, query := range queries {
			if _, err := pool.Exec(cleanupCtx, query.sql, query.args...); err != nil {
				t.Errorf("cleanup %s: %v", query.name, err)
			}
		}
	}()

	user := &domain.Users{
		TelegramUserID: uniqueID,
		Username:       "repository_test",
		DisplayName:    "Repository Test",
	}
	if err := repo.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	stats, err := repo.GetUserStats(ctx, user.ID)
	if err != nil || stats == nil {
		t.Fatalf("GetUserStats() after CreateUser() stats = %v, error = %v", stats, err)
	}
	if stats.XP != 0 || stats.Level != 1 || stats.CurrentStreak != 0 || stats.BestStreak != 0 {
		t.Fatalf("default UserStats = %+v", stats)
	}

	duplicate := &domain.Users{TelegramUserID: user.TelegramUserID}
	if err := repo.CreateUser(ctx, duplicate); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate CreateUser() error = %v", err)
	}

	loadedUser, err := repo.GetActiveUserByTelegramID(ctx, user.TelegramUserID)
	if err != nil || loadedUser == nil {
		t.Fatalf("GetActiveUserByTelegramID() user = %v, error = %v", loadedUser, err)
	}
	if err := repo.SetUserState(ctx, user, domain.StateOnboarded); err != nil {
		t.Fatalf("SetUserState() error = %v", err)
	}
	user.DisplayName = "Updated Test User"
	if err := repo.UpdateUser(ctx, user); err != nil {
		t.Fatalf("UpdateUser() error = %v", err)
	}

	team := &domain.Teams{
		Name:             "Repository Test Team",
		TelegramChatID:   -uniqueID,
		Timezone:         "Europe/Moscow",
		PublishLocalTime: time.Date(0, time.January, 1, 12, 30, 0, 0, time.UTC),
		Workdays:         []int{1, 2, 3, 4, 5},
		LatePolicy:       "NEXT_DIGEST",
	}
	if err := repo.CreateTeam(ctx, team); err != nil {
		t.Fatalf("CreateTeam() error = %v", err)
	}

	loadedTeam, err := repo.GetTeamByTelegramChatID(ctx, team.TelegramChatID)
	if err != nil || loadedTeam == nil {
		t.Fatalf("GetTeamByTelegramChatID() team = %v, error = %v", loadedTeam, err)
	}
	if loadedTeam.PublishLocalTime.Hour() != 12 || loadedTeam.PublishLocalTime.Minute() != 30 {
		t.Fatalf("PublishLocalTime = %v", loadedTeam.PublishLocalTime)
	}
	if len(loadedTeam.Workdays) != 5 {
		t.Fatalf("Workdays = %v", loadedTeam.Workdays)
	}

	if err := repo.SaveUserInTeamByChatID(ctx, user.ID, team.ID); err != nil {
		t.Fatalf("SaveUserInTeamByChatID() error = %v", err)
	}
	if err := repo.SaveUserInTeamByChatID(ctx, user.ID, team.ID); err != nil {
		t.Fatalf("idempotent SaveUserInTeamByChatID() error = %v", err)
	}
	if err := repo.SaveUserRoleInTeam(ctx, user.ID, team.ID, "developer"); err != nil {
		t.Fatalf("SaveUserRoleInTeam() error = %v", err)
	}
	if err := repo.SetActiveTeam(ctx, user, team.ID); err != nil {
		t.Fatalf("SetActiveTeam() error = %v", err)
	}
	loadedUser, err = repo.GetActiveUserByTelegramID(ctx, user.TelegramUserID)
	if err != nil || loadedUser == nil || loadedUser.ActiveTeamID == nil || *loadedUser.ActiveTeamID != team.ID {
		t.Fatalf("GetActiveUserByTelegramID() active team user = %v, error = %v", loadedUser, err)
	}
	members, err := repo.GetTeamMembers(ctx, team.ID)
	if err != nil || len(members) != 1 || members[0].Role != "developer" {
		t.Fatalf("GetTeamMembers() members = %v, error = %v", members, err)
	}

	standupDate := time.Date(2026, time.August, 26, 0, 0, 0, 0, time.FixedZone("test", 3*60*60))
	submission := &domain.Submissions{
		TeamID:      team.ID,
		UserID:      user.ID,
		StandupDate: standupDate,
		Status:      domain.SubmissionStatusAwaitingConfirmation,
		Format:      domain.SubmissionFormatVoice,
		DoneText:    stringPointer("Implemented repository"),
		PlansText:   stringPointer("Add integration tests"),
	}
	if err := repo.SaveSubmission(ctx, submission); err != nil {
		t.Fatalf("SaveSubmission() error = %v", err)
	}
	if err := repo.ConfirmSubmission(ctx, submission.ID); err != nil {
		t.Fatalf("ConfirmSubmission() error = %v", err)
	}
	submissions, err := repo.GetSubmissionsByTeamAndDate(ctx, team.ID, standupDate)
	if err != nil || len(submissions) != 1 {
		t.Fatalf("GetSubmissionsByTeamAndDate() submissions = %v, error = %v", submissions, err)
	}
	if submissions[0].StandupDate.Format(time.DateOnly) != standupDate.Format(time.DateOnly) {
		t.Fatalf("StandupDate = %v", submissions[0].StandupDate)
	}
	if submissions[0].Format != domain.SubmissionFormatVoice {
		t.Fatalf("Format = %q", submissions[0].Format)
	}

	stats.XP = 100
	stats.Level = 2
	stats.CurrentStreak = 2
	stats.BestStreak = 3
	stats.LastStandupDate = &standupDate
	if err := repo.SaveUserStats(ctx, stats); err != nil {
		t.Fatalf("SaveUserStats() update error = %v", err)
	}
	loadedStats, err := repo.GetUserStats(ctx, user.ID)
	if err != nil || loadedStats == nil || loadedStats.XP != 100 || loadedStats.Level != 2 || loadedStats.CurrentStreak != 2 {
		t.Fatalf("GetUserStats() stats = %v, error = %v", loadedStats, err)
	}

	ownerTeam := &domain.Teams{
		Name:             "Owner Team",
		TelegramChatID:   ownerTeamChatID,
		Timezone:         "Europe/Moscow",
		PublishLocalTime: time.Date(0, time.January, 1, 10, 0, 0, 0, time.UTC),
		Workdays:         []int{1, 2, 3, 4, 5},
		LatePolicy:       domain.LatePolicyNextDigest,
	}
	if err := repo.CreateTeamForOwner(ctx, user, ownerTeam); err != nil {
		t.Fatalf("CreateTeamForOwner() error = %v", err)
	}
	membership, err := repo.GetTeamMembership(ctx, user.ID, ownerTeam.ID)
	if err != nil || membership == nil || !membership.IsOwner {
		t.Fatalf("GetTeamMembership() membership = %v, error = %v", membership, err)
	}
	memberships, err := repo.GetTeamsByUserID(ctx, user.ID)
	if err != nil || len(memberships) != 2 {
		t.Fatalf("GetTeamsByUserID() memberships = %v, error = %v", memberships, err)
	}
	memberStats, err := repo.GetTeamMemberStats(ctx, ownerTeam.ID)
	if err != nil || len(memberStats) != 1 || memberStats[0].XP != 100 {
		t.Fatalf("GetTeamMemberStats() members = %v, error = %v", memberStats, err)
	}
	loadedUser, err = repo.GetActiveUserByTelegramID(ctx, user.TelegramUserID)
	if err != nil || loadedUser == nil || loadedUser.ActiveTeamID == nil || *loadedUser.ActiveTeamID != ownerTeam.ID {
		t.Fatalf("active team after CreateTeamForOwner() user = %v, error = %v", loadedUser, err)
	}

	invalidStats := *stats
	invalidStats.XP = -1
	if err := repo.SaveUserStats(ctx, &invalidStats); err == nil {
		t.Fatal("SaveUserStats() with negative XP error = nil")
	}
	if err := repo.DeleteUserStats(ctx, user.ID); err != nil {
		t.Fatalf("DeleteUserStats() error = %v", err)
	}
	if deletedStats, err := repo.GetUserStats(ctx, user.ID); err != nil || deletedStats != nil {
		t.Fatalf("GetUserStats() after delete stats = %v, error = %v", deletedStats, err)
	}

	if err := repo.SoftDeleteTeam(ctx, team.ID); err != nil {
		t.Fatalf("SoftDeleteTeam() error = %v", err)
	}
	if activeTeam, err := repo.GetTeamByUUID(ctx, team.ID); err != nil || activeTeam != nil {
		t.Fatalf("GetTeamByUUID() after delete team = %v, error = %v", activeTeam, err)
	}
	if err := repo.SoftDeleteUser(ctx, user.ID); err != nil {
		t.Fatalf("SoftDeleteUser() error = %v", err)
	}
	if activeUser, err := repo.GetActiveUserByTelegramID(ctx, user.TelegramUserID); err != nil || activeUser != nil {
		t.Fatalf("GetActiveUserByTelegramID() after delete user = %v, error = %v", activeUser, err)
	}
}

func integrationDatabaseURL(t *testing.T) string {
	t.Helper()
	if os.Getenv("REPOSITORY_INTEGRATION") != "1" {
		t.Skip("set REPOSITORY_INTEGRATION=1 to run PostgreSQL integration test")
	}
	if databaseURL := os.Getenv("TEST_DATABASE_URL"); databaseURL != "" {
		return databaseURL
	}

	values, err := godotenv.Read("../../../.env")
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if values["DATABASE_URL"] == "" {
		t.Fatal("TEST_DATABASE_URL or DATABASE_URL in .env is required")
	}
	return values["DATABASE_URL"]
}

func stringPointer(value string) *string {
	return &value
}
