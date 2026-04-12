# Редактор персонажей и контракт данных (для JS-клиента и нейросетей)

Документ описывает **как устроены персонажи** в репозитории `grpc_auth`: хранение в **character-service**, общий формат в **`pkg/gamekit`**, связь с **game-service** по WebSocket. Цель — чтобы фронтенд (JavaScript/TypeScript) или кодогенератор могли **аккуратно** реализовать редактор и превью без догадок по полям.

---

## 1. Две «роли» данных о персонаже

| Слой | Назначение | Где живёт |
|------|------------|-----------|
| **Persistent blob** | То, что сохраняется в БД в колонке `data` (opaque `bytes` в gRPC). JSON UTF-8. | **character-service** + контракт **`gamekit.CharacterPlayData`** |
| **Runtime в игре** | Позиция, HP, взгляд меняются каждый тик; характеристики пока **только читаются** из ECS (сервер не пересчитывает модификаторы из stats в этом документе). | **game-service** → события WebSocket **`type: "state"`** |

Редактор в основном работает с **persistent blob** и метаданными (`display_name`, `id`, …). Игровой клиент дополнительно подписывается на **`state`**, чтобы видеть актуальные `hp`, `x`, `y` и копию **`stats`**.

---

## 2. JSON в поле `Character.data` (версия 2)

Источник правды в Go: файл `pkg/gamekit/character_play_data.go` (типы `CharacterPlayData`, `CharacterStats`, константа **`CharacterDataSchemaVersion = 2`**).

### 2.1. Корневой объект `CharacterPlayData`

Сериализуется в JSON и кладётся в gRPC-поле **`bytes data`** у сообщения `Character`.

| JSON-поле | Тип в JS | Обязательность | Смысл |
|-----------|----------|------------------|--------|
| `schema_version` | `number` | Рекомендуется | Версия формата; сейчас **`2`**. Старые сохранения могли не иметь поля — сервер при разборе нормализует. |
| `x` | `number` | Да | Клетка сетки при последнем сохранении (целое). |
| `y` | `number` | Да | Клетка сетки. |
| `hp` | `number` | Да | Текущие HP при сохранении; если ≤0 при загрузке, сервер подставит дефолт. |
| `face_dx` | `number` | Да | Взгляд {-1,0,1}; пара (0,0) при загрузке заменяется на дефолт (1,0). |
| `face_dy` | `number` | Да | Взгляд {-1,0,1}. |
| `stats` | объект | Да | См. §2.2. Если объект отсутствует или «все нули», сервер подставит дефолтные 10 по всем характеристикам. |
| `sprite` | `string` | Рекомендуется | Имя листа ходьбы в клиенте: каталог `data/anim/<sprite>/<sprite>.png` (например **`Male 01-1`**, **`Female 01-2`**). Пусто после нормализации → **`Male 01-1`**. В **`state`** уходит как `players[].sprite`. |

### 2.2. Объект `stats` (`CharacterStats`)

Ключи **короткие** (`str`, `dex`, …), как в типичном JSON для D&D-подобных систем.

| JSON-ключ | TS-поле | Тип | Дефолт «нового персонажа» |
|-----------|---------|-----|---------------------------|
| `str` | strength | `number` | `10` |
| `dex` | dexterity | `number` | `10` |
| `con` | constitution | `number` | `10` |
| `int` | intelligence | `number` | `10` |
| `wis` | wisdom | `number` | `10` |
| `cha` | charisma | `number` | `10` |

**Важно:** ключ `int` в JavaScript нельзя использовать как `object.int` без кавычек — используйте `stats["int"]` или маппинг при парсинге.

### 2.3. Пример минимального тела для `CreateCharacter`

```json
{
  "schema_version": 2,
  "x": 0,
  "y": 0,
  "hp": 10,
  "face_dx": 1,
  "face_dy": 0,
  "sprite": "Male 01-1",
  "stats": {
    "str": 16,
    "dex": 14,
    "con": 13,
    "int": 8,
    "wis": 12,
    "cha": 10
  }
}
```

В gRPC это поле **`data`** типа `bytes`: UTF-8 JSON выше (не base64 внутри protobuf JSON-gateway, если используете чистый protobuf — передавайте байты строки).

### 2.4. Обратная совместимость (v1 без `stats`)

Старый формат мог выглядеть так:

```json
{"x":3,"y":4,"hp":12,"face_dx":0,"face_dy":1}
```

Сервер **`game-service`** при входе в игру вызывает **`gamekit.ParseCharacterPlayData`**: после `JSON.parse` недостающие **`stats`** становятся «все 10», **`hp`/взгляд** нормализуются по правилам в `Normalize()`.

---

## 3. gRPC API character-service (редактор / тулзы)

- **Proto:** `character-service/proto/character/v1/character.proto`
- **Порт по умолчанию:** `50055`
- **Аутентификация сервис-сервис:** metadata **`x-service-token`** (если в `config-character.toml` задан `service_token`). Значение должно совпадать с токеном клиента.

Сервис: **`character.v1.CharacterService`**.

| RPC | Когда использовать в редакторе |
|-----|--------------------------------|
| `ListCharacters` | Список персонажей пользователя (`user_id`, `limit`, `offset`). |
| `GetCharacter` | Загрузить одного по `id` + `user_id`. |
| `CreateCharacter` | Создать нового: обязательны **`display_name`**, можно передать свой **`id`** (UUID) или пусто — сервер сгенерирует. **`data`** — JSON из §2, **`schema_version`** — `2`. |
| `ReplaceCharacterData` | Обновить только blob и версию схемы; для редактора «сохранить лист» без смены имени. `expected_version: 0` — без optimistic lock (как в world-service). |
| `DeleteCharacter` | Удалить персонажа. |
| `ResolvePlayCharacter` | Обычно вызывает **game-service** перед сессией; редактору не обязателен, но можно использовать для «проверить шаблон». |

**Ограничение для браузера:** нативный gRPC из чистого JS в браузере без прокси неудобен. Варианты:

1. **Node.js**-скрипт / Electron с `@grpc/grpc-js` и сгенерированными stubs.
2. **grpcurl** / Postman для ручной отладки (см. `character-service/POSTMAN_TESTS.md`).
3. В будущем — **HTTP/JSON gateway** под редактор (отдельная задача в репозитории).

Поля запросов с **`user_id`**: в доверенном режиме их подставляет бэкенд от JWT; **не** передавайте чужой `user_id` с публичного фронта без проверки на gateway.

---

## 4. Связка с игрой (WebSocket game-service)

### 4.1. Подключение

- URL (через **gateway**): `GET /ws/game?token=<JWT>&character_id=<UUID>`  
  Gateway проксирует query на game-service (в т.ч. **`character_id`**).

### 4.2. Что делает сервер

1. Проверяет JWT → **`user_id`**.
2. Если настроен character-service: **`ResolvePlayCharacter(user_id, character_id)`**.
3. Парсит **`character.data`** как **`CharacterPlayData`** → спавн в ECS (позиция, HP, взгляд, **stats**, **sprite**).
4. При отключении **последней** вкладки пользователя — сериализует обратно в JSON и пишет **`ReplaceCharacterData`** или **`CreateCharacter`** (если персонаж ещё не был в БД).

### 4.3. Событие `state` (игровой тик)

Конверт WebSocket:

```json
{
  "service": "game",
  "type": "state",
  "payload": { ... }
}
```

`payload` — объект с полями **`players`**, **`tiles`**, **`tick_at`**. Элемент **`players[]`** (тип `gamekit.Player`):

| Поле | Тип | Смысл |
|------|-----|--------|
| `id` | number | `user_id` игрока (из JWT). |
| `x`, `y` | number | Текущая клетка. |
| `hp` | number | Текущие HP. |
| `face_dx`, `face_dy` | number | Взгляд. |
| `stats` | object | Те же ключи `str`…`cha`, что в §2.2. |
| `sprite` | string | Имя листа анимации (как `CharacterPlayData.sprite`); игровой клиент рисует из `data/anim/…`. |

Редактор **не обязан** парсить весь `state`, но для «живого превью» на сцене полезно показывать хотя бы `stats` из последнего `state` выбранного `id`.

---

## 5. Рекомендации по UX редактора (логика на клиенте)

Сервер **character-service** сейчас **не валидирует** разумность stats (нет проверки point-buy, минимума/максимума). Имеет смысл на **клиенте** (и позже на отдельном BFF):

- Ограничить диапазон (например 3–18 или 8–15 — по вашей системе).
- Показывать модификатор `(value - 10) / 2` если нужно.
- Сохранять в **`data`** всегда полный JSON из §2.1, чтобы `schema_version` и `stats` были согласованы.

Имя персонажа: поле **`display_name`** в gRPC **обязательно** при `CreateCharacter` (не пустая строка).

---

## 6. TypeScript: ориентировочные типы (скопируйте в проект)

```typescript
/** Значение gamekit.CharacterDataSchemaVersion */
export const CHARACTER_DATA_SCHEMA_VERSION = 2;

export interface CharacterStatsWire {
  str: number;
  dex: number;
  con: number;
  int: number;
  wis: number;
  cha: number;
}

/** JSON в character.data (UTF-8 bytes в gRPC) */
export interface CharacterPlayDataWire {
  schema_version?: number;
  x: number;
  y: number;
  hp: number;
  face_dx: number;
  face_dy: number;
  stats: CharacterStatsWire;
  /** например "Male 01-1" — папка в data/anim на игровом клиенте */
  sprite?: string;
}

export function defaultCharacterStats(): CharacterStatsWire {
  return { str: 10, dex: 10, con: 10, int: 10, wis: 10, cha: 10 };
}

export function defaultCharacterPlayData(): CharacterPlayDataWire {
  return {
    schema_version: CHARACTER_DATA_SCHEMA_VERSION,
    x: 0,
    y: 0,
    hp: 10,
    face_dx: 1,
    face_dy: 0,
    stats: defaultCharacterStats(),
    sprite: "Male 01-1",
  };
}
```

Сериализация для gRPC из браузера без бинарного protobuf: обычно **отдельный бэкенд** принимает JSON и вызывает gRPC; иначе используйте **grpc-web** + codegen.

---

## 7. Чеклист для нейросети / разработчика фронта

1. Хранить канонический JSON персонажа в форме **`CharacterPlayDataWire`**; не выдумывать другие имена полей для stats (использовать **`str`…`cha`**) и **`sprite`** (как в `gamekit`).
2. При загрузке строки из API всегда делать **`JSON.parse`** в UTF-8 строку из `bytes`.
3. Перед отправкой **`CreateCharacter` / `ReplaceCharacterData`**: выставить **`schema_version: 2`** в JSON и в поле **`schema_version`** сообщения protobuf (дублирование допустимо и соответствует серверу).
4. Для входа в игру: получить JWT → выбрать UUID персонажа → открыть WebSocket с **`character_id`**.
5. Не полагаться на то, что сервер отклонит «кривые» stats, пока не добавлена валидация.

---

## 8. Ссылки на исходники в репозитории

| Что | Путь |
|-----|------|
| Структуры и парсинг JSON персонажа | `pkg/gamekit/character_play_data.go` |
| Игрок в событии `state` | `pkg/gamekit/wire.go` → `Player` |
| ECS (6-й компонент stats) | `game-service/internal/infrastructure/gameecs/*.go` |
| Join/save персонажа в игре | `game-service/internal/app/game/character_session.go`, `internal/ports/ws/game/handler.go` |
| Proto | `character-service/proto/character/v1/character.proto` |

При добавлении новых полей (например инвентарь): расширяйте **`CharacterPlayData`** в `gamekit`, поднимите **`CharacterDataSchemaVersion`**, обновите этот MD и миграцию чтения в **`ParseCharacterPlayData` / `Normalize`**.
