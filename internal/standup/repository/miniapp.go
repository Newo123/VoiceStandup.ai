package repository

import (
	"context"
	"errors"
	"time"

	"VoiceStandup.ai/internal/core/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (r *Repository) GetTeamsByUserID(ctx context.Context, userID uuid.UUID) ([]domain.TeamMembership, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			t.id,
			t.name,
			t.telegram_chat_id,
			t.timezone,
			t.publish_local_time,
			t.workdays,
			t.late_policy,
			t.last_published_standup_date,
			t.created_at,
			t.deleted_at,
			tm.role,
			tm.is_owner
		FROM team_members AS tm
		JOIN teams AS t ON t.id = tm.team_id AND t.deleted_at IS NULL
		WHERE tm.user_id = $1
			AND tm.status = 'active'
			AND tm.deleted_at IS NULL
		ORDER BY t.created_at`, userID)
	if err != nil {
		return nil, wrapError("get teams by user", err)
	}
	defer rows.Close()

	memberships := make([]domain.TeamMembership, 0)
	for rows.Next() {
		membership, scanErr := scanTeamMembership(rows)
		if scanErr != nil {
			return nil, wrapError("scan team membership", scanErr)
		}
		memberships = append(memberships, *membership)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapError("iterate team memberships", err)
	}
	return memberships, nil
}

func (r *Repository) GetTeamMembership(
	ctx context.Context,
	userID uuid.UUID,
	teamID uuid.UUID,
) (*domain.TeamMembership, error) {
	membership, err := scanTeamMembership(r.db.QueryRow(ctx, `
		SELECT
			t.id,
			t.name,
			t.telegram_chat_id,
			t.timezone,
			t.publish_local_time,
			t.workdays,
			t.late_policy,
			t.last_published_standup_date,
			t.created_at,
			t.deleted_at,
			tm.role,
			tm.is_owner
		FROM team_members AS tm
		JOIN teams AS t ON t.id = tm.team_id AND t.deleted_at IS NULL
		WHERE tm.user_id = $1
			AND tm.team_id = $2
			AND tm.status = 'active'
			AND tm.deleted_at IS NULL`, userID, teamID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, wrapError("get team membership", err)
	}
	return membership, nil
}

func (r *Repository) GetTeamMemberStats(
	ctx context.Context,
	teamID uuid.UUID,
) ([]domain.TeamMemberStats, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			u.id,
			COALESCE(u.username, ''),
			COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), 'Участник'),
			tm.role,
			tm.is_owner,
			COALESCE(us.xp, 0),
			COALESCE(us.level, 1),
			COALESCE(us.current_streak, 0),
			COALESCE(us.best_streak, 0)
		FROM team_members AS tm
		JOIN users AS u ON u.id = tm.user_id AND u.deleted_at IS NULL
		LEFT JOIN user_stats AS us ON us.user_id = u.id
		WHERE tm.team_id = $1
			AND tm.status = 'active'
			AND tm.deleted_at IS NULL
		ORDER BY COALESCE(us.xp, 0) DESC, u.created_at`, teamID)
	if err != nil {
		return nil, wrapError("get team member stats", err)
	}
	defer rows.Close()

	members := make([]domain.TeamMemberStats, 0)
	for rows.Next() {
		var member domain.TeamMemberStats
		if err := rows.Scan(
			&member.UserID,
			&member.Username,
			&member.DisplayName,
			&member.Role,
			&member.IsOwner,
			&member.XP,
			&member.Level,
			&member.CurrentStreak,
			&member.BestStreak,
		); err != nil {
			return nil, wrapError("scan team member stats", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapError("iterate team member stats", err)
	}
	return members, nil
}

// CreateTeamForOwner атомарно создаёт команду, owner membership и выбирает
// новую команду активной для владельца.
func (r *Repository) CreateTeamForOwner(ctx context.Context, owner *domain.Users, team *domain.Teams) error {
	applyTeamDefaults(team)
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return wrapError("begin create team transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	created, err := scanTeam(tx.QueryRow(ctx, `
		INSERT INTO teams (
			name, telegram_chat_id, timezone, publish_local_time,
			workdays, late_policy, last_published_standup_date
		)
		SELECT $2, $3, $4, $5, $6, $7, $8
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING
			id,
			name,
			telegram_chat_id,
			timezone,
			publish_local_time,
			workdays,
			late_policy,
			last_published_standup_date,
			created_at,
			deleted_at`,
		owner.ID,
		team.Name,
		team.TelegramChatID,
		team.Timezone,
		team.PublishLocalTime.Format("15:04:05"),
		workdaysToSmallInts(team.Workdays),
		team.LatePolicy,
		nullableDate(team.LastPublishedStandupDate),
	))
	if err != nil {
		return wrapError("create owner team", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO team_members (
			team_id, user_id, role, is_owner, full_name, status
		)
		VALUES (
			$1, $2, '', true,
			COALESCE(NULLIF($3, ''), NULLIF($4, ''), 'Участник'),
			'active'
		)`, created.ID, owner.ID, owner.DisplayName, owner.Username); err != nil {
		return wrapError("create owner membership", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET active_team_id = $2
		WHERE id = $1 AND deleted_at IS NULL`, owner.ID, created.ID); err != nil {
		return wrapError("select created team", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return wrapError("commit create team transaction", err)
	}

	*team = *created
	owner.ActiveTeamID = &team.ID
	return nil
}

func scanTeamMembership(row rowScanner) (*domain.TeamMembership, error) {
	membership := &domain.TeamMembership{}
	var publishTime pgtype.Time
	var workdays []int16
	var lastPublished pgtype.Date

	err := row.Scan(
		&membership.Team.ID,
		&membership.Team.Name,
		&membership.Team.TelegramChatID,
		&membership.Team.Timezone,
		&publishTime,
		&workdays,
		&membership.Team.LatePolicy,
		&lastPublished,
		&membership.Team.CreatedAt,
		&membership.Team.DeletedAt,
		&membership.Role,
		&membership.IsOwner,
	)
	if err != nil {
		return nil, err
	}

	if publishTime.Valid {
		duration := time.Duration(publishTime.Microseconds) * time.Microsecond
		membership.Team.PublishLocalTime = time.Date(
			0,
			time.January,
			1,
			int(duration/time.Hour),
			int(duration%time.Hour/time.Minute),
			int(duration%time.Minute/time.Second),
			int(duration%time.Second),
			time.UTC,
		)
	}
	membership.Team.Workdays = smallIntsToWorkdays(workdays)
	if lastPublished.Valid {
		membership.Team.LastPublishedStandupDate = lastPublished.Time
	}
	return membership, nil
}
