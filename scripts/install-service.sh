#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
UNIT_SRC="$ROOT/deploy/clinepass-manager.service"
UNIT_DST="/etc/systemd/system/clinepass-manager.service"

have() { command -v "$1" >/dev/null 2>&1; }

ensure_browser_deps() {
	if have Xvfb; then
		echo "==> Xvfb 已安装"
		return 0
	fi
	echo "==> 未找到 Xvfb，安装浏览器依赖"
	(cd "$ROOT" && make browser-deps)
}

write_unit() {
	if [[ ! -f "$UNIT_SRC" ]]; then
		echo "缺少 $UNIT_SRC"
		exit 1
	fi
	sed "s|__ROOT__|$ROOT|g" "$UNIT_SRC"
}

install_systemd() {
	local tmp
	tmp="$(mktemp)"
	write_unit >"$tmp"
	if have sudo; then
		sudo cp "$tmp" "$UNIT_DST"
		sudo systemctl daemon-reload
		sudo systemctl enable clinepass-manager
		sudo systemctl restart clinepass-manager
	else
		cp "$tmp" "$UNIT_DST"
		systemctl daemon-reload
		systemctl enable clinepass-manager
		systemctl restart clinepass-manager
	fi
	rm -f "$tmp"
	echo "==> 已发出 restart，等待 http://127.0.0.1:9999/api/health"
	if ! wait_http "http://127.0.0.1:9999/api/health" 60; then
		echo "==> 服务没有在 60 秒内就绪"
		if have journalctl; then
			journalctl -u clinepass-manager -n 40 --no-pager || true
		fi
		systemctl --no-pager --full status clinepass-manager || true
		exit 1
	fi
	echo "==> 已启动 clinepass-manager"
	echo "==> 工作目录 $ROOT"
	echo "==> 打开 http://127.0.0.1:9999"
	if have journalctl; then
		journalctl -u clinepass-manager -n 15 --no-pager || true
	elif have systemctl; then
		systemctl --no-pager --full status clinepass-manager || true
	fi
}

wait_http() {
	local url="$1"
	local seconds="$2"
	local i
	for i in $(seq 1 "$seconds"); do
		if curl -sf "$url" >/dev/null 2>&1; then
			echo "==> HTTP 已就绪（${i}s）"
			return 0
		fi
		if have systemctl && ! systemctl is-active --quiet clinepass-manager; then
			echo "==> clinepass-manager 已退出"
			return 1
		fi
		sleep 1
	done
	return 1
}

ensure_browser_deps

if [[ ! -x "$ROOT/bin/server" ]]; then
	echo "缺少 $ROOT/bin/server，请先 make build"
	exit 1
fi

if have systemctl && [[ -d /run/systemd/system ]]; then
	install_systemd
	exit 0
fi

echo "==> 没有 systemd，前台启动 $ROOT/bin/server"
cd "$ROOT"
exec "$ROOT/bin/server"
