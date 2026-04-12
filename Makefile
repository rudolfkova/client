# Репозиторий client (Go).
#
# --- Редактор персонажей (character-web) ---
# После запуска открой в браузере:
#   http://127.0.0.1:8765/
# (или тот адрес, что выведет сервер в логе, если поменяешь LISTEN).
# Превью и список скинов: каталог data (по умолчанию -data data) → /assets/... и GET /api/anims
# (сканирует data/anim/*/имя.png без пересборки).
# Токен character-service: переменная CHARACTER_WEB_SERVICE_TOKEN или make TOKEN=...
#
# Примеры:
#   make character-web
#   make character-web TOKEN=секрет GRPC=127.0.0.1:50055
#   make character-web LISTEN=127.0.0.1:9999

.PHONY: character-web

LISTEN ?= 127.0.0.1:8765
GRPC   ?= 127.0.0.1:50055
DATA   ?= data
# TOKEN не обязателен в make: можно export CHARACTER_WEB_SERVICE_TOKEN=... перед запуском.
ifneq ($(strip $(TOKEN)),)
TOKENARG := -token=$(TOKEN)
endif

character-web:
	go run ./cmd/character-web -listen=$(LISTEN) -grpc=$(GRPC) -data=$(DATA) $(TOKENARG)
