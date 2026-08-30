# VoiceStandup.ai

[![Go checks](https://github.com/Newo123/VoiceStandup.ai/actions/workflows/ci.yml/badge.svg)](https://github.com/Newo123/VoiceStandup.ai/actions/workflows/ci.yml)

Telegram-бот для асинхронных стендапов. Участник отправляет голосовое сообщение или текст, сервис превращает его в структурированный статус, показывает предпросмотр и по расписанию собирает командную сводку.

## Что умеет проект

- принимает голосовые и текстовые статусы в Telegram;
- распознаёт речь и выделяет секции «Сделано», «Планы» и «Блокеры»;
- показывает предпросмотр перед публикацией;
- поддерживает команды, участников и локальное расписание стендапов;
- откладывает публикацию статуса через Redis;
- формирует и отправляет командный дайджест;
- хранит пользователей, команды, статусы и статистику в PostgreSQL;
- начисляет XP, уровни и стрики;
- предоставляет REST API для Telegram Mini App с проверкой `initData`.

## Как работает основной сценарий

```text
Голос или текст
       ↓
Telegram-бот
       ↓
STT + LLM через OpenRouter
       ↓
Предпросмотр статуса
       ↓
Подтверждение пользователя
       ↓
PostgreSQL + отложенная публикация в Redis
       ↓
Командная сводка в Telegram
```

## Стек

- Go 1.26;
- Telegram Bot API;
- OpenRouter для STT и LLM;
- PostgreSQL 18;
- Redis 8;
- Goose для миграций;
- Docker Compose.

## Быстрый запуск

### Требования

- Go версии из [`go.mod`](go.mod);
- Docker с поддержкой Docker Compose;
- токен Telegram-бота от [@BotFather](https://t.me/BotFather);
- API-ключ [OpenRouter](https://openrouter.ai/).

### 1. Подготовьте переменные окружения

```bash
make env-init
```

Откройте созданный `.env` и замените значения `change_me`. Для запуска приложения обязательно нужны:

```dotenv
BOT_TOKEN=...
LLM_API_KEY=...
POSTGRES_PASSWORD=...
REDIS_PASSWORD=...
DATABASE_URL=postgresql://voice_standup:<пароль>@localhost:5432/voice_standup?sslmode=disable
```

Значения в `DATABASE_URL` должны соответствовать переменным `POSTGRES_*`.

### 2. Запустите PostgreSQL, Redis и примените миграции

```bash
make up
```

Проверить состояние контейнеров:

```bash
make ps
```

### 3. Запустите приложение

```bash
go run ./cmd/bot
```

Одновременно запустятся Telegram-бот, обработчик отложенной публикации, сервис дайджестов и HTTP API для Mini App.

Проверить HTTP-сервер:

```bash
curl http://localhost:8080/healthz
```

Ожидаемый ответ:

```json
{"status":"ok"}
```

Остановить инфраструктуру:

```bash
make down
```

Удалить контейнеры вместе с локальными данными PostgreSQL и Redis:

```bash
make down-volumes
```

## Конфигурация

Полный список настроек и значения по умолчанию находятся в [`.env.example`](.env.example).

| Переменная | Назначение | Значение по умолчанию |
|---|---|---|
| `BOT_TOKEN` | токен Telegram-бота | обязательна |
| `LLM_API_KEY` | общий ключ OpenRouter для LLM и STT | обязательна |
| `LLM_MODEL` | модель структурирования текста | `google/gemini-2.5-flash` |
| `STT_MODEL` | модель распознавания речи | `openai/whisper-large-v3-turbo` |
| `DATABASE_URL` | строка подключения приложения к PostgreSQL | обязательна |
| `REDIS_ADDR` | адрес Redis | `localhost:6379` |
| `REDIS_PASSWORD` | пароль Redis | обязательна |
| `HTTP_ADDR` | адрес REST API | `:8080` |
| `TELEGRAM_AUTH_MAX_AGE` | допустимый возраст Telegram `initData` | `5m` |
| `LOGGER_LEVEL` | уровень логирования | `debug` |

Секреты из `.env` нельзя добавлять в Git.

## REST API для Mini App

Публичный healthcheck:

```http
GET /healthz
```

Маршруты `/api/v1/*` требуют заголовок с исходной строкой `Telegram.WebApp.initData`:

```http
Authorization: tma <initData>
```

Основные группы методов:

- профиль и пользователи: `/api/v1/me`, `/api/v1/users`;
- команды и участники: `/api/v1/teams`, `/api/v1/teams/{teamID}/members`;
- отчёты: `/api/v1/reports`.

Форматы запросов, ответы и правила авторизации описаны в [`docs/http-api.md`](docs/http-api.md).

## Тесты

Статический анализ и все обычные тесты:

```bash
go vet ./...
go test -race -count=1 ./...
```

Интеграционный тест репозитория отключён по умолчанию. Запускайте его только с отдельной тестовой базой, к которой уже применены миграции:

```bash
REPOSITORY_INTEGRATION=1 \
TEST_DATABASE_URL='postgresql://user:password@localhost:5432/voice_standup_test?sslmode=disable' \
go test ./internal/standup/repository -run TestRepositoryCRUDIntegration -count=1 -v
```

Те же проверки `go vet` и `go test -race` выполняются в GitHub Actions для каждого pull request в `main`.

## Полезные команды

```bash
make help                         # показать доступные команды
make logs                        # смотреть логи всех контейнеров
make logs service=postgres       # смотреть логи PostgreSQL
make migrate-status              # проверить состояние миграций
make migrate-version             # показать текущую версию схемы
make migrate-create name=feature # создать новую SQL-миграцию
```

## Структура проекта

```text
cmd/bot/                    точка входа приложения
config/                     загрузка и проверка конфигурации
internal/core/              PostgreSQL, Redis, LLM, STT и Telegram-клиенты
internal/standup/           бизнес-сценарии стендапа
internal/transport/bot/     обработчики Telegram-бота
internal/transport/http/    REST API для Mini App
migrations/                 SQL-миграции Goose
docs/                       схемы и документация API
```

## Документация

- [REST API для Mini App](docs/http-api.md)
- [Схема базы данных](docs/database-schema.puml)
- [Последовательность обработки стендапа](docs/standup-sequence.puml)
- [Работа с миграциями](migrations/README.md)

## Статус

Проект находится на стадии MVP. Основной сквозной сценарий — от сообщения пользователя до командной сводки — реализован; интерфейс и продуктовые сценарии продолжают дорабатываться.
