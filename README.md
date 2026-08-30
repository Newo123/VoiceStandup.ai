# VoiceStandup.ai

Backend-сервис Telegram-бота для асинхронных командных стендапов. Бот принимает
текстовые и голосовые отчёты, готовит их к публикации, поддерживает отложенное
подтверждение и формирует командные дайджесты.

> Фронтенд находится в отдельном репозитории и в этот проект не входит.

## Возможности

- регистрация пользователей и добавление в команды через Telegram;
- PostgreSQL-репозиторий и SQL-миграции Goose;
- Redis для кэша и отложенной публикации отчётов;
- отложенная публикация с отменой или немедленным подтверждением;
- обработка текста и голосовых сообщений через LLM/STT-провайдеры;
- расчёт геймификации и публикация дайджестов по расписанию;
- проверки Go-кода в GitHub Actions.

## Стек

- Go 1.26;
- PostgreSQL 18;
- Redis 8;
- Telegram Bot API;
- Docker Compose и Goose для инфраструктуры и миграций.

## Быстрый старт

### 1. Требования

- Go 1.26 или новее;
- Docker Desktop с Docker Compose;
- токен Telegram-бота;
- ключи LLM и STT-провайдеров.

### 2. Настройте окружение

Скопируйте шаблон:

```bash
cp .env.example .env
```

В Windows PowerShell:

```powershell
Copy-Item .env.example .env
```

Заполните в `.env` как минимум следующие переменные:

```dotenv
DATABASE_URL=postgresql://voice_standup:change_me@localhost:5432/voice_standup?sslmode=disable
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=change_me
BOT_TOKEN=your_telegram_bot_token
LLM_API_KEY=your_llm_api_key
STT_API_KEY=your_stt_api_key
```

Не коммитьте `.env`: в нём находятся секреты.

### 3. Запустите PostgreSQL и Redis

```bash
docker compose up -d postgres redis
docker compose ps
```

### 4. Примените миграции

```bash
docker compose --profile tools run --rm migrator up
```

### 5. Запустите бота

```bash
go run ./cmd/bot
```

Для остановки нажмите `Ctrl+C`.

## Полезные команды

Если установлен `make`, доступны короткие команды:

```bash
make env-init       # создать .env из шаблона
make infra-up       # запустить PostgreSQL и Redis
make migrate-up     # применить миграции
make migrate-status # проверить состояние миграций
make down           # остановить инфраструктуру
```

Полный список: `make help`.

## Проверка кода

```bash
go vet ./...
go test -count=1 ./...
```

В Pull Request автоматически запускается GitHub Actions workflow с `go vet` и
тестами с race detector.

## Структура проекта

```text
cmd/bot/                         точка входа Telegram-бота
config/                          загрузка и проверка конфигурации
internal/core/                   инфраструктурные клиенты
internal/standup/                бизнес-логика стендапов
internal/transport/              Telegram-роутинг и обработчики
migrations/                      SQL-миграции Goose
docs/                            диаграммы архитектуры и БД
```

## Отложенная публикация

Пакет `internal/standup/delayed_publish` создаёт Redis-ключ с TTL две минуты.
При его истечении worker подтверждает отчёт в PostgreSQL и запускает расчёт XP
и стриков. Пользователь может отменить публикацию — тогда статус отчёта
изменяется на `cancelled` — или подтвердить её немедленно.

Для получения событий истечения ключей Redis должен быть запущен с настройкой
`notify-keyspace-events Ex`; она уже задана в `docker-compose.yml`.

## Миграции

Подробные правила создания, применения и отката миграций находятся в
[migrations/README.md](migrations/README.md).
