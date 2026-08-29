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
