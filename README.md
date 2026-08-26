# ClinePass Manager

多账号登录管理：Go 提供 API、任务队列和前端托管；默认用 **Python Cloak 官方包装**（`humanize` + `geoip`）跑浏览器登录，提取 Cookie / 用户 ID / API Key / 支付链接，并转发套餐用量。

前端是 React + Vite + shadcn/ui。

## 目录

```
cmd/server/      HTTP 入口
internal/        API、任务、存储、用量、Playwright-Go 回退
worker/          Python 登录工人（Cloak 官方 launch）
scripts/         环境检测与自动安装
web/             前端
data/            运行时数据（库、浏览器配置目录、截图）
bin/server       生产二进制（make build 生成）
```

## 准备

本机需要能编译 Go、跑前端。登录工人的 Python / uv / 虚拟环境可以由 Make 自动补：

- Go 1.22+
- Node 20+
- Linux 上装 Chromium 依赖和 Xvfb（无桌面服务器必须）：`make browser-deps`

`make`、`make build`、`make ensure-env` 都会跑 `scripts/ensure-worker-env.sh`，按顺序检查：

1. Python 3（没有就 `apt` 装 `python3` / `python3-venv` / `python3-pip`）
2. [uv](https://github.com/astral-sh/uv)（没有或不能用，就走官方安装脚本）
3. 系统 `python3-venv`（有 Python 但缺 `ensurepip` 时自动装）
4. `worker/.venv` 和 `cloakbrowser`（没有或坏了就重建）

已经装好的会跳过。用 `apt` 时需要 sudo。

首次启动会在后台下载 CloakBrowser 到缓存目录（约 200MB），HTTP 先起来，不会因此 502。systemd 开了 `ProtectHome` 时，缓存和运行目录在 `data/` 下。版本和 license 在网页「设置 → 运行环境」里改，不用每次改 yaml。

## 常用命令

都在仓库根目录执行。`make` 等于 `make dev`。

| 命令 | 做什么 |
|---|---|
| `make` / `make dev` | 先 `ensure-env`，再 `go mod tidy`、装前端依赖，同时起后端 `:8080` 和 Vite `:5173` |
| `make build` | 先 `ensure-env`，构建 `web/dist`，再编译 `bin/server`（生产） |
| `make start` | `make build` 后生成 systemd 单元并启动。没有 systemd 则前台跑 `bin/server` |
| `make ensure-env` | 只检查/安装 Python、uv、工人虚拟环境，不启动服务 |
| `make worker-venv` | 与 `ensure-env` 相同（兼容旧名字） |
| `make api` | 只起后端，开发端口 `:8080`（不跑 ensure-env，不启前端） |
| `make web` | 只起 Vite 前端 `:5173` |
| `make install-web` | `cd web && npm install` |
| `make build-web` | 先 `npm install`，再 `npm run build` |
| `make tidy` | `go mod tidy` |
| `make browser-deps` | `apt` 安装 Xvfb 和 Chromium 运行库（需要 sudo） |
| `make worker-test` | 跑工人单测（`test_urls`、`test_herosms`、`test_cloak`、`test_radar`） |
| `make install-pw` | 安装 Playwright-Go 驱动（仅 `LOGIN_ENGINE=go` 回退时需要） |

单独补环境、不启动：

```bash
make ensure-env
```

## 开发

```bash
make
```

会先补齐工人环境，再安装 Go / 前端依赖，然后同时启动：

| 服务 | 地址 |
|---|---|
| 后端 API | http://127.0.0.1:8080 |
| 前端（Vite，`/api` 代理到后端） | http://127.0.0.1:5173 |

浏览器打开 **http://127.0.0.1:5173**。`Ctrl+C` 同时退出两边。后端 `go run` 就绪后才会起前端。

已经 `ensure-env` 过、只想重开服务：

```bash
make api    # 仅后端，:8080
make web    # 仅前端，:5173
```

页面：`/` 仪表盘，`/automation` 提取支付链接，`/account` 账号，`/logs` 日志，`/settings` 设置。

改 `worker/` 后不用重编 Go，重启任务即可。改 Go 代码在开发模式用 `make`（`go run`）会重新编译。

## 生产

在项目根目录一行启动（补环境、装 Xvfb、编前端/后端、按当前目录生成 systemd 单元并重启）：

```bash
make start
```

然后打开 **http://127.0.0.1:9999**。单元里的路径按仓库实际位置生成，不必手写 `/data/...`。没有 systemd 时，`make start` 会前台跑 `./bin/server`。

只构建、不装服务：

```bash
make build
./bin/server
```

`bin/server` 会自己切到项目根目录，再找 `worker/login.py`、`worker/.venv`、`web/dist`。HTTP 先监听，Cloak 二进制在后台下载。

改过 Go 或前端后，再执行一次 `make start`。只改 Python 工人时 `sudo systemctl restart clinepass-manager` 即可。不要把开发态的 `:8080` 当生产。

指定解释器或回退旧引擎：

```bash
LOGIN_PYTHON=./worker/.venv/bin/python ./bin/server
LOGIN_ENGINE=go ./bin/server          # 回退 Playwright-Go，没有官方 humanize/geoip
```

无图形界面的服务器会自动起 Xvfb，按有界面方式跑 Chrome。本机已有 `DISPLAY` 时，设置里的「无头」仍然生效。`ProtectHome` 可保留，HOME / 缓存 / `XDG_RUNTIME_DIR` 都落在 `data/`。

## 账号格式

- 谷歌：`邮箱----密码----辅助邮箱`
- 微软：`邮箱----密码`（导入批次选微软）

不要把真实账密、Cookie、代理密码写进仓库。

## 配置

端口、数据目录等可以写在项目根目录的 `config.yaml`。启动后改文件需要重启才生效。

优先级：**环境变量 > config.yaml > 代码默认值**。

`make` / `make api` 仍会设 `ADDR=:8080`，所以开发端口不受 yaml 影响。生产 `make start` 读 yaml 里的 `addr`（默认 `:9999`）。

也可以用 `CONFIG_FILE=/path/to.yaml` 指定别的文件。

## 环境变量

| 变量 | 默认 | 说明 |
|---|---|---|
| `ADDR` | `:9999` | 监听地址，也可写在 `config.yaml` 的 `addr`。`make` / `make api` 开发固定 `:8080` |
| `DATA_DIR` | `./data` | SQLite、浏览器配置目录、截图、HOME 不可写时的回退目录 |
| `HOME` | 进程用户家目录 | systemd `ProtectHome` 时请指到 `DATA_DIR/home` |
| `INVITE_URL` | `https://authkit.cline.bot` | 邀请链接，可在设置里改 |
| `HEADLESS` | `true` | 无头初始值，可在设置里改 |
| `PROXY` | 空 | 全局代理初始值，可在设置里改 |
| `MAX_CONCURRENT` | `1` | 同时登录数。Cloak 免费版通常为 1 |
| `LOGIN_ENGINE` | `python` | `python` 走官方包装；`go` 走旧 Playwright-Go |
| `LOGIN_PYTHON` | `worker/.venv/bin/python` | 工人解释器 |
| `CLOAKBROWSER_VERSION` | `151.0.7922.108.2` | 二进制初始版本，可在设置里改。151 需 Cloak license；没 key 时包装会回落免费 146 |
| `CLOAKBROWSER_CACHE_DIR` | `$HOME/.cloakbrowser` | 缓存目录。HOME 不可写时落到 `data/home/.cloakbrowser` |
| `CLOAKBROWSER_BINARY_PATH` | 空 | 跳过下载，使用本地 chrome |
| `CLOAKBROWSER_LICENSE_KEY` | 空 | Cloak key 初始值，可在设置里改。151 必须有，下载走 `cloakbrowser.dev/api/download/{version}` |

登录失败截图在 `data/screenshots/<账号ID>.png`。每个账号的浏览器配置在 `data/profiles/<账号ID>/`。
