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
#   # Локально, gRPC на удалённом хосте:
#   make character-web TOKEN=секрет GRPC_HOST=157.22.231.18
#   # Само приложение + HTTP на 0.0.0.0, gRPC на той же машине (VPS, рядом с character-service):
#   make character-web-vps TOKEN=секрет
#   make character-web LISTEN=127.0.0.1:9999
#
# Сборка бинаря Linux для копирования на сервер: make build-character-web-linux
#   На VPS: рядом с client должен лежать репо grpc_auth (см. go.mod replace => ../grpc_auth/pkg/gamekit).
#   GOWORK=off — не подхватывать go.work с чужой машины (см. character-web цель).

.PHONY: character-web character-web-vps build-character-web-linux windows

LISTEN     ?= 127.0.0.1:8765
# Хост character-service; порт gRPC 50055 зашит в cmd/character-web
GRPC_HOST  ?= 127.0.0.1
DATA       ?= data

# Редактор в сети: тот же хост, что и docker-compose (character-service на 127.0.0.1:50055).
WEB_LISTEN_VPS ?= 0.0.0.0:8765
# Если character-service в другом контейнере/хосте — override: make character-web-vps GRPC_HOST=хост
# TOKEN не обязателен в make: можно export CHARACTER_WEB_SERVICE_TOKEN=... перед запуском.
ifneq ($(strip $(TOKEN)),)
TOKENARG := -token=$(TOKEN)
endif

character-web:
	GOWORK=off go run ./cmd/character-web -listen=$(LISTEN) -grpc-host=$(GRPC_HOST) -data=$(DATA) $(TOKENARG)

character-web-vps:
	$(MAKE) character-web LISTEN=$(WEB_LISTEN_VPS) GRPC_HOST=$(GRPC_HOST)

# Бинарь в dist/character-web/ — скопируй на сервер вместе с каталогом data/ (и при необходимости pkg path для go build на CI не нужен, билд с этой машины).
CHARACTER_WEB_BIN ?= dist/character-web/character-web
build-character-web-linux:
	@mkdir -p $(dir $(CHARACTER_WEB_BIN))
	GOWORK=off GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o $(CHARACTER_WEB_BIN) ./cmd/character-web
	@echo "OK: $(CHARACTER_WEB_BIN)"
	@echo "Сервер: положи рядом data/, запуск: $(CHARACTER_WEB_BIN) -listen=0.0.0.0:8765 -grpc-host=127.0.0.1 -data=./data $(TOKENARG)"

# Кросс-сборка готовой папки для Windows (amd64): exe + data + README + bat.
# Скопируйте $(WINDOWS_DIR) на ПК и запустите run-game.bat или game.exe из cmd.
WINDOWS_DIR ?= dist/windows-client
windows:
	rm -rf $(WINDOWS_DIR)
	mkdir -p $(WINDOWS_DIR)
	GOWORK=off GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o $(WINDOWS_DIR)/game.exe ./cmd/game
	GOWORK=off GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o $(WINDOWS_DIR)/editor.exe ./cmd/editor
	cp -r data $(WINDOWS_DIR)/
	cp packaging/windows-client/README.txt $(WINDOWS_DIR)/README.txt
	cp packaging/windows-client/run-game.bat $(WINDOWS_DIR)/
	cp packaging/windows-client/run-editor.bat $(WINDOWS_DIR)/
	@echo "Готово: $(WINDOWS_DIR) — перенесите всю папку на Windows."
