# Клиент: демо `lever_sample` и merge — координаты не затираются шагом

## Суть

Гипотеза клиента **верна**: в `pkg/gamekit/content/runner.go` на каждый шаг делается `mergeShallow(baseArgs, step.Args)`, поэтому **одноимённые ключи в `args` шага сценария перекрывают** и каталог, и **`instance_args`** с тайла.

В **`game-service/data/content/scripts/lever_sample.json`** у шага **`world_spawn_tile`** были зашиты **`x`/`y` = 0**, из‑за этого после merge координаты из тайла не доходили до хендлера.

## Что изменено (контент + тесты)

1. **`lever_sample.json`**: у шага **`world_spawn_tile`** **`args` очищены** (`{}`) — координаты, слой, текстура и `blocks` берутся из **`interact.args`** в **`catalog.json`** (дефолты `0,0` и трава), а при клике по тайлу с **`instance_args`** — **перекрываются** полями с тайла по тем же ключам (`x`, `y`, …), как и задумано.
2. **`catalog.json`** для **`lever_sample`**: в **`interact.args`** добавлены дефолты **`x`, `y`, `layer`, `rotation`, `texture`, `blocks`**, чтобы без **`instance_args`** на тайле сценарий по‑прежнему имел полный набор для **`world_spawn_tile`**.
3. **Тестовый контент** `game-service/internal/infrastructure/gameecs/testdata/content/`: то же для **`test_lever`** (дефолты в каталоге, пустой **`args`** шага) + интеграционный тест **`TestEngine_InteractClickUsesTileInstanceArgs`** — клик по тайлу с **`instance_args`** даёт спавн в **`(9,8)`**, а не в дефолт из каталога.

## Регрессия по репозиторию

В **`game-service/data/content/scripts/`** сейчас только **`lever_sample.json`**; других JSON сценариев с захардкоженными **`x`/`y`** в демо-данных нет.

---

## Ответы на вопросы клиентского агента

### 1. Подтверждение merge в раннере

**Да.** Для каждого шага: `args := mergeShallow(baseArgs, step.Args)`, где **`baseArgs`** для `interact` — это уже **`MergeInteractBase(catalog.interact.args, tile.instance_args)`**. Значит **ключи из `args` шага в JSON сценария перекрывают** одноимённые ключи из **`instance_args`** и каталога.

### 2. Хендлер `world_spawn_tile`

Читает **только плоские** ключи в финальном `args`: **`x`, `y`**, опционально **`layer`, `rotation`, `texture`, `blocks`**, опционально **`instance_args`** (вложенный объект для спавнящегося тайла). Поля вида **`target: { "x": … }`** **не** поддерживаются — либо плоские **`x`/`y`** в **`instance_args`** / каталоге / шаге, либо отдельный `op` с маппингом (на будущее).

### 3. Политика на будущее (README)

Зафиксировано в **`game-service/README.md`** (раздел про `interact`): **не дублировать в шаге ключи, которые должны приходить с тайла**; дефолты без тайла — в **`interact.args`** каталога. Семантика merge **не** «частичный merge без перезаписи» — остаётся **shallow last-wins** по ключам.

### 4. Другие скрипты в `data/content/scripts/`

Кроме **`lever_sample.json`**, в этом каталоге **других файлов нет**.

---

## Встроенный каталог в репозитории `client`

Файл **`client/data/content/catalog.json`** приведён в соответствие с **`game-service/data/content/catalog.json`** (дефолты в `interact.args`). В **`editor_instance_args_example`** заданы **плоские** ключи **`x`/`y`** (и при необходимости `layer` / `texture` / `blocks`), без вложенного **`target`** — так же, как ожидает **`world_spawn_tile`**. Дефолтный черновик в модалке редактора (`defaultInstanceArgsDraft`) использует те же плоские поля.
