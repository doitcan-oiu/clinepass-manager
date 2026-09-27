#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
UNIT_SRC="$ROOT/auto/deploy/clinepass-auto.service"
UNIT_DST="/etc/systemd/system/clinepass-auto.service"

have() { command -v "$1" >/dev/null 2>&1; }

listen_url() {
 local addr="${AUTO_ADDR:-:9998}"
 if [[ -z "${AUTO_ADDR:-}" && -f "$ROOT/config.yaml" ]]; then
  local line
  line="$(grep -E '^[[:space:]]*auto_addr:' "$ROOT/config.yaml" | head -1 || true)"
  line="${line#*auto_addr:}"
  line="${line%%#*}"
  line="${line// /}"
  line="${line//\"/}"
  line="${line//\'/}"
  [[ -z "$line" ]] || addr="$line"
 fi
 local host="${addr%:*}" port="${addr##*:}"
 [[ -n "$host" && "$host" != "0.0.0.0" && "$host" != "::" ]] || host="127.0.0.1"
 echo "http://${host}:${port}"
}
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
		sudo systemctl enable clinepass-auto
		sudo systemctl restart clinepass-auto
	else
		cp "$tmp" "$UNIT_DST"
		systemctl daemon-reload
		systemctl enable clinepass-auto
		systemctl restart clinepass-auto
	fi
	rm -f "$tmp"
	echo "==> 已发出 restart，等待 $(listen_url)/api/health"
	if ! wait_http "$(listen_url)/api/health" 60; then
		echo "==> 服务没有在 60 秒内就绪"
		if have journalctl; then
			journalctl -u clinepass-auto -n 40 --no-pager || true
		fi
		systemctl --no-pager --full status clinepass-auto || true
		exit 1
	fi
	echo "==> 已启动 clinepass-auto"
	echo "==> 工作目录 $ROOT"
	echo "==> 打开 $(listen_url)"
	if have journalctl; then
		journalctl -u clinepass-auto -n 15 --no-pager || true
	elif have systemctl; then
		systemctl --no-pager --full status clinepass-auto || true
	fi
}

wait_http() {
	local url="$1"
	local seconds="$2"
	local i
	for i in $(seq 1 "$seconds"); do
		# The binary reads YAML/env or the persisted token, supplies Bearer auth,
		# and verifies service + protocol. No credential is exposed in argv.
		if (cd "$ROOT" && "$ROOT/bin/auto" --health-check "$url") >/dev/null 2>&1; then
			echo "==> HTTP 已就绪（${i}s）"
			return 0
		fi
		if have systemctl && ! systemctl is-active --quiet clinepass-auto; then
			echo "==> clinepass-auto 已退出"
			return 1
		fi
		sleep 1
	done
	return 1
}

ensure_browser_deps

if [[ ! -x "$ROOT/bin/auto" ]]; then
	echo "缺少 $ROOT/bin/auto，请先 make build-auto"
	exit 1
fi

if have systemctl && [[ -d /run/systemd/system ]]; then
	install_systemd
	exit 0
fi

echo "==> 没有 systemd，前台启动 $ROOT/bin/auto"
cd "$ROOT"
exec "$ROOT/bin/auto"
