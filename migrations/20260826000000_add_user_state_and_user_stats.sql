-- +goose Up
ALTER TABLE users
    ADD COLUMN state varchar NOT NULL DEFAULT '';

CREATE TABLE user_stats (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    xp integer NOT NULL DEFAULT 0,
    level integer NOT NULL DEFAULT 1,
    current_streak integer NOT NULL DEFAULT 0,
    best_streak integer NOT NULL DEFAULT 0,
    last_standup_date date,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT user_stats_xp_non_negative CHECK (xp >= 0),
    CONSTRAINT user_stats_level_positive CHECK (level >= 1),
    CONSTRAINT user_stats_current_streak_non_negative CHECK (current_streak >= 0),
    CONSTRAINT user_stats_best_streak_valid CHECK (best_streak >= current_streak)
);

CREATE INDEX user_stats_xp_idx ON user_stats (xp DESC);

INSERT INTO user_stats (user_id)
SELECT id
FROM users;

-- +goose StatementBegin
CREATE FUNCTION create_user_stats_for_new_user()
RETURNS trigger AS $$
BEGIN
    INSERT INTO user_stats (user_id) VALUES (NEW.id);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER users_create_stats
AFTER INSERT ON users
FOR EACH ROW
EXECUTE FUNCTION create_user_stats_for_new_user();

-- +goose Down
DROP TRIGGER users_create_stats ON users;
DROP FUNCTION create_user_stats_for_new_user();
DROP TABLE user_stats;

ALTER TABLE users
    DROP COLUMN state;
