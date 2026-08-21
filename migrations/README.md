# Миграции базы данных

Для управления миграциями PostgreSQL используется
[Goose](https://github.com/pressly/goose).

## Структура

```text
migrations/
├── README.md
└── 00001_initial_schema.sql
```

Каждый файл миграции содержит две секции:

```sql
-- +goose Up

CREATE TABLE example (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid()
);


-- +goose Down

DROP TABLE IF EXISTS example;
```

- `Up` применяет изменение.
- `Down` отменяет изменение.
- Goose автоматически выполняет обычную SQL-миграцию в транзакции.


## Настройка подключения

В корне проекта должен находиться локальный файл `.env`:

```dotenv
DATABASE_URL=postgresql://voice_standup:local_dev_password@localhost:5432/voice_standup?sslmode=disable
```

Файл `.env` содержит секреты и не должен попадать в Git. Шаблон без настоящих
секретов хранится в `.env.example`.

Перед запуском Goose загрузите переменные из `.env`:

```bash
set -a
source .env
set +a
```

## Запуск PostgreSQL

```bash
docker compose up -d postgres
docker compose ps
```

Контейнер PostgreSQL должен иметь статус `healthy`.

Посмотреть логи:

```bash
docker compose logs postgres
```

## Применение миграций

Применить все новые миграции:

```bash
goose -dir migrations postgres "$DATABASE_URL" up
```

Проверить состояние:

```bash
goose -dir migrations postgres "$DATABASE_URL" status
```

Посмотреть текущую версию базы:

```bash
goose -dir migrations postgres "$DATABASE_URL" version
```

## Откат

Откатить последнюю миграцию:

```bash
goose -dir migrations postgres "$DATABASE_URL" down
```

Откатить и повторно применить последнюю миграцию:

```bash
goose -dir migrations postgres "$DATABASE_URL" redo
```

Команду `down` следует использовать осторожно: откат может удалить таблицы и
данные.

## Создание новой миграции

```bash
goose -dir migrations -s create migration_name sql
```

Например:

```bash
goose -dir migrations -s create add_team_description sql
```

Будет создан файл примерно такого вида:

```text
migrations/00002_add_team_description.sql
```

Пример содержимого:

```sql
-- +goose Up

ALTER TABLE teams
    ADD COLUMN description text;


-- +goose Down

ALTER TABLE teams
    DROP COLUMN description;
```

## Правила работы

1. Миграции применяются строго по порядку.
2. Каждое изменение схемы оформляется новой миграцией.
3. Уже отправленные в общий репозиторий миграции не редактируются.
4. Для секции `Up` добавляется соответствующий откат в `Down`.
5. Таблицы и внешние ключи удаляются в обратном порядке.
6. Секреты и пароли не добавляются в SQL-файлы.
7. Перед коммитом проверяются `up`, `down` и повторный `up`.

## Проверка миграции

```bash
goose -dir migrations postgres "$DATABASE_URL" up
goose -dir migrations postgres "$DATABASE_URL" status
goose -dir migrations postgres "$DATABASE_URL" down
goose -dir migrations postgres "$DATABASE_URL" up
```

Посмотреть созданные таблицы:

```bash
docker compose exec postgres \
  psql -U voice_standup -d voice_standup -c '\dt'
```

Посмотреть структуру таблицы:

```bash
docker compose exec postgres \
  psql -U voice_standup -d voice_standup -c '\d+ submissions'
```

## Служебная таблица Goose

После первого запуска Goose автоматически создаёт таблицу `goose_db_version`.
В ней хранится информация о применённых миграциях. Изменять или удалять эту
таблицу вручную не нужно.