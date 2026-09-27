.DEFAULT_GOAL := dev

.PHONY: dev dev-all api auto web build build-auto build-all start start-auto start-all tidy install-web build-web install-pw browser-deps ensure-env worker-venv worker-test pay-tool

dev: install-web
	@echo "==> 启动主服务 :8081 和前端 :5173（需要自动化时另开 make auto）"
	@bash -c 'set -u; trap "kill 0 2>/dev/null || true" EXIT INT TERM; ADDR=:8081 go run ./cmd/server & (cd web && npm run dev) & wait'

dev-all: ensure-env install-web
	@bash -c 'set -u; trap "kill 0 2>/dev/null || true" EXIT INT TERM; ADDR=:8081 go run ./cmd/server & go run ./auto & (cd web && npm run dev) & wait'

api:
	ADDR=:8081 go run ./cmd/server

auto:
	go run ./auto

web:
	cd web && npm run dev

install-web:
	cd web && npm install

build-web: install-web
	cd web && npm run build

tidy:
	go mod tidy

install-pw:
	go run github.com/mxschmitt/playwright-go/cmd/playwright@v0.6201.0 install --with-deps

browser-deps:
	$(MAKE) -C auto browser-deps

ensure-env worker-venv:
	@bash auto/scripts/ensure-worker-env.sh

worker-test:
	$(MAKE) -C auto worker-test

build: build-web
	go build -o bin/server ./cmd/server

build-auto:
	go build -o bin/auto ./auto

build-all: build build-auto

start: build
	@bash scripts/install-service.sh

start-auto: ensure-env build-auto
	@bash auto/scripts/install-service.sh

start-all: ensure-env build-all
	@bash -c 'if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then bash scripts/install-service.sh && bash auto/scripts/install-service.sh; else trap "kill 0 2>/dev/null || true" EXIT INT TERM; ./bin/server & ./bin/auto & wait; fi'

pay-tool:
	$(MAKE) -C auto pay-tool
