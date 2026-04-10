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
| `type` | string | да | Тип действия (движок не валидирует список на уровне порта; типичные: `move`, `hit`, `spawn_tile`, `clear_tile`, `save_world`). Пустая строка → `reject` с `reason: "missing_type"`. |
| `payload` | JSON (raw) | нет | Произвольный JSON; интерпретация — в доменном слое. |

Примеры:

**Движение** — `type: "move"`, `payload`: `{ "dx", "dy" }` (целые, по смыслу направление / желаемая дельта).

- Сервер хранит **последний интент** на игрока (между тиками не сбрасывается). Каждое сообщение `move` **только обновляет** этот интент (значения ограничиваются **`Speed.MaxStep`** по каждой оси, как раньше).
- **Один шаг по клеткам** выполняется **не чаще одного раза за тик симуляции** (см. `movement_apply_every_n_ticks` в конфиге; при `1` — каждый тик, при `2` — через тик и т.д.). Частота broadcast **`state`** по-прежнему задаётся **`tick_rate`**. Много одинаковых `move` в одном тике **не умножают** шаг — удобно слать «всё ещё жму вправо» каждый кадр клиента.
- Чтобы **остановиться**, отправьте `dx: 0, dy: 0` (сброс интента).
- **Диагональ** (`dx` и `dy` оба ненулевые): сервер не делает один скачок по диагонали за шаг — движение идёт **лесенкой** (по очереди по X и по Y), скорость по «диагонали визуально» ближе к ортогонали; при упоре в стену по текущей оси фаза не ломается, шаг повторится на следующем применении движения.

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

**Тайл (редактор)** — `type: "spawn_tile"`. В клетке `(x,y)` на заданном **слое** `layer` удаляется предыдущий тайл на этом же слое (если был) и создаётся новый: позиция, слой, поворот, текстура, коллизия. Другие слои в той же клетке не трогаются (например, трава на слое `0` и цветок на слое `1`). Поля `layer` и `rotation` можно опустить — по умолчанию `0`. `rotation` — четверти оборота по часовой стрелке (`0`…`3`); любое целое приводится к `0..3`. Проверок прав пока нет.

```json
{
  "service": "game",
  "type": "spawn_tile",
  "payload": { "x": 2, "y": 3, "layer": 0, "rotation": 1, "texture": "wall", "blocks": true }
}
```

**Очистка слоя** — `type: "clear_tile"`. Удаляются все тайлы в клетке `(x,y)` на указанном `layer`; остальные слои в клетке сохраняются.

```json
{
  "service": "game",
  "type": "clear_tile",
  "payload": { "x": 2, "y": 3, "layer": 1 }
}
```

**Сохранение мира в world-service** — `type: "save_world"`.

- Кто может: только соединение, у которого **`user_id` из JWT** (тот же, что при открытии `?token=...`) совпадает с **`save_world_admin_user_id`** в конфиге game-service (по умолчанию **`1`**). Идентификатор берётся **с сервера из уже проверенного токена при WS**, в payload JWT класть не нужно.
- Куда пишет: gRPC **world-service** (`world_service_addr` / `WORLD_SERVICE_ADDR`), с тем же **`world_service_token`**, что и при старте для `GetWorld`.
- Логика имени: **`name`** — отображаемое имя мира (после `TrimSpace` непустое). Если в world-service уже есть мир с таким **`name`** → **`ReplaceWorldSnapshot`** по его `id`; если нет → **`CreateWorld`** с новым UUID в качестве `id`.
- Снимок: текущий ECS-мир сериализуется **ark-serde**; в БД уходит **`schema_version`** = **`gamekit.SnapshotSchemaVersion`** (сейчас **1**).

Запрос (пример):

```json
{
  "service": "game",
  "type": "save_world",
  "payload": {
    "name": "Лобби острова",
    "description": "черновик из редактора"
  }
}
```

`description` опционален.

Ответ **не broadcast**: только тому сокету, с которого пришёл запрос. Тип **`save_world_result`**:

```json
{
  "service": "game",
  "type": "save_world_result",
  "payload": {
    "ok": true,
    "world_id": "550e8400-e29b-41d4-a716-446655440000",
    "name": "Лобби острова",
    "version": 3
  }
}
```

При ошибке: **`ok": false`**, поле **`code`** — машинное имя (см. таблицу ниже), **`message`** — текст для логов.

| `code` | Когда |
|--------|--------|
| `save_world_forbidden` | `user_id` не равен настроенному админу. |
| `save_world_disabled` | В конфиге `save_world_admin_user_id = 0` (или env обнулил id). |
| `save_world_invalid_name` | Пустое имя после обрезки пробелов или JSON payload не разобрался. |
| `save_world_unconfigured` | Не задан `world_service_addr`. |
| `save_world_serialize_failed` | Ошибка ark-serde при снимке мира. |
| `save_world_upstream_failed` | Ошибка gRPC к world-service (сеть, `AlreadyExists` при гонке и т.д.). |

Конфиг: `save_world_admin_user_id` в TOML; переопределение через **`SAVE_WORLD_ADMIN_USER_ID`**.

Невалидный `payload` для `move`/`hit`/`spawn_tile`/`clear_tile` движок молча игнорирует (без `reject`); для **`save_world`** ошибки разбора имени отражаются в **`save_world_result`**. Отклонения по протоколу уровня envelope — только перечисленные `reason` в разделе `reject` ниже.

Если внутренняя очередь действий переполнена, действие **не принимается**; клиент получает отдельное сообщение `reject` (см. ниже).

## Исходящие сообщения (сервер → клиент)

Формат такой же: `service`, `type`, `payload` (у `payload` на wire обычно сериализованный JSON объект события).

- **Broadcast:** события без привязки к одному пользователю уходят всем подключённым клиентам.
- **Точечно:** если у события задан получатель (по `user_id`), сообщение получают только соединения этого пользователя. Сейчас так уходит **`save_world_result`** (только инициатор сохранения).

Типы событий задаёт движок. Событие **`state`** (broadcast каждый тик): `payload` содержит **`players`** (массив `{id,x,y,hp,face_dx,face_dy}` — направление для анимаций, см. `pkg/gamekit`), **`tiles`** (массив `{x,y,layer,rotation,texture,blocks}`), **`tick_at`**. Если у любого тайла в клетке `blocks: true`, в клетку нельзя войти действием `move`.

**`face_dx` / `face_dy`:** целые в **{-1, 0, 1}**, совместимы с осями `move` и координатами клетки: **+X** — в сторону увеличения `x` (условно «вправо»), **+Y** — увеличения `y` (условно «вниз»). Обновляются при **успешном** шаге по сетке (в т.ч. полушаг лесенки при диагонали); при блоке стеной не меняются. При **первом спавне** игрока — **`(1, 0)`**. Ключи **всегда** присутствуют в JSON. Если в сохранённом мире в ECS было `(0, 0)`, в `state` отдаётся **`gamekit.DefaultPlayerFaceDX` / `DY`** (те же 1, 0).

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
| Сборка | `cmd/game-service` | `worldclient.GetWorld` (если задан `world_id`) → `gameecs.NewEngine(snapshot, movement_apply_every_n_ticks)` → `app.NewService(engine, ...)` |

## Движок: ECS (Ark)

Состояние мира — **[Ark ECS](https://github.com/mlange-42/ark)** в `internal/infrastructure/gameecs`. У игрока одна сущность на `user_id`.

**Компоненты** — пакет **`github.com/rudolfkova/grpc_auth/pkg/gamekit`** (`pkg/gamekit`, см. `pkg/gamekit/README.md`):

| Компонент | Назначение |
|-----------|------------|
| `PlayerRef` | `UserID` — связь с игроком по id из JWT |
| `PlayerFace` | `DX`, `DY` в {-1,0,1} — взгляд / последний успешный шаг; в `state` как `face_dx`, `face_dy` |
| `GridPos` | Целочисленные `X`, `Y` (игроки и тайлы на одной сетке) |
| `TileLayer` | `Z` — индекс слоя в клетке; несколько сущностей с разными `Z` в одной `(x,y)` |
| `TileFacing` | `RotationQuarter` — ориентация тайла, четверти по часовой стрелке `0..3` |
| `TileTexture` | `Name` — строка-идентификатор текстуры для клиента |
| `TileSolid` | `Blocks` — запрет входа в клетку при `move` (если хотя бы один тайл в клетке блокирует) |
| `Speed` | `MaxStep` — при записи интента из `move` дельта по каждой оси режется в **[-MaxStep, MaxStep]**; за один **тик** выполняется не больше одного шага с этой дельтой. При спавне **1**. При `MaxStep <= 0` движение отключено. |
| `Health` | `HP`; старт **`gamekit.DefaultPlayerHP`** (10) |

**Системы** (`internal/infrastructure/gameecs`, интерфейс `System` — `Update(*TickContext)`):

- **`SystemRegistry`** — per-action системы на каждое действие, затем post-tick.
- **`MoveIntentCaptureSystem`**, **`DamageSystem`**, **`TileSpawnSystem`**, **`TileClearSystem`** — per-action; **`MovementApplySystem`** — один шаг движения за тик по сохранённому интенту.
- **`SnapshotSystem`** — заполняет `TickContext.Players` и `TickContext.Tiles` для `state`.

**`TickContext`** и **`PlayerEntitySink`** — внутри `gameecs`; `*gameecs.Engine` реализует `ports.GameEngine` и `PlayerEntitySink`.

Планировщика Ark отдельно нет (см. [документацию Ark](https://mlange-42.github.io/ark/)); оркестрация — `SystemRegistry.Update`.

## Разработка

Сборка из корня модуля `game-service` (зависит от `go.work` в монорепо):

```bash
go build -o /tmp/game-service ./cmd/game-service
```

Конфиг и порт — `cmd/game-service`, `internal/config`, `deploy/docker/config-game.toml`. **`tick_rate`** — интервал тика симуляции и частота `state`. **`movement_apply_every_n_ticks`** — как часто именно **движение по интенту** совершает шаг (меньше нагрузка на «скорость бега» без замедления остального тика).

**Лобби / мир:** поле `world_id` в TOML или переменная окружения **`WORLD_ID`**. Если в окружении процесса переменная **`WORLD_ID` задана** (в т.ч. пустая строка), она **перекрывает** значение из файла — так удобнее прокидывать id из Docker/Kubernetes на инстанс. В `docker-compose` для `game-service` объявлен проброс `WORLD_ID` с хоста (`environment: - WORLD_ID`). Пример: `WORLD_ID=my-lobby-world docker compose up -d game-service`.

**Загрузка из world-service:** если **`world_id` непустой**, при старте выполняется gRPC **`GetWorld`** на **`world_service_addr`** (TOML или **`WORLD_SERVICE_ADDR`**). Опционально **`world_service_token`** / **`WORLD_SERVICE_TOKEN`** (`x-service-token`). Поле **`snapshot`** (bytes) должно быть **JSON от [ark-serde](https://github.com/mlange-42/ark-serde)** — тот же формат, что даёт `arkserde.Serialize(world)` для `*ecs.World` с зарегистрированными компонентами игрока **`PlayerRef`**, **`GridPos`**, **`Speed`**, **`Health`**, **`PlayerFace`** (все из `gamekit`). Десериализация: `arkserde.Deserialize` в пустой мир, затем восстанавливается индекс `user_id → entity` (дубликаты `PlayerRef.UserID` или `UserID == 0` — ошибка старта). Пустой `snapshot` — пустой мир. Старые снимки **без** `PlayerFace` могут не загрузиться или дать `(0,0)` в ECS — в **`state`** тогда подставляется **`DefaultPlayerFaceDX`/`DY`** (1, 0). Старый самодельный JSON вида `{"players":[...]}` **больше не поддерживается**. Если задан `world_id`, но пустой `world_service_addr`, процесс завершится с ошибкой.

**Сохранение из игры** (редактор): см. входящее событие **`save_world`** выше; для записи в world-service адрес gRPC должен быть задан независимо от того, задан ли **`world_id`** при старте (пустой `world_id` = пустой мир в памяти, сохранить его под именем всё равно можно).

Снимок для БД можно получить из отладочного/утилитарного кода, вызвав `arkserde.Serialize` на том же наборе компонентов, что и движок (см. тест `TestNewEngine_fromArkSerdeSnapshot`). Если в мире есть **тайлы**, в снимке должны быть зарегистрированы те же пять компонентов, что и в движке: `GridPos`, `TileLayer`, `TileFacing`, `TileTexture`, `TileSolid` (все из `gamekit`); старый формат с тремя компонентами на тайл больше не совместим.
