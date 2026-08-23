-- +goose Up
CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    telegram_user_id bigint NOT NULL UNIQUE,
    username varchar,
    display_name varchar,
    created_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE TABLE teams (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name varchar NOT NULL,
    telegram_chat_id bigint NOT NULL UNIQUE,
    timezone varchar NOT NULL,
    publish_local_time time NOT NULL,
    workdays smallint[] NOT NULL,
    late_policy varchar NOT NULL,
    last_published_standup_date date,
    created_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE TABLE team_members (
    team_id uuid NOT NULL REFERENCES teams(id),
    user_id uuid NOT NULL REFERENCES users(id),
    role varchar NOT NULL,
    is_owner bool NOT NULL DEFAULT FALSE,
    full_name varchar NOT NULL,
    status varchar NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    PRIMARY KEY (team_id, user_id)
);

CREATE TABLE telegram_updates (
    update_id bigint PRIMARY KEY,
    received_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE submissions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id uuid NOT NULL REFERENCES teams(id),
    user_id uuid NOT NULL REFERENCES users(id),
    standup_date date NOT NULL,
    status varchar NOT NULL,
    done_text text,
    plans_text text,
    blockers_text text,
    confirmed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz,
    UNIQUE (team_id, user_id, standup_date)
);


-- +goose Down
DROP TABLE submissions;
DROP TABLE telegram_updates;
DROP TABLE team_members;
DROP TABLE teams;
DROP TABLE users;
