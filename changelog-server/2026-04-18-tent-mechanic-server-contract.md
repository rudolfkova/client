# 2026-04-18: Tent Mechanic Server Contract

## Context

Этот документ описывает ожидаемую серверную логику для механики палатки и дельту клиента, нужную для поддержки маппинга `item_def_id != texture`.

## User Scenario (source requirement)

- Есть предмет "сложенная палатка" (folded tent), его можно носить в инвентаре.
- Если folded tent лежит на земле, с ним можно взаимодействовать.
- При `use` (не `pickup`) folded tent разворачивается в сложный объект из 4 тайлов.
- Из этих 4 тайлов:
  - 3 тайла - визуальные, без коллизии;
  - 1 тайл - якорный, с ним можно взаимодействовать для сворачивания, и он не должен быть pickable.
- При `use` по якорному тайлу сервер сворачивает палатку:
  - удаляет все 4 тайла deployed-палатки;
  - спавнит на месте folded tent, который снова можно подобрать в инвентарь.

## Fixed Contract Decisions

- Surface policy: `anchor_only` - сворачивание только через 1 якорный тайл.
- Mapping policy: `separate_ids` - сервер может держать `texture` и `item_def_id` разными.
- Клиент не моделирует палатку как отдельную сущность: он рендерит только тайлы из `state/tile_updates`.

## Client Delta Added

В клиент добавлен резолвер `item_def_id` по тайлу:

1. сначала читается `tile.instance_args.item_def_id`;
2. если поле отсутствует/невалидно - fallback на `tile.texture`.

Это применено в двух путях:

- `interact` selection/send;
- `pickup_item` selection/send.

Файлы:

- `internal/gameclient/game.go`
- `internal/gameclient/item_def_resolver_test.go`

## Server Requirements

### 1) Deployed Tent World Model

- Deployed палатка хранится как 4 тайла (обычные `Tile`), без отдельного wire-типа.
- Сервер управляет атомарностью на своей стороне и отправляет клиенту:
  - либо `state.tiles` full snapshot;
  - либо `state.tile_updates` c нужными `upsert/remove`.

### 2) Anchor Tile Requirement

- Якорный тайл обязан иметь валидный `instance_args.item_def_id`, указывающий на каталожный id, у которого есть `interact` сценарий сворачивания.
- Нежелательно делать якорный тайл `pickable`.
- Для визуальных (неякорных) тайлов `item_def_id` не обязателен.

### 3) Folded Tent Requirement

- Folded tent на земле должен иметь каталожный `item_def_id` с `pickable: true`.
- На folded tent также доступен `interact` сценарий разворачивания (use action).

### 4) Payload Expectations

- Клиент отправляет стандартные `gamekit` intents:
  - `interact`: `item_def_id`, `click_x`, `click_y`, `click_layer`;
  - `pickup_item`: `item_def_id`, `click_x`, `click_y`, `click_layer`.
- Новый wire-type не требуется.

## Example Flows

### A. Folded -> Deployed

1. Клиент отправляет `interact` по folded tile.
2. Сервер валидирует вход.
3. Сервер удаляет folded tile (или снимает pickable состояние) и спавнит 4 deployed-тайла.
4. В `state` клиент получает 4 тайла палатки; якорный содержит `instance_args.item_def_id`.

### B. Deployed -> Folded

1. Клиент отправляет `interact` по якорному tile.
2. Сервер по сценарию удаляет 4 deployed-тайла.
3. Сервер спавнит folded tile (pickable).

## Edge Cases

- `instance_args` невалиден или без `item_def_id`: клиент fallback-ится на `texture`.
- Сервер не должен рассчитывать на локальную группировку на клиенте: клиент удаляет/добавляет только то, что пришло в `tile_updates`.
- Если якорный тайл не на верхнем слое клетки, клиент выберет верхний интерактивный тайл, поэтому у целевой клетки должен быть корректный layer-priority.
- Для `pickup_item` остаётся клиентская проверка соседства (Chebyshev <= 1).

## Acceptance Checklist

- `folded` можно подобрать.
- `use` по `folded` разворачивает палатку в 4 тайла.
- `use` по якорному тайлу сворачивает палатку и возвращает `folded`.
- На развернутой палатке pickable доступен только там, где это задумано сервером.
- Клиент отправляет `item_def_id` из `instance_args` при наличии и корректно fallback-ится на `texture`.
