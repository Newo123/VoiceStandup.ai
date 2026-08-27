-- +goose Up
ALTER TABLE streaks
    DROP CONSTRAINT streaks_team_id_fkey,
    DROP CONSTRAINT streaks_user_id_fkey,
    ADD CONSTRAINT streaks_team_member_fkey
        FOREIGN KEY (team_id, user_id)
        REFERENCES team_members (team_id, user_id)
        ON DELETE CASCADE;

-- +goose Down
ALTER TABLE streaks
    DROP CONSTRAINT streaks_team_member_fkey,
    ADD CONSTRAINT streaks_team_id_fkey
        FOREIGN KEY (team_id)
        REFERENCES teams (id)
        ON DELETE CASCADE,
    ADD CONSTRAINT streaks_user_id_fkey
        FOREIGN KEY (user_id)
        REFERENCES users (id)
        ON DELETE CASCADE;
