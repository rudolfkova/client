# Server Guide: Client Texture Keys

Этот документ описывает, как сервер должен задавать `Tile.texture`, чтобы клиент корректно рисовал тайлы.

## 1) Supported Texture Key Formats

Клиент понимает два вида ключей:

- **Indexed tileset key**: `Base_N`
- **Single texture key**: `Name`

Где именно лежат файлы:

- tileset: `data/assets/tileSets/<Base>.png`
- single image: `data/assets/<Name>.png` (только корень `assets`, без поддиректорий)

## 2) Indexed Tileset (`Base_N`) Rules

### Base

- `Base` - имя PNG-файла без расширения.
- Пример: файл `data/assets/tileSets/tent.png` -> `Base = tent`.

### Index

- `N` - 1-based индекс тайла.
- Индексация идёт:
  - слева направо внутри строки;
  - затем сверху вниз по строкам.

Формула:

- `cols = imageWidth / 16`
- `index0 = row * cols + col`
- `wireName = Base_(index0 + 1)`

Размер ячейки в листе фиксирован: `16x16`.

## 3) Tent Example (`tent.png`)

Файл: `data/assets/tileSets/tent.png`

Сервер должен использовать ключи вида:

- `tent_1`
- `tent_2`
- `tent_3`
- ...

Номер зависит от фактического расположения квадрантов на листе:

- верхний левый квадрант листа всегда `tent_1`;
- следующий справа - `tent_2`;
- начало второй строки - `tent_(cols+1)`.

## 4) Single Texture (`Name`) Rules

Для одиночных png в `data/assets`:

- файл `data/assets/Chest.png` -> ключ `Chest`;
- файл `data/assets/grass.png` -> ключ `grass`.

Ключ не включает путь и расширение.

## 5) Fallback Behavior (Catalog IDs)

Если у `texture` нет картинки, но строка совпадает с id предмета в каталоге (`catalog.items`), клиент рисует placeholder `Chest`.

Это fallback, а не основной режим. Для стабильного рендера лучше слать валидные keys (`Base_N` или `Name`).

## 6) Interact/Pickup and `item_def_id`

Для interact/pickup клиент выбирает `item_def_id` так:

1. `tile.instance_args.item_def_id` (если присутствует и валиден),
2. иначе `tile.texture`.

Это позволяет серверу держать `item_def_id` отдельно от `texture` (например `texture: tent_1`, `item_def_id: tent_anchor`).

## 7) Valid / Invalid Examples

### Valid

- `tent_1`
- `tent_12`
- `Beach_Tile_3`
- `Chest`

### Invalid (for indexed parse)

- `tent` (нет `_N` индекса)
- `tent_0` (`N` должен быть >= 1)
- `tent_x` (индекс должен быть числом)
- `tent.png` (расширение в wire-ключе недопустимо)

## 8) Recommendations For Server

- Для сложных multi-tile объектов (например палатка 2x2) задавайте явные `texture` для каждого тайла.
- Якорный тайл для логики взаимодействия помечайте через `instance_args.item_def_id`.
- Не полагайтесь на fallback-placeholder для продакшн контента.
