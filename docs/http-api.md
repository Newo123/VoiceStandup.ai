# REST API для Telegram Mini App

Сервер запускается вместе с ботом на адресе из `HTTP_ADDR` (`:8080` по умолчанию).

## Авторизация

Mini App передаёт исходную строку `Telegram.WebApp.initData` в заголовке:

```http
Authorization: tma <initData>
```

Backend проверяет HMAC-подпись Telegram и `auth_date`. Максимальный возраст данных
задаётся через `TELEGRAM_AUTH_MAX_AGE` (`5m` по умолчанию). Значения из
`initDataUnsafe` не используются.

## Методы

### `GET /healthz`

Проверка процесса без авторизации.

### `GET /api/v1/me`

Возвращает пользователя, его XP/level/streak и активную команду.

### `GET /api/v1/teams`

Возвращает команды, в которых пользователь состоит активно.

### `POST /api/v1/teams`

Создаёт команду, делает пользователя владельцем и выбирает новую команду активной.

```json
{
  "name": "Backend",
  "telegram_chat_id": -1001234567890,
  "timezone": "Europe/Moscow",
  "publish_local_time": "10:30",
  "workdays": [1, 2, 3, 4, 5],
  "late_policy": "NEXT_DIGEST"
}
```

`workdays` использует ISO-нумерацию: `1` — понедельник, `7` — воскресенье.
Поддерживаемые late policy: `NEXT_DIGEST`, `SEPARATE_MESSAGE`.

### `PUT /api/v1/me/active-team`

Выбирает активную команду. Пользователь должен состоять в ней.

```json
{"team_id": "00000000-0000-0000-0000-000000000000"}
```

### `GET /api/v1/teams/{teamID}/members`

Возвращает участников команды и их статистику, отсортированную по XP. Доступен
только участникам команды.

## Ошибки

Все ошибки имеют единый JSON-формат:

```json
{
  "error": {
    "code": "forbidden",
    "message": "Нет доступа к этой команде"
  }
}
```
