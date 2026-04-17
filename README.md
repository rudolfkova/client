# dnd client

Игровой клиент и редактор мира для `grpc_auth` gateway/game-service.

## Modules

- `cmd/game` - игровой клиент (Ebiten).
- `cmd/editor` - редактор мира (Ebiten).
- `cmd/character-web` - локальный web-инструмент для персонажей.
- `internal/app` - application-слой (сессии, use-cases).
- `internal/core` - core-слой без UI/WS деталей.
- `internal/infra` - инфраструктурные адаптеры (ws writer).
- `internal/gameclient` и `internal/editor` - presentation/UI.

## Contract Sources

Источники истины по протоколу и инвариантам:

- `/home/grach/dnd/grpc_auth/game-service/docs/architecture_guardrails.md`
- `/home/grach/dnd/grpc_auth/game-service/docs/tick_invariants.md`
- `/home/grach/dnd/grpc_auth/pkg/gamekit/README.md`
- `/home/grach/dnd/grpc_auth/pkg/gamekit/COMPATIBILITY.md`
- `/home/grach/dnd/grpc_auth/gateway/GATEWAY_API.md`

## Documentation

- `docs/architecture_guardrails.md` - правила зависимостей и границ слоев.
- `docs/behavior_contract.md` - обязательное поведение клиента при WS событиях.
- `docs/changelog-client/` - изменения клиентского поведения и миграции.
