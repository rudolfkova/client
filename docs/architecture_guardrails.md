# Client Architecture Guardrails

Документ задает обязательные правила зависимостей для клиента.

## Layers

- `internal/core` - модели и правила состояния, без `ebiten`, `websocket`, `config`.
- `internal/app` - use-cases/сессии, работает через порты.
- `internal/infra` - реализации портов (WS writer и прочие адаптеры).
- `internal/gameclient`, `internal/editor`, `cmd/*` - presentation/composition root.

## Dependency Rules

- `core` НЕ импортирует `app`, `infra`, `cmd`, `gameclient`, `editor`.
- `app` может импортировать `core` и `gamekit`, но не должен знать о конкретном UI.
- `infra` может импортировать `app` (порты) и внешние SDK.
- UI слои вызывают `app` use-cases, а не собирают wire-конверты напрямую.

## Protocol Rules

- Публичный wire-контракт берется из `pkg/gamekit`, локальные дубли DTO запрещены.
- Для новых сообщений использовать additive-first подход и сохранять backward compatibility.
- Неизвестные поля/типы от сервера должны безопасно игнорироваться.

## File Growth Rules

- Новую функциональность добавлять через новые small files в feature-папке.
- Избегать god-файлов: при росте >300-400 строк делить по сценариям.
- Все изменения протокольного поведения фиксировать в `docs/changelog-client/`.
