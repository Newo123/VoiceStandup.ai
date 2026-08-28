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

func (r *Repository) CreateTeam(ctx context.Context, team *domain.Teams) error {
	applyTeamDefaults(team)

	row := r.db.QueryRow(ctx, `
		INSERT INTO teams (
			name, telegram_chat_id, timezone, publish_local_time,
			workdays, late_policy, last_published_standup_date
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
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
		team.Name,
		team.TelegramChatID,
		team.Timezone,
		team.PublishLocalTime.Format("15:04:05"),
		workdaysToSmallInts(team.Workdays),
		team.LatePolicy,
		nullableDate(team.LastPublishedStandupDate),
	)

	created, err := scanTeam(row)
	if err != nil {
		return wrapError("create team", err)
	}
	*team = *created
	return nil
}

func (r *Repository) GetTeamByUUID(ctx context.Context, teamID uuid.UUID) (*domain.Teams, error) {
	team, err := scanTeam(r.db.QueryRow(ctx, `
		SELECT
			id,
			name,
			telegram_chat_id,
			timezone,
			publish_local_time,
			workdays,
			late_policy,
			last_published_standup_date,
			created_at,
			deleted_at
		FROM teams
		WHERE id = $1 AND deleted_at IS NULL`, teamID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, wrapError("get team by uuid", err)
	}
	return team, nil
}

func (r *Repository) GetTeamByTelegramChatID(ctx context.Context, chatID int64) (*domain.Teams, error) {
	team, err := scanTeam(r.db.QueryRow(ctx, `
		SELECT
			id,
			name,
			telegram_chat_id,
			timezone,
			publish_local_time,
			workdays,
			late_policy,
			last_published_standup_date,
			created_at,
			deleted_at
		FROM teams
		WHERE telegram_chat_id = $1 AND deleted_at IS NULL`, chatID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, wrapError("get team by telegram chat id", err)
	}
	return team, nil
}

func (r *Repository) GetAllTeams(ctx context.Context) ([]domain.Teams, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			id,
			name,
			telegram_chat_id,
			timezone,
			publish_local_time,
			workdays,
			late_policy,
			last_published_standup_date,
			created_at,
			deleted_at
		FROM teams
		WHERE deleted_at IS NULL
		ORDER BY created_at`)
	if err != nil {
		return nil, wrapError("get all teams", err)
	}
	defer rows.Close()

	teams := make([]domain.Teams, 0)
	for rows.Next() {
		team, scanErr := scanTeam(rows)
		if scanErr != nil {
			return nil, wrapError("scan team", scanErr)
		}
		teams = append(teams, *team)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapError("iterate teams", err)
	}
	return teams, nil
}

func (r *Repository) UpdateTeam(ctx context.Context, team *domain.Teams) error {
	applyTeamDefaults(team)

	row := r.db.QueryRow(ctx, `
		UPDATE teams
		SET name = $2,
			telegram_chat_id = $3,
			timezone = $4,
			publish_local_time = $5,
			workdays = $6,
			late_policy = $7,
			last_published_standup_date = $8
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
		team.ID,
		team.Name,
		team.TelegramChatID,
		team.Timezone,
		team.PublishLocalTime.Format("15:04:05"),
		workdaysToSmallInts(team.Workdays),
		team.LatePolicy,
		nullableDate(team.LastPublishedStandupDate),
	)

	updated, err := scanTeam(row)
	if err != nil {
		return wrapError("update team", err)
	}
	*team = *updated
	return nil
}

func (r *Repository) SoftDeleteTeam(ctx context.Context, teamID uuid.UUID) error {
	commandTag, err := r.db.Exec(ctx, `
		UPDATE teams
		SET deleted_at = now()
		WHERE id = $1 AND deleted_at IS NULL`, teamID)
	if err != nil {
		return wrapError("soft delete team", err)
	}
	return ensureAffected("soft delete team", commandTag.RowsAffected())
}

func (r *Repository) SaveUserInTeamByChatID(
	ctx context.Context,
	userID uuid.UUID,
	teamID uuid.UUID,
) error {
	commandTag, err := r.db.Exec(ctx, `
		INSERT INTO team_members (
			team_id, user_id, role, is_owner, full_name, status
		)
		SELECT
			t.id,
			u.id,
			'',
			false,
			COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), 'Участник'),
			'active'
		FROM users u
		JOIN teams t ON t.id = $2 AND t.deleted_at IS NULL
		WHERE u.id = $1 AND u.deleted_at IS NULL
		ON CONFLICT (team_id, user_id) DO UPDATE
		SET full_name = EXCLUDED.full_name,
			status = 'active',
			deleted_at = NULL`, userID, teamID)
	if err != nil {
		return wrapError("save user in team", err)
	}
	return ensureAffected("save user in team", commandTag.RowsAffected())
}

func (r *Repository) SaveUserRoleInTeam(
	ctx context.Context,
	userID uuid.UUID,
	teamID uuid.UUID,
	role string,
) error {
	commandTag, err := r.db.Exec(ctx, `
		UPDATE team_members
		SET role = $3
		WHERE user_id = $1
			AND team_id = $2
			AND deleted_at IS NULL`, userID, teamID, role)
	if err != nil {
		return wrapError("save user role in team", err)
	}
	return ensureAffected("save user role in team", commandTag.RowsAffected())
}

func (r *Repository) GetTeamMembers(ctx context.Context, teamID uuid.UUID) ([]domain.TeamMembers, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			tm.team_id,
			tm.user_id,
			tm.role,
			tm.is_owner,
			tm.full_name,
			tm.status,
			tm.created_at,
			tm.deleted_at
		FROM team_members tm
		JOIN users u ON u.id = tm.user_id AND u.deleted_at IS NULL
		WHERE tm.team_id = $1
			AND tm.deleted_at IS NULL
			AND tm.status = 'active'
		ORDER BY tm.created_at`, teamID)
	if err != nil {
		return nil, wrapError("get team members", err)
	}
	defer rows.Close()

	members := make([]domain.TeamMembers, 0)
	for rows.Next() {
		var member domain.TeamMembers
		if err := rows.Scan(
			&member.TeamID,
			&member.UserID,
			&member.Role,
			&member.IsOwner,
			&member.FullName,
			&member.Status,
			&member.CreatedAt,
			&member.DeletedAt,
		); err != nil {
			return nil, wrapError("scan team member", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapError("iterate team members", err)
	}
	return members, nil
}

func (r *Repository) MarkDigestPublished(ctx context.Context, teamID uuid.UUID, date time.Time) error {
	commandTag, err := r.db.Exec(ctx, `
		UPDATE teams
		SET last_published_standup_date = $2::date
		WHERE id = $1 AND deleted_at IS NULL`, teamID, date.Format(time.DateOnly))
	if err != nil {
		return wrapError("mark digest published", err)
	}
	return ensureAffected("mark digest published", commandTag.RowsAffected())
}

func scanTeam(row rowScanner) (*domain.Teams, error) {
	team := &domain.Teams{}
	var publishTime pgtype.Time
	var workdays []int16
	var lastPublished pgtype.Date

	err := row.Scan(
		&team.ID,
		&team.Name,
		&team.TelegramChatID,
		&team.Timezone,
		&publishTime,
		&workdays,
		&team.LatePolicy,
		&lastPublished,
		&team.CreatedAt,
		&team.DeletedAt,
	)
	if err != nil {
		return nil, err
	}

	if publishTime.Valid {
		duration := time.Duration(publishTime.Microseconds) * time.Microsecond
		team.PublishLocalTime = time.Date(
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
	team.Workdays = smallIntsToWorkdays(workdays)
	if lastPublished.Valid {
		team.LastPublishedStandupDate = lastPublished.Time
	}

	return team, nil
}

func applyTeamDefaults(team *domain.Teams) {
	if team.Timezone == "" {
		team.Timezone = "Europe/Moscow"
	}
	if len(team.Workdays) == 0 {
		team.Workdays = []int{1, 2, 3, 4, 5}
	}
	if team.LatePolicy == "" {
		team.LatePolicy = "NEXT_DIGEST"
	}
}

func workdaysToSmallInts(workdays []int) []int16 {
	result := make([]int16, len(workdays))
	for index, workday := range workdays {
		result[index] = int16(workday)
	}
	return result
}

func smallIntsToWorkdays(workdays []int16) []int {
	result := make([]int, len(workdays))
	for index, workday := range workdays {
		result[index] = int(workday)
	}
	return result
}

func nullableDate(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.Format(time.DateOnly)
}
