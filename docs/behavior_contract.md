# Client Behavior Contract

Обязательное поведение клиента при обмене с game-service.

## Envelope Gate

- Обрабатываются только сообщения с `service == "game"`.
- Иные сервисы игнорируются без изменения world state.

## State Handling

- `type == "state"`:
  - парсить `StatePayload`;
  - обновлять `players` как полный снапшот;
  - если пришел `tiles` - это full-resync (полная замена тайлов);
  - если `tiles` нет, но есть `tile_updates` - применять дельту (`upsert`/`remove`).

## Reject/Error Handling

- `type == "reject"` - логировать reason/message/request_type/request_service.
- `type == "error"` - логировать server error message.
- Ошибки reject/error не должны ломать игровой цикл клиента.

## Reconnect/Channel Closure

- Закрытие game WS канала считается ошибкой сессии и возвращается в UI loop.
- Закрытие lobby WS канала считается ошибкой сессии и возвращается в UI loop.

## Command Rules

- UI формирует intent и вызывает app/use-case.
- Сериализация `Envelope` и запись в websocket выполняется инфраструктурным writer.
