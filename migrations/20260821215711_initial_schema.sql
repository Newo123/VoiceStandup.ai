-- +goose Up
CREATE TABLE users (
                       id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
                       telegram_user_id bigint NOT NULL UNIQUE,
                       username varchar(255),
                       display_name varchar(255),
                       locale varchar(20),
                       created_at timestamptz NOT NULL DEFAULT now(),
                       deleted_at timestamptz
);

COMMENT ON TABLE users IS
    'Пользователи Telegram, зарегистрированные в боте';

COMMENT ON COLUMN users.deleted_at IS
    'Время мягкого удаления пользователя; NULL означает, что пользователь активен';


CREATE TABLE teams (
                       id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
                       name varchar(255) NOT NULL,
                       telegram_chat_id bigint NOT NULL UNIQUE,
                       timezone varchar(100) NOT NULL DEFAULT 'Europe/Moscow',
                       publish_local_time time NOT NULL,
                       workdays smallint[] NOT NULL DEFAULT ARRAY[1, 2, 3, 4, 5],
                       late_policy varchar(30) NOT NULL DEFAULT 'NEXT_DIGEST',
                       created_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE teams IS
    'Команды, для которых бот собирает и публикует стендапы';

COMMENT ON COLUMN teams.timezone IS
    'Часовой пояс команды в формате IANA, например Europe/Moscow';

COMMENT ON COLUMN teams.publish_local_time IS
    'Локальное время публикации сводки в часовом поясе команды';

COMMENT ON COLUMN teams.workdays IS
    'Рабочие дни по ISO: 1 — понедельник, 7 — воскресенье';

COMMENT ON COLUMN teams.late_policy IS
    'Политика опоздавших статусов, например NEXT_DIGEST или ADDENDUM';


CREATE TABLE team_members (
                              team_id uuid NOT NULL
                                  REFERENCES teams(id) ON DELETE CASCADE,

                              user_id uuid NOT NULL
                                  REFERENCES users(id) ON DELETE CASCADE,

                              role varchar(30) NOT NULL DEFAULT 'MEMBER',
                              status varchar(30) NOT NULL DEFAULT 'ACTIVE',
                              joined_at timestamptz NOT NULL DEFAULT now(),

                              PRIMARY KEY (team_id, user_id)
);

COMMENT ON TABLE team_members IS
    'Участники команд и их роли';


CREATE TABLE telegram_updates (
                                  update_id bigint PRIMARY KEY,
                                  received_at timestamptz NOT NULL DEFAULT now(),
                                  processed_at timestamptz,
                                  error_code varchar(100)
);

COMMENT ON TABLE telegram_updates IS
    'Обработанные Telegram updates для защиты от повторной обработки';

COMMENT ON COLUMN telegram_updates.update_id IS
    'Уникальный идентификатор update, полученный от Telegram';


CREATE TABLE submissions (
                             id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

                             team_id uuid NOT NULL
                                 REFERENCES teams(id) ON DELETE CASCADE,

                             user_id uuid NOT NULL
                                 REFERENCES users(id) ON DELETE CASCADE,

                             standup_date date NOT NULL,
                             status varchar(30) NOT NULL DEFAULT 'RECEIVED',
                             telegram_chat_id bigint,
                             telegram_message_id bigint,

    -- Внешний ключ добавляется позже, после создания report_versions.
                             active_report_version_id uuid,

                             created_at timestamptz NOT NULL DEFAULT now(),
                             updated_at timestamptz NOT NULL DEFAULT now(),

                             UNIQUE (team_id, user_id, standup_date)
);

COMMENT ON TABLE submissions IS
    'Ежедневная сдача стендапа конкретным участником команды';

COMMENT ON COLUMN submissions.standup_date IS
    'Календарная дата стендапа в локальном часовом поясе команды';

COMMENT ON COLUMN submissions.active_report_version_id IS
    'Текущая версия отчёта, выбранная для публикации';


CREATE TABLE voice_recordings (
                                  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

                                  submission_id uuid NOT NULL
                                      REFERENCES submissions(id) ON DELETE CASCADE,

                                  telegram_file_id varchar(255) NOT NULL,
                                  telegram_file_unique_id varchar(255),
                                  object_key varchar(500),
                                  duration_seconds integer,
                                  mime_type varchar(100),
                                  retention_until timestamptz,
                                  created_at timestamptz NOT NULL DEFAULT now(),
                                  deleted_at timestamptz,

                                  CHECK (duration_seconds IS NULL OR duration_seconds >= 0)
);

COMMENT ON TABLE voice_recordings IS
    'Метаданные голосовых сообщений, отправленных для стендапа';

COMMENT ON COLUMN voice_recordings.object_key IS
    'Ключ файла в объектном хранилище; само аудио в PostgreSQL не хранится';

COMMENT ON COLUMN voice_recordings.retention_until IS
    'Время, после которого исходное аудио необходимо удалить';


CREATE TABLE transcripts (
                             id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

                             recording_id uuid NOT NULL UNIQUE
                                 REFERENCES voice_recordings(id) ON DELETE CASCADE,

                             provider varchar(100) NOT NULL,
                             model varchar(100),
                             raw_text text NOT NULL,
                             language varchar(20),
                             confidence numeric(5, 4),
                             created_at timestamptz NOT NULL DEFAULT now(),

                             CHECK (
                                 confidence IS NULL
                                     OR (confidence >= 0 AND confidence <= 1)
                                 )
);

COMMENT ON TABLE transcripts IS
    'Исходные результаты распознавания голосовых сообщений';

COMMENT ON COLUMN transcripts.raw_text IS
    'Исходный неизменяемый результат распознавания речи';


CREATE TABLE report_versions (
                                 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

                                 submission_id uuid NOT NULL
                                     REFERENCES submissions(id) ON DELETE CASCADE,

                                 transcript_id uuid
                                                    REFERENCES transcripts(id) ON DELETE SET NULL,

                                 version_no integer NOT NULL,
                                 done_text text,
                                 plans_text text,
                                 blockers_text text,
                                 state varchar(30) NOT NULL DEFAULT 'DRAFT',
                                 source varchar(30),
                                 model varchar(100),
                                 confirmed_at timestamptz,
                                 created_at timestamptz NOT NULL DEFAULT now(),

                                 UNIQUE (submission_id, version_no),

                                 CHECK (version_no > 0)
);

COMMENT ON TABLE report_versions IS
    'Версии структурированного стендап-отчёта';

COMMENT ON COLUMN report_versions.version_no IS
    'Последовательный номер версии внутри одной submission';

COMMENT ON COLUMN report_versions.done_text IS
    'Что участник уже сделал';

COMMENT ON COLUMN report_versions.plans_text IS
    'Что участник планирует сделать';

COMMENT ON COLUMN report_versions.blockers_text IS
    'Блокеры участника';

COMMENT ON COLUMN report_versions.state IS
    'Состояние версии, например DRAFT или CONFIRMED';


-- Добавляется после создания report_versions из-за циклической связи:
-- submission содержит активную версию, а версия принадлежит submission.
ALTER TABLE submissions
    ADD CONSTRAINT submissions_active_report_version_fk
        FOREIGN KEY (active_report_version_id)
            REFERENCES report_versions(id)
            ON DELETE SET NULL;


CREATE TABLE digests (
                         id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

                         team_id uuid NOT NULL
                             REFERENCES teams(id) ON DELETE CASCADE,

                         standup_date date NOT NULL,
                         status varchar(30) NOT NULL DEFAULT 'PREPARING',
                         rendered_text text,
                         telegram_message_id bigint,
                         published_at timestamptz,
                         created_at timestamptz NOT NULL DEFAULT now(),

                         UNIQUE (team_id, standup_date)
);

COMMENT ON TABLE digests IS
    'Подготовленные и опубликованные ежедневные сводки команды';

COMMENT ON COLUMN digests.rendered_text IS
    'Готовый текст сводки, отправляемый в Telegram';

COMMENT ON COLUMN digests.telegram_message_id IS
    'Идентификатор опубликованного сообщения в Telegram';


CREATE TABLE digest_items (
                              digest_id uuid NOT NULL
                                  REFERENCES digests(id) ON DELETE CASCADE,

                              report_version_id uuid NOT NULL
                                  REFERENCES report_versions(id),

                              position integer NOT NULL,

                              PRIMARY KEY (digest_id, report_version_id),
                              UNIQUE (digest_id, position),

                              CHECK (position >= 0)
);

COMMENT ON TABLE digest_items IS
    'Версии отчётов, включённые в конкретную сводку';

COMMENT ON COLUMN digest_items.position IS
    'Порядок отчёта внутри сводки';


CREATE INDEX submissions_team_date_idx
    ON submissions (team_id, standup_date);

CREATE INDEX submissions_user_date_idx
    ON submissions (user_id, standup_date);

CREATE INDEX submissions_status_idx
    ON submissions (status);

CREATE INDEX voice_recordings_submission_idx
    ON voice_recordings (submission_id);

CREATE INDEX voice_recordings_retention_idx
    ON voice_recordings (retention_until)
    WHERE deleted_at IS NULL;

CREATE INDEX report_versions_submission_idx
    ON report_versions (submission_id);

CREATE INDEX report_versions_state_idx
    ON report_versions (state);

CREATE INDEX digests_team_date_status_idx
    ON digests (team_id, standup_date, status);


-- +goose Down
DROP TABLE IF EXISTS digest_items;
DROP TABLE IF EXISTS digests;

-- Сначала разрываем циклическую связь.
ALTER TABLE IF EXISTS submissions
    DROP CONSTRAINT IF EXISTS submissions_active_report_version_fk;

DROP TABLE IF EXISTS report_versions;
DROP TABLE IF EXISTS transcripts;
DROP TABLE IF EXISTS voice_recordings;
DROP TABLE IF EXISTS submissions;
DROP TABLE IF EXISTS telegram_updates;
DROP TABLE IF EXISTS team_members;
DROP TABLE IF EXISTS teams;
DROP TABLE IF EXISTS users;