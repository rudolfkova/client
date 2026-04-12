# Character service (gRPC)

## Назначение

Хранит персонажей пользователей: владелец в API — **`user_id`** (как в auth/chat), UUID персонажа — **`id`** (как `id` мира в world-service). Несколько персонажей на пользователя, opaque **`data`**.

## Доступ и владение

- **Сейчас**: доверенные внутренние клиенты (game-service). Metadata **`x-service-token`** / Bearer, если в конфиге задан `service_token`.
- В теле запросов поле **`user_id`** — владелец (game подставляет `user_id` из JWT). В БД колонка по-прежнему `owner_user_id`; смысл тот же.
- Изоляция: запросы с условием **`id` + `owner_user_id`**. Чужой UUID → **NotFound**.

## Пустой персонаж

**`ResolvePlayCharacter`**: `user_id` + `id` (UUID). Нет строки → `persisted=false`, в `character` те же `user_id`/`id`, остальное пустое; первое сохранение — **`CreateCharacter`**. Есть строка → `persisted=true`.

## RPC

| RPC | Смысл |
|-----|--------|
| `ResolvePlayCharacter` | Перед сессией: из БД или пустой шаблон |
| `GetCharacter` | Строго из БД |
| `GetCharacterByDisplayName` | По `display_name` в рамках `user_id` |
| `ListCharacters` | Список (`limit`/`offset`) |
| `CreateCharacter` | Пустой `id` → сервер генерирует UUID |
| `ReplaceCharacterData` | `expected_version=0` — без optimistic lock |
| `DeleteCharacter` | Удаление |

## Порт и кодоген

- **`:50055`**. Proto: `character-service/proto/character/v1/character.proto`. **`make gen-character`**.

## Postman (gRPC + JSON)

У части версий Postman тело JSON к gRPC сопоставляется с полями **не только по имени**, но и по **порядку ключей** в объекте (как порядок полей в `message` в `.proto`). Имеет смысл располагать ключи **в порядке номеров полей** в `character.proto` (сначала поле 1, потом 2, …) и для `int64` использовать **число**, не строку.

Пошаговые сценарии: **`POSTMAN_TESTS.md`**.
