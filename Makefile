.DEFAULT_GOAL := dev

.PHONY: dev api web build tidy install-web build-web install-pw

dev:
	@echo "==> 安装依赖"
	go mod tidy
	cd web && npm install
	@echo "==> 启动后端 http://127.0.0.1:8080 （go run 编译期间请等待）"
	@echo "==> Ctrl+C 会同时退出两边"
	@bash -c 'set -u; \
		cleanup() { kill 0 2>/dev/null || true; }; \
		trap cleanup EXIT INT TERM; \
		go run ./cmd/server & \
		echo "==> 等待后端 /api/health ..."; \
		ready=0; \
		for i in $$(seq 1 120); do \
			if curl -sf http://127.0.0.1:8080/api/health >/dev/null 2>&1; then \
				ready=1; \
				break; \
			fi; \
			sleep 0.5; \
		done; \
		if [ "$$ready" != 1 ]; then \
			echo "==> 后端启动超时"; \
			exit 1; \
		fi; \
		echo "==> 后端已就绪，启动前端 http://127.0.0.1:5173"; \
		(cd web && npm run dev) & \
		wait'

api:
	go run ./cmd/server

web:
	cd web && npm run dev

install-web:
	cd web && npm install

build-web:
	cd web && npm run build

tidy:
	go mod tidy

install-pw:
	go run github.com/mxschmitt/playwright-go/cmd/playwright@v0.6201.0 install --with-deps

build: build-web
	go build -o bin/server ./cmd/server
