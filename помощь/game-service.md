# game-service

WebSocket-сервис игрового мира: принимает действия игроков, крутит тик-движок, рассылает события.

## Подключение

- **Протокол:** WebSocket (текстовые JSON-сообщения).
- **Аутентификация:** JWT в query-параметре `token`. В claims ожидается поле `user_id` (число) — идентификатор игрока. Поле `email` (строка) подставляется auth-сервисом в access-токен; для старых токенов без `email` в логах будет пустая строка.
- При невалидном токене сервер отправляет одно JSON-сообщение и закрывает соединение:

```json
{
  "service": "game",
  "type": "error",
  "payload": { "message": "invalid token" }
}
```

Точный URL зависит от деплоя (например, за gateway: путь к игровому WS, как настроено в `gateway`).

## Входящие сообщения (клиент → сервер)

Каждое сообщение — JSON-объект **конверта** (envelope):

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `service` | string | да | Должно быть `"game"`. Иначе приходит `reject` с `reason: "wrong_service"`. |
| `type` | string | да | Тип действия (движок не валидирует список на уровне порта; типичные: `move`, `hit`, `spawn_tile`). Пустая строка → `reject` с `reason: "missing_type"`. |
| `payload` | JSON (raw) | нет | Произвольный JSON; интерпретация — в доменном слое. |

Примеры:

**Движение** (`payload` — координаты или дельта, в зависимости от реализации движка):

```json
{
  "service": "game",
  "type": "move",
  "payload": { "dx": 1, "dy": 0 }
}
```

**Удар** (`target_id` и положительный `damage` обязательны для применения урона):

```json
{
  "service": "game",
  "type": "hit",
  "payload": { "target_id": 42, "damage": 2 }
}
```

**Тайл (редактор)** — `type: "spawn_tile"`. В клетке `(x,y)` удаляется предыдущий тайл (если был) и создаётся новый сущностью ECS: позиция, имя текстуры, флаг коллизии. Проверок прав пока нет.

```json
{
  "service": "game",
  "type": "spawn_tile",
  "payload": { "x": 2, "y": 3, "texture": "wall", "blocks": true }
}
```

Невалидный `payload` для `move`/`hit`/`spawn_tile` движок молча игнорирует (без `reject`); отклонения по протоколу — только перечисленные `reason` выше.

Если внутренняя очередь действий переполнена, действие **не принимается**; клиент получает отдельное сообщение `reject` (см. ниже).

## Исходящие сообщения (сервер → клиент)

Формат такой же: `service`, `type`, `payload` (у `payload` на wire обычно сериализованный JSON объект события).

- **Broadcast:** события без привязки к одному пользователю уходят всем подключённым клиентам.
- **Точечно:** если у события задан получатель (по `user_id`), сообщение получают только соединения этого пользователя.

Типы событий задаёт движок. Событие **`state`** (broadcast каждый тик): `payload` содержит **`players`** (массив `{id,x,y,hp}`), **`tiles`** (массив `{x,y,texture,blocks}`), **`tick_at`**. Если `blocks: true`, в клетку нельзя войти действием `move`.

### Отклонение запроса (`type: "reject"`)

Когда запрос **не принят** (неверный сервис, нет `type`, невалидный JSON, переполнена очередь), сервер отправляет:

```json
{
  "service": "game",
  "type": "reject",
  "payload": {
    "reason": "queue_full",
    "message": "action queue is full, try again later",
    "request_type": "move",
    "request_service": "game"
  }
}
```

Поле **`reason`** — стабильный код для клиентского логирования:

| `reason` | Когда |
|----------|--------|
| `invalid_json` | Тело сообщения не распарсилось как JSON envelope. |
| `wrong_service` | `service` ≠ `"game"`. |
| `missing_type` | Пустой или отсутствующий `type`. |
| `queue_full` | Очередь действий переполнена, действие отброшено. |

`request_type` и `request_service` могут отсутствовать, если не применимо.

## Логи сервера

При успешном подключении и при закрытии соединения пишутся записи **`player connected`** / **`player disconnected`** с полями **`user_id`** и **`email`** (если email есть в JWT).

## Слои (чистая архитектура)

| Слой | Пакет | Роль |
|------|--------|------|
| Модели / DTO | `internal/domain/models` | `Action`, `Event`, `Outbound` (внутри — `gamekit.Envelope`) |
| Общий контракт с клиентом | `github.com/rudolfkova/grpc_auth/pkg/gamekit` (`pkg/gamekit`) | ECS-компоненты + JSON WS: `Envelope`, интенты, `Player`/`Tile` в `state` |
| Сбор событий тика | `internal/domain/gameplay` | `Emitter` |
| Порты | `internal/domain/ports` | `GameEngine` — `ProcessTick` (зависимость приложения от абстракции) |
| Прикладной сервис | `internal/app/game` | Тикер, очередь действий, маршаллинг в `Envelope` |
| Адаптер WS | `internal/ports/ws/game` | HTTP/WebSocket |
| Инфраструктура ECS | `internal/infrastructure/gameecs` | Ark `World`, `Engine`, системы, `SystemRegistry` |
| Сборка | `cmd/game-service` | `worldclient.GetWorld` (если задан `world_id`) → `gameecs.NewEngine(snapshot)` → `app.NewService(engine, ...)` |

## Движок: ECS (Ark)

Состояние мира — **[Ark ECS](https://github.com/mlange-42/ark)** в `internal/infrastructure/gameecs`. У игрока одна сущность на `user_id`.

**Компоненты** — пакет **`github.com/rudolfkova/grpc_auth/pkg/gamekit`** (`pkg/gamekit`, см. `pkg/gamekit/README.md`):

| Компонент | Назначение |
|-----------|------------|
| `PlayerRef` | `UserID` — связь с игроком по id из JWT |
| `GridPos` | Целочисленные `X`, `Y` (игроки и тайлы на одной сетке) |
| `TileTexture` | `Name` — строка-идентификатор текстуры для клиента |
| `TileSolid` | `Blocks` — запрет входа в клетку при `move` |
| `Speed` | `MaxStep` — допустимый диапазон `dx`/`dy` за один `move` по каждой оси: **[-MaxStep, MaxStep]**; при спавне **1**. При `MaxStep <= 0` движение не применяется. |
| `Health` | `HP`; старт **`gamekit.DefaultPlayerHP`** (10) |

**Системы** (`internal/infrastructure/gameecs`, интерфейс `System` — `Update(*TickContext)`):

- **`SystemRegistry`** — per-action системы на каждое действие, затем post-tick.
- **`MovementSystem`**, **`DamageSystem`**, **`TileSpawnSystem`** — игроки и тайлы.
- **`SnapshotSystem`** — заполняет `TickContext.Players` и `TickContext.Tiles` для `state`.

**`TickContext`** и **`PlayerEntitySink`** — внутри `gameecs`; `*gameecs.Engine` реализует `ports.GameEngine` и `PlayerEntitySink`.

Планировщика Ark отдельно нет (см. [документацию Ark](https://mlange-42.github.io/ark/)); оркестрация — `SystemRegistry.Update`.

## Разработка

Сборка из корня модуля `game-service` (зависит от `go.work` в монорепо):

```bash
go build -o /tmp/game-service ./cmd/game-service
```

Конфиг и порт — `cmd/game-service`, `internal/config`, `deploy/docker/config-game.toml`.

**Лобби / мир:** поле `world_id` в TOML или переменная окружения **`WORLD_ID`**. Если в окружении процесса переменная **`WORLD_ID` задана** (в т.ч. пустая строка), она **перекрывает** значение из файла — так удобнее прокидывать id из Docker/Kubernetes на инстанс. В `docker-compose` для `game-service` объявлен проброс `WORLD_ID` с хоста (`environment: - WORLD_ID`). Пример: `WORLD_ID=my-lobby-world docker compose up -d game-service`.

**Загрузка из world-service:** если **`world_id` непустой**, при старте выполняется gRPC **`GetWorld`** на **`world_service_addr`** (TOML или **`WORLD_SERVICE_ADDR`**). Опционально **`world_service_token`** / **`WORLD_SERVICE_TOKEN`** (`x-service-token`). Поле **`snapshot`** (bytes) должно быть **JSON от [ark-serde](https://github.com/mlange-42/ark-serde)** — тот же формат, что даёт `arkserde.Serialize(world)` для `*ecs.World` с зарегистрированными компонентами **`world.PlayerRef`**, **`world.GridPos`**, **`world.Speed`**, **`world.Health`**. Десериализация: `arkserde.Deserialize` в пустой мир, затем восстанавливается индекс `user_id → entity` (дубликаты `PlayerRef.UserID` или `UserID == 0` — ошибка старта). Пустой `snapshot` — пустой мир. Старый самодельный JSON вида `{"players":[...]}` **больше не поддерживается**. Если задан `world_id`, но пустой `world_service_addr`, процесс завершится с ошибкой.

Снимок для БД можно получить из отладочного/утилитарного кода, вызвав `arkserde.Serialize` на том же наборе компонентов, что и движок (см. тест `TestNewEngine_fromArkSerdeSnapshot`).
