package httptransport

import (
	"time"

	"VoiceStandup.ai/internal/core/domain"
	"VoiceStandup.ai/internal/standup/miniapp"
)

type createTeamRequest struct {
	Name             string `json:"name"`
	TelegramChatID   int64  `json:"telegram_chat_id"`
	Timezone         string `json:"timezone"`
	PublishLocalTime string `json:"publish_local_time"`
	Workdays         []int  `json:"workdays"`
	LatePolicy       string `json:"late_policy"`
}

type selectActiveTeamRequest struct {
	TeamID string `json:"team_id"`
}

type updateTeamRequest struct {
	Name             *string `json:"name"`
	Timezone         *string `json:"timezone"`
	PublishLocalTime *string `json:"publish_local_time"`
	Workdays         *[]int  `json:"workdays"`
	LatePolicy       *string `json:"late_policy"`
}

type profileResponse struct {
	User       userResponse  `json:"user"`
	Stats      statsResponse `json:"stats"`
	ActiveTeam *teamResponse `json:"active_team"`
}

type userResponse struct {
	ID             string `json:"id"`
	TelegramUserID int64  `json:"telegram_user_id"`
	Username       string `json:"username"`
	DisplayName    string `json:"display_name"`
}

type statsResponse struct {
	XP              int     `json:"xp"`
	Level           int     `json:"level"`
	CurrentStreak   int     `json:"current_streak"`
	BestStreak      int     `json:"best_streak"`
	LastStandupDate *string `json:"last_standup_date"`
}

type teamResponse struct {
	ID                   string  `json:"id"`
	Name                 string  `json:"name"`
	TelegramChatID       int64   `json:"telegram_chat_id"`
	Timezone             string  `json:"timezone"`
	PublishLocalTime     string  `json:"publish_local_time"`
	Workdays             []int   `json:"workdays"`
	LatePolicy           string  `json:"late_policy"`
	Role                 string  `json:"role,omitempty"`
	IsOwner              bool    `json:"is_owner"`
	LastPublishedStandup *string `json:"last_published_standup_date"`
}

type teamMemberResponse struct {
	UserID        string `json:"user_id"`
	Username      string `json:"username"`
	DisplayName   string `json:"display_name"`
	Role          string `json:"role"`
	IsOwner       bool   `json:"is_owner"`
	XP            int    `json:"xp"`
	Level         int    `json:"level"`
	CurrentStreak int    `json:"current_streak"`
	BestStreak    int    `json:"best_streak"`
}

type reportResponse struct {
	ID           string  `json:"id"`
	TeamID       string  `json:"team_id"`
	StandupDate  string  `json:"standup_date"`
	Status       string  `json:"status"`
	Format       string  `json:"format"`
	DoneText     *string `json:"done_text"`
	PlansText    *string `json:"plans_text"`
	BlockersText *string `json:"blockers_text"`
	ConfirmedAt  *string `json:"confirmed_at"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    *string `json:"updated_at"`
}

func reportResponseFromDomain(submission domain.Submissions) reportResponse {
	var confirmedAt *string
	if submission.ConfirmedAt != nil {
		confirmedAt = datePointer(submission.ConfirmedAt)
	}
	var updatedAt *string
	if submission.UpdatedAt != nil {
		updatedAt = datePointer(submission.UpdatedAt)
	}
	return reportResponse{
		ID:           submission.ID.String(),
		TeamID:       submission.TeamID.String(),
		StandupDate:  submission.StandupDate.Format(time.DateOnly),
		Status:       submission.Status,
		Format:       submission.Format,
		DoneText:     submission.DoneText,
		PlansText:    submission.PlansText,
		BlockersText: submission.BlockersText,
		ConfirmedAt:  confirmedAt,
		CreatedAt:    submission.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    updatedAt,
	}
}

func userResponseFromDomain(user domain.Users) userResponse {
	return userResponse{
		ID:             user.ID.String(),
		TelegramUserID: user.TelegramUserID,
		Username:       user.Username,
		DisplayName:    user.DisplayName,
	}
}

func profileResponseFromDomain(profile *miniapp.Profile) profileResponse {
	response := profileResponse{
		User: userResponse{
			ID:             profile.User.ID.String(),
			TelegramUserID: profile.User.TelegramUserID,
			Username:       profile.User.Username,
			DisplayName:    profile.User.DisplayName,
		},
		Stats: statsResponse{
			XP:              profile.Stats.XP,
			Level:           profile.Stats.Level,
			CurrentStreak:   profile.Stats.CurrentStreak,
			BestStreak:      profile.Stats.BestStreak,
			LastStandupDate: datePointer(profile.Stats.LastStandupDate),
		},
	}
	if profile.ActiveTeam != nil {
		team := teamResponseFromDomain(domain.TeamMembership{Team: *profile.ActiveTeam})
		response.ActiveTeam = &team
	}
	return response
}

func teamResponseFromDomain(membership domain.TeamMembership) teamResponse {
	team := membership.Team
	return teamResponse{
		ID:                   team.ID.String(),
		Name:                 team.Name,
		TelegramChatID:       team.TelegramChatID,
		Timezone:             team.Timezone,
		PublishLocalTime:     team.PublishLocalTime.Format("15:04"),
		Workdays:             team.Workdays,
		LatePolicy:           team.LatePolicy,
		Role:                 membership.Role,
		IsOwner:              membership.IsOwner,
		LastPublishedStandup: datePointerValue(team.LastPublishedStandupDate),
	}
}

func teamMemberResponseFromDomain(member domain.TeamMemberStats) teamMemberResponse {
	return teamMemberResponse{
		UserID:        member.UserID.String(),
		Username:      member.Username,
		DisplayName:   member.DisplayName,
		Role:          member.Role,
		IsOwner:       member.IsOwner,
		XP:            member.XP,
		Level:         member.Level,
		CurrentStreak: member.CurrentStreak,
		BestStreak:    member.BestStreak,
	}
}

func datePointer(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	date := value.Format(time.DateOnly)
	return &date
}

func datePointerValue(value time.Time) *string {
	if value.IsZero() {
		return nil
	}
	date := value.Format(time.DateOnly)
	return &date
}
