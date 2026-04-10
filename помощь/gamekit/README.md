# `github.com/rudolfkova/grpc_auth/pkg/gamekit` — общий контракт game-service (Go + Ark ECS)

Этот модуль вынесен в `pkg/gamekit`, чтобы **клиент на Go** (редактор, бот, утилиты) и **`game-service`** использовали **одни и те же типы**:

- **ECS-компоненты** Ark — те же struct’ы, что регистрирует сервер в `*ecs.World` (игроки, тайлы).
- **Wire-типы** — JSON для WebSocket: обёртка сообщения, интенты действий, проекции в событии `state`.

Сервер по-прежнему живёт в `game-service`; сюда перенесено только то, что должно быть **общим контрактом**.

---

## Подключение в другом модуле Go

### Вариант A — с GitHub (рекомендуется)

После пуша в репозиторий:

```bash
go get github.com/rudolfkova/grpc_auth/pkg/gamekit@main
```

Пока нужный код только в другой ветке (например `game`), укажите имя ветки вместо `main`:

```bash
go get github.com/rudolfkova/grpc_auth/pkg/gamekit@game
```

(или `@v0.1.0`, если повесите semver-тег на коммит, где лежит этот `go.mod`.)

В `go.mod` появится `require github.com/rudolfkova/grpc_auth/pkg/gamekit v0.0.0-...` — `replace` не нужен.

### Вариант B — тот же монорепозиторий (`go.work`)

В корневом `go.work` уже есть:

```text
use ./pkg/gamekit
```

В `go.mod` вашего клиента внутри монорепо:

```go
require github.com/rudolfkova/grpc_auth/pkg/gamekit v0.0.0-00010101000000-000000000000

replace github.com/rudolfkova/grpc_auth/pkg/gamekit => ../pkg/gamekit
```

Путь в `replace` — **от каталога вашего модуля** до `pkg/gamekit` (при необходимости поправьте `../`).

Затем:

```bash
go mod tidy
```

### Вариант C — клиент вне репозитория, без `go get`

Укажите `replace` на **абсолютный или относительный путь** к клону:

```go
replace github.com/rudolfkova/grpc_auth/pkg/gamekit => /home/you/dnd/grpc_auth/pkg/gamekit
```

---

## Импорт в коде

```go
import "github.com/rudolfkova/grpc_auth/pkg/gamekit"
```

---

## ECS-компоненты (дублировать определения на клиенте нельзя — только импорт отсюда)

| Тип | Назначение |
|-----|------------|
| `gamekit.PlayerRef` | `UserID int64` — связь с JWT |
| `gamekit.GridPos` | Целочисленная клетка `X`, `Y` (игроки и тайлы на одной сетке) |
| `gamekit.Speed` | `MaxStep` — clamp дельты при записи интента `move`; один шаг по сетке за тик на сервере |
| `gamekit.Health` | `HP` |
| `gamekit.DefaultPlayerHP` | Константа стартового HP на сервере |
| `gamekit.PlayerFace` | Компонент `DX`,`DY` ∈ {-1,0,1} — взгляд игрока (в JSON state: `face_dx`, `face_dy`) |
| `gamekit.DefaultPlayerFaceDX` / `DY` | Дефолт при спавне и в state при `(0,0)` в ECS: **(1, 0)** — ось +X на сетке |
| `gamekit.TileTexture` | `Name` — строка-идентификатор текстуры у клиента |
| `gamekit.TileSolid` | `Blocks` — если `true`, участвует в блокировке клетки для `move` |
| `gamekit.TileLayer` | `Z` — слой в клетке `(x,y)`; несколько тайлов в одной клетке различаются по `Z` |
| `gamekit.TileFacing` | `RotationQuarter` — поворот, четверти по часовой стрелке `0..3` |

На **сервере** зарегистрированы мапперы:

- `Map5[PlayerRef, GridPos, Speed, Health, PlayerFace]` — игроки  
- `Map5[GridPos, TileLayer, TileFacing, TileTexture, TileSolid]` — тайлы  

Клиент, который **симулирует или отображает** тот же мир через Ark, должен использовать **те же типы** в своих `MapN` / `FilterN`, иначе ark-serde и контракт разъедутся.

---

## WebSocket: константы и `Envelope`

```go
gamekit.ServiceGame   // "game"
gamekit.TypeMove      // "move"
gamekit.TypeHit       // "hit"
gamekit.TypeSpawnTile // "spawn_tile"
gamekit.TypeClearTile // "clear_tile"
gamekit.TypeSaveWorld // "save_world"
gamekit.TypeState           // "state"
gamekit.TypeReject          // "reject"
gamekit.TypeError           // "error"
gamekit.TypeSaveWorldResult // "save_world_result"
```

Каждое сообщение клиент → сервер (и часть ответов):

```go
type Envelope struct {
    Service string          `json:"service"`
    Type    string          `json:"type"`
    Payload json.RawMessage `json:"payload"`
}
```

Правило: **`service` всегда `gamekit.ServiceGame`**, валидный JWT в query `token` на `/ws/game` — см. README `game-service`.

---

## Исходящие интенты (кладутся в `Envelope.Payload` как JSON)

**Движение** — интент направления; сервер применяет **один** шаг за игровой тик. `dx:0, dy:0` — сброс.

```json
{ "dx": 1, "dy": 0 }
```

```go
payload, _ := json.Marshal(gamekit.MoveIntent{DX: 1, DY: 0})
```

**Удар**

```json
{ "target_id": 42, "damage": 2 }
```

**Спавн тайла (редактор)** — заменяет только тайлы в `(x,y)` на слое `layer` (по умолчанию `0`). `rotation` — четверти по часовой стрелке, по умолчанию `0`; см. `gamekit.NormalizeTileRotationQuarter`.

```json
{ "x": 2, "y": 3, "layer": 0, "rotation": 0, "texture": "wall", "blocks": true }
```

```go
payload, _ := json.Marshal(gamekit.TileSpawnIntent{X: 2, Y: 3, Layer: 0, Rotation: 0, Texture: "wall", Blocks: true})
```

**Очистка слоя в клетке**

```json
{ "x": 2, "y": 3, "layer": 1 }
```

```go
payload, _ := json.Marshal(gamekit.TileClearIntent{X: 2, Y: 3, Layer: 1})
```

**Сохранение мира** (только если game-service разрешает вашему `user_id`; см. `game-service/README.md`)

```json
{ "name": "Мой мир", "description": "опционально" }
```

```go
payload, _ := json.Marshal(gamekit.SaveWorldIntent{Name: "Мой мир", Description: "опционально"})
```

Сборка конверта:

```go
env := gamekit.Envelope{
    Service: gamekit.ServiceGame,
    Type:    gamekit.TypeSpawnTile,
    Payload: payload,
}
// conn.WriteJSON(env)
```

---

## Входящее событие `state`

Сервер каждый тик шлёт broadcast `type: "state"`. Разбор `payload`:

```go
var env gamekit.Envelope
// ... ReadJSON(&env)
if env.Type == gamekit.TypeState {
    var st gamekit.StatePayload
    if err := json.Unmarshal(env.Payload, &st); err != nil { ... }
    // st.Players, st.Tiles, st.TickAt
}
```

- `Players` — `[]gamekit.Player` (`id`, `x`, `y`, `hp`, `face_dx`, `face_dy` — всегда в JSON, без `omitempty`)  
- `Tiles` — `[]gamekit.Tile` (`x`, `y`, `layer`, `rotation`, `texture`, `blocks`)  
- `TickAt` — `time.Time` (JSON с сервера в формате времени Go)

---

## Что сознательно **не** в `gamekit`

- Внутренние типы очереди тика: `Action`, `Event`, `Outbound` — остаются в `game-service/internal/domain/models`.
- gRPC world-service — отдельный proto.
- Реализация систем ECS — `game-service/internal/infrastructure/gameecs`.

---

## Кратко для нейросети (контекст задачи)

> Нужно писать **Go-клиент** к **game-service** по **WebSocket**, опционально с **Ark ECS** локально.  
Сохранение мира (game-service → world-service): `TypeSaveWorld` / `SaveWorldIntent`, ответ `TypeSaveWorldResult` / `SaveWorldResultPayload`; константа **`SnapshotSchemaVersion`**.

> Все **имена компонентов игрока/тайла** и **формы JSON** для `move` / `hit` / `spawn_tile` / `clear_tile` / `save_world` и для `state` брать из модуля **`github.com/rudolfkova/grpc_auth/pkg/gamekit`**.  
> Подключение: `go get github.com/rudolfkova/grpc_auth/pkg/gamekit@main` или в монорепо `replace ... => ../pkg/gamekit`.  
> Не дублировать struct’ы компонентов в клиенте — только импорт `gamekit`.
