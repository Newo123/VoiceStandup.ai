# Миграции базы данных

Для управления миграциями PostgreSQL используется
[Goose](https://github.com/pressly/goose).

## Структура

```text
migrations/
├── Dockerfile
├── README.md
├── 20260821215711_initial_schema.sql
├── 20260826000000_add_user_state_and_user_stats.sql
└── 20260829000000_add_submission_format_and_active_team.sql
```

Каждый SQL-файл миграции содержит секции `Up` и `Down`:

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
- По умолчанию Goose выполняет SQL-миграцию в транзакции.

## Настройка окружения

Скопируйте шаблон переменных окружения и при необходимости измените значения:

```bash
cp .env.example .env
```

`DATABASE_URL` предназначен для приложения, запущенного на хосте. Контейнер migrator формирует внутреннюю строку подключения из `POSTGRES_DB`, `POSTGRES_USER` и `POSTGRES_PASSWORD`, используя сервис `postgres` и порт `5432`.

Если в имени пользователя, пароле или названии базы есть специальные символы URL, их нужно percent-encode.

## Запуск инфраструктуры

```bash
docker compose up -d postgres redis
docker compose ps
```

PostgreSQL и Redis должны перейти в состояние `healthy`.

## Применение миграций

Migrator находится в профиле `tools`, запускается как одноразовый контейнер и автоматически ждёт готовности PostgreSQL:

```bash
docker compose --profile tools run --rm migrator
```

Проверить состояние миграций:

```bash
docker compose --profile tools run --rm migrator status
```

Посмотреть текущую версию базы:

```bash
docker compose --profile tools run --rm migrator version
```

## Откат

Откатить последнюю миграцию:

```bash
docker compose --profile tools run --rm migrator down
```

Откатить и повторно применить последнюю миграцию:

```bash
docker compose --profile tools run --rm migrator redo
```

Команды отката следует использовать осторожно: они могут удалить таблицы и данные.

## Создание новой миграции

При локально установленном Goose:

```bash
goose -dir migrations create migration_name sql
```

Будет создан timestamped-файл примерно такого вида:

```text
migrations/20260823120000_migration_name.sql
```

## Правила работы

1. Миграции применяются строго по порядку.
2. Каждое изменение схемы оформляется новой миграцией.
3. Уже опубликованные миграции не редактируются.
4. Для секции `Up` добавляется соответствующий откат в `Down`.
5. Таблицы и внешние ключи удаляются в обратном порядке зависимостей.
6. Секреты и пароли не добавляются в SQL-файлы.
7. Перед коммитом проверяются `up`, `down` и повторный `up`.

После первого успешного запуска Goose создаёт служебную таблицу `goose_db_version`. Изменять или удалять её вручную не следует.
