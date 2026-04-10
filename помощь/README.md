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
| `gamekit.Speed` | `MaxStep` — лимит шага за один `move` |
| `gamekit.Health` | `HP` |
| `gamekit.DefaultPlayerHP` | Константа стартового HP на сервере |
| `gamekit.TileTexture` | `Name` — строка-идентификатор текстуры у клиента |
| `gamekit.TileSolid` | `Blocks` — если `true`, сервер не пускает `move` в эту клетку |

На **сервере** зарегистрированы мапперы:

- `Map4[PlayerRef, GridPos, Speed, Health]` — игроки  
- `Map3[GridPos, TileTexture, TileSolid]` — тайлы  

Клиент, который **симулирует или отображает** тот же мир через Ark, должен использовать **те же типы** в своих `MapN` / `FilterN`, иначе ark-serde и контракт разъедутся.

---

## WebSocket: константы и `Envelope`

```go
gamekit.ServiceGame   // "game"
gamekit.TypeMove      // "move"
gamekit.TypeHit       // "hit"
gamekit.TypeSpawnTile // "spawn_tile"
gamekit.TypeState     // "state"
gamekit.TypeReject    // "reject"
gamekit.TypeError     // "error"
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

**Движение**

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

**Спавн тайла (редактор)**

```json
{ "x": 2, "y": 3, "texture": "wall", "blocks": true }
```

```go
payload, _ := json.Marshal(gamekit.TileSpawnIntent{X: 2, Y: 3, Texture: "wall", Blocks: true})
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

- `Players` — `[]gamekit.Player` (`id`, `x`, `y`, `hp`)  
- `Tiles` — `[]gamekit.Tile` (`x`, `y`, `texture`, `blocks`)  
- `TickAt` — `time.Time` (JSON с сервера в формате времени Go)

---

## Что сознательно **не** в `gamekit`

- Внутренние типы очереди тика: `Action`, `Event`, `Outbound` — остаются в `game-service/internal/domain/models`.
- gRPC world-service — отдельный proto.
- Реализация систем ECS — `game-service/internal/infrastructure/gameecs`.

---

## Кратко для нейросети (контекст задачи)

> Нужно писать **Go-клиент** к **game-service** по **WebSocket**, опционально с **Ark ECS** локально.  
> Все **имена компонентов игрока/тайла** и **формы JSON** для `move` / `hit` / `spawn_tile` и для `state` брать из модуля **`github.com/rudolfkova/grpc_auth/pkg/gamekit`**.  
> Подключение: `go get github.com/rudolfkova/grpc_auth/pkg/gamekit@main` или в монорепо `replace ... => ../pkg/gamekit`.  
> Не дублировать struct’ы компонентов в клиенте — только импорт `gamekit`.
