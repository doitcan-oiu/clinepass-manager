#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VENV="$ROOT/worker/.venv"
REQ="$ROOT/worker/requirements.txt"

export PATH="${HOME}/.local/bin:${HOME}/.cargo/bin:/usr/local/bin:${PATH}"

have() { command -v "$1" >/dev/null 2>&1; }

uv_ok() {
	have uv && uv --version >/dev/null 2>&1
}

apt_install() {
	if ! have apt-get; then
		echo "当前系统没有 apt-get，请先手动安装：$*"
		return 1
	fi
	if have sudo && sudo -n true >/dev/null 2>&1; then
		sudo DEBIAN_FRONTEND=noninteractive apt-get install -y "$@"
	elif have sudo; then
		echo "==> 需要 sudo 安装：$*"
		sudo DEBIAN_FRONTEND=noninteractive apt-get install -y "$@"
	else
		echo "没有 sudo，无法安装：$*"
		return 1
	fi
}

python_bin() {
	if have python3; then
		command -v python3
	elif have python; then
		command -v python
	else
		echo ""
	fi
}

venv_usable() {
	local py="$1"
	[[ -n "$py" ]] && "$py" -c "import venv, ensurepip" >/dev/null 2>&1
}

ensure_python() {
	local py
	py="$(python_bin)"
	if [[ -n "$py" ]]; then
		echo "==> Python: $($py --version 2>&1)  ($py)"
		return 0
	fi
	echo "==> 未找到 Python，开始安装 python3"
	apt_install python3 python3-venv python3-pip
	py="$(python_bin)"
	if [[ -z "$py" ]]; then
		echo "安装 Python 失败"
		exit 1
	fi
	echo "==> Python: $($py --version 2>&1)"
}

ensure_uv() {
	if uv_ok; then
		echo "==> uv: $(uv --version 2>&1)  ($(command -v uv))"
		return 0
	fi
	echo "==> 未找到可用的 uv，开始安装"
	if have curl; then
		curl -LsSf https://astral.sh/uv/install.sh | sh
	elif have wget; then
		wget -qO- https://astral.sh/uv/install.sh | sh
	fi
	export PATH="${HOME}/.local/bin:${HOME}/.cargo/bin:${PATH}"
	if uv_ok; then
		echo "==> uv: $(uv --version 2>&1)  ($(command -v uv))"
		return 0
	fi
	echo "==> uv 未装上，将改用 python3-venv"
}

ensure_venv_module() {
	if uv_ok; then
		return 0
	fi
	local py
	py="$(python_bin)"
	if venv_usable "$py"; then
		return 0
	fi
	echo "==> python3-venv/ensurepip 不可用，开始安装"
	local ver
	ver="$("$py" -c 'import sys; print("%d.%d" % (sys.version_info.major, sys.version_info.minor))')"
	apt_install "python${ver}-venv" python3-venv python3-pip || apt_install python3-venv python3-pip
	if venv_usable "$py"; then
		return 0
	fi
	echo "==> 系统 venv 仍不可用，再试一次安装 uv"
	ensure_uv
	if uv_ok; then
		return 0
	fi
	echo "无法创建虚拟环境。请安装 uv 或 python3-venv。"
	exit 1
}

ensure_browser_deps() {
	if [[ -n "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ]]; then
		return 0
	fi
	if have Xvfb; then
		echo "==> Xvfb: $(command -v Xvfb)"
		return 0
	fi
	echo "==> 服务器没有显示器且未安装 Xvfb，开始安装浏览器依赖"
	(cd "$ROOT" && make browser-deps)
}

ensure_worker() {
	if [[ -x "$VENV/bin/python" ]] && "$VENV/bin/python" -c "import cloakbrowser" >/dev/null 2>&1; then
		echo "==> 登录工人已就绪：$VENV"
		return 0
	fi
	echo "==> 安装登录工人依赖（cloakbrowser）"
	rm -rf "$VENV"
	if uv_ok; then
		uv venv "$VENV"
		uv pip install --python "$VENV/bin/python" -r "$REQ"
	else
		"$(python_bin)" -m venv "$VENV"
		"$VENV/bin/python" -m pip install -U pip
		"$VENV/bin/python" -m pip install -r "$REQ"
	fi
	"$VENV/bin/python" -c "import cloakbrowser"
	echo "==> 登录工人已就绪：$VENV"
}

ensure_python
ensure_uv
ensure_venv_module
ensure_worker
ensure_browser_deps
