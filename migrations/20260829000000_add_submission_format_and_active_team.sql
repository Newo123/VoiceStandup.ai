-- +goose Up
ALTER TABLE users
    ADD COLUMN active_team_id uuid REFERENCES teams (id) ON DELETE SET NULL;

UPDATE users AS u
SET active_team_id = membership.team_id
FROM (
    SELECT tm.user_id, tm.team_id
    FROM team_members AS tm
    JOIN (
        SELECT user_id
        FROM team_members
        WHERE deleted_at IS NULL AND status = 'active'
        GROUP BY user_id
        HAVING count(*) = 1
    ) AS single_membership ON single_membership.user_id = tm.user_id
    WHERE tm.deleted_at IS NULL AND tm.status = 'active'
) AS membership
WHERE membership.user_id = u.id;

ALTER TABLE submissions
    ADD COLUMN format varchar NOT NULL DEFAULT 'text',
    ADD CONSTRAINT submissions_format_valid CHECK (format IN ('text', 'voice'));

-- +goose Down
ALTER TABLE submissions
    DROP COLUMN format;

ALTER TABLE users
    DROP COLUMN active_team_id;
