# 2026-04-18: client core refactor phase 1

## Added

- Документация по архитектурным guardrails и behavior contract.
- Application/core/infra каркас для game session message pipeline и command pipeline.

## Changed

- UI код (`gameclient`, `editor`) больше не отправляет игровые WS intents напрямую через `gamews.Send`.
- Обработка входящих `Envelope` вынесена в session pipeline с обработчиками.
- Анимация тайлсетов получила явный контекст времени рендера (`DrawOpts.AnimSeconds`) вместо глобальной фазы.

## Compatibility

- Wire-формат сообщений и типы `pkg/gamekit` не изменены.
- Поведение `state`/`tile_updates`/`reject`/`error` сохранено.
