# ClinePass Manager

多账号登录管理：Go 提供 API、任务队列和前端托管；默认用 **Python Cloak 官方包装**（`humanize` + `geoip`）跑浏览器登录，提取 Cookie / 用户 ID / API Key / 支付链接，并转发套餐用量。

前端是 React + Vite + shadcn/ui。

## 目录

```
cmd/server/      HTTP 入口
internal/        API、任务、存储、用量、Playwright-Go 回退
worker/          Python 登录工人（Cloak 官方 launch）
web/             前端
data/            运行时数据（库、浏览器配置目录、截图）
bin/server       生产二进制（make build 生成）
```

## 准备

- Go 1.22+
- Node 20+
- Python 3.12+（登录工人）。没有 `python3-venv` 时可用 [uv](https://github.com/astral-sh/uv)
- Linux 浏览器依赖和虚拟显示（服务器无桌面时需要 Xvfb）：

```bash
make browser-deps
```

等价于安装 `xvfb` 以及 Chromium 常用库。

首次登录会下载 CloakBrowser 到 `~/.cloakbrowser/`（约 200MB）。也可在设置里填 `CLOAKBROWSER_LICENSE_KEY`。

`make` / `make build` 会自动执行 `make worker-venv`，在 `worker/.venv` 安装 `cloakbrowser`。

## 开发

在项目根目录：

```bash
make
```

会安装 Go / 前端 / Python 工人依赖，然后同时启动：

| 服务 | 地址 |
|---|---|
| 后端 API | http://127.0.0.1:8080 |
| 前端（Vite，`/api` 代理到后端） | http://127.0.0.1:5173 |

浏览器打开 **http://127.0.0.1:5173**。`Ctrl+C` 同时退出两边。后端 `go run` 就绪后才会起前端。

分开跑：

```bash
make api    # 仅后端，:8080
make web    # 仅前端，:5173
```

页面：`/` 仪表盘，`/automation` 提取支付链接，`/account` 账号，`/logs` 日志，`/settings` 设置。

改登录工人后不用重编 Go，重启任务即可。改 Go 代码在开发模式用 `make`（`go run`）会重新编译。

## 生产

必须在项目根目录构建并启动，工人脚本和 `web/dist` 按相对路径查找：

```bash
make build
./bin/server
```

然后打开 **http://127.0.0.1:9999**（Go 托管已构建的前端）。

改过登录或设置后，要重新 `make build` 并重启 `./bin/server` 才生效。不要直接跑开发态的 `:8080` 当生产。

建议在仓库根目录启动，保证能找到：

- `worker/login.py` 和 `worker/.venv/bin/python`
- `web/dist`

指定解释器或回退旧引擎：

```bash
LOGIN_PYTHON=./worker/.venv/bin/python ./bin/server
LOGIN_ENGINE=go ./bin/server          # 回退 Playwright-Go，没有官方 humanize/geoip
```

无图形界面的服务器会自动起 Xvfb，按有界面方式跑 Chrome。本机已有 `DISPLAY` 时，设置里的「无头」仍然生效。

## 账号格式

- 谷歌：`邮箱----密码----辅助邮箱`
- 微软：`邮箱----密码`（导入批次选微软）

不要把真实账密、Cookie、代理密码写进仓库。

## 环境变量

| 变量 | 默认 | 说明 |
|---|---|---|
| `ADDR` | `:9999` | 生产监听地址。`make` / `make api` 开发固定 `:8080` |
| `DATA_DIR` | `./data` | SQLite、浏览器配置目录、截图 |
| `INVITE_URL` | `https://authkit.cline.bot` | 邀请链接，可在设置里改 |
| `HEADLESS` | `true` | 无头初始值，可在设置里改 |
| `PROXY` | 空 | 全局代理初始值，可在设置里改 |
| `MAX_CONCURRENT` | `1` | 同时登录数。Cloak 免费版通常为 1 |
| `LOGIN_ENGINE` | `python` | `python` 走官方包装；`go` 走旧 Playwright-Go |
| `LOGIN_PYTHON` | `worker/.venv/bin/python` | 工人解释器 |
| `CLOAKBROWSER_VERSION` | `146.0.7680.177.5` | 二进制版本 |
| `CLOAKBROWSER_CACHE_DIR` | `~/.cloakbrowser` | 缓存目录 |
| `CLOAKBROWSER_BINARY_PATH` | 空 | 跳过下载，使用本地 chrome |
| `CLOAKBROWSER_LICENSE_KEY` | 空 | Cloak key（可选） |

登录失败截图在 `data/screenshots/<账号ID>.png`。每个账号的浏览器配置在 `data/profiles/<账号ID>/`。
