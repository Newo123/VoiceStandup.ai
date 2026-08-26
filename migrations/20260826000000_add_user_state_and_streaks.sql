-- +goose Up
ALTER TABLE users
    ADD COLUMN state varchar NOT NULL DEFAULT '';

CREATE TABLE streaks (
    team_id uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    current_count integer NOT NULL DEFAULT 0,
    best_count integer NOT NULL DEFAULT 0,
    last_standup_date date,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, user_id),
    CHECK (current_count >= 0),
    CHECK (best_count >= current_count)
);

CREATE INDEX streaks_user_id_idx ON streaks (user_id);

-- +goose Down
DROP TABLE streaks;

ALTER TABLE users
    DROP COLUMN state;
