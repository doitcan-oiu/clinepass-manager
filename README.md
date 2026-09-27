# ClinePass Manager

主程序负责账号池、负载均衡、OpenAI 兼容转发、套餐用量、请求日志、仪表盘和前端托管。自动化已拆成 `auto/` 下的独立 Go 程序，负责登录、提取 Cookie / 用户 ID / API Key / 支付链接、支付、虚拟卡、短信和定时续 Cookie。两个程序可以分别启动、停止和更新。

## 目录

```text
cmd/server/       主程序入口
internal/         主程序 API、配置/模型/存储公共代码、账号池、转发和统计
auto/main.go      独立自动化服务入口
auto/internal/    自动化 API、任务队列、浏览器、登录、卡台、短信、导出
auto/worker/      现有 Python Cloak 执行器及测试
auto/login-tool/  独立人工付款助手
auto/scripts/    自动化环境安装和 systemd 安装脚本
scripts/         主程序 systemd 安装脚本
web/             React + Vite 前端
data/            主程序 SQLite（运行数据）
auto/data/       Auto 独立 SQLite、连接密钥、浏览器配置和截图
```

两个 Go 程序共用仓库的 `go.mod` 和基础存储代码，但单独编译、单独运行。主程序编译依赖中不包含 Playwright、Chrome、登录任务、短信或卡台客户端。默认自动化仍使用原有 Python Cloak 执行器，Go 负责服务和调度；没有把浏览器流程重写成纯 Go。可用 `LOGIN_ENGINE=go` 选择原有 Playwright-Go 回退引擎。

## 开发

需要 Go 1.26 和 Node 20+。仅运行主程序及前端不需要 Python、CloakBrowser 或 Xvfb：

```bash
make dev                  # 主程序 :8081，前端 :5173
# 需要自动化时，另外一个终端：
make ensure-env           # 安装 Python/uv/Cloak 执行器及 Linux 浏览器依赖
make auto                 # 独立 Auto 服务 :9998，首次启动生成连接密钥
```

`make dev-all` 会准备自动化依赖并同时启动三个进程。也可以分别运行 `make api`、`make web` 和 `make auto`。本机开发也使用独立数据库，需要在设置 → Auto 连接中保存 `http://127.0.0.1:9998` 和 Auto 密钥。

Windows 可分别在终端运行：

```powershell
go run ./cmd/server
go run ./auto
# 前端另一个终端：
cd web
npm install
npm run dev
```

Windows 的 Vite 默认代理仍指向 `:8081`，开发时可先设置 `$env:ADDR=":8081"`；生产 `web/dist` 由主程序托管，无需 Vite。Windows 自动化可用 `python -m venv auto/worker/.venv`，然后用该环境的 `Scripts/python.exe -m pip install -r auto/worker/requirements.txt` 安装执行器。

## 构建与部署

### 主程序使用 Docker Compose

在主服务器安装 Docker Engine / Docker Desktop 和 Compose 插件后，在仓库根目录运行：

```bash
docker compose build
docker compose up -d
# 或合并为一条命令：
docker compose up -d --build
```

打开 `http://服务器IP:9999`。Dockerfile 会分阶段构建前端和 Go 主程序，运行镜像只包含主程序、前端静态资源及证书等运行依赖；不需要宿主机安装 Go 或 Node。Compose 只启动 `manager`，Auto 仍在另一台服务器独立启动。

数据库、账号池、连接设置和统计保存在宿主机 `./data`，整个目录挂载到 `/app/data`，重建镜像或容器不会清除这些数据。`config.yaml` 只读挂载到容器，不会被打包进镜像。容器内部固定监听 `9999`，如需修改对外端口，在仓库根目录的 `.env` 写入 `MANAGER_PORT=8080` 后重新执行 `docker compose up -d`。时区可在 `.env` 中用 `TZ=Asia/Shanghai` 指定。

启动后到设置 → Auto 连接填写 **Auto 服务器实际可访问的地址**及密钥；容器内的 `127.0.0.1` 指向容器自身。Compose 健康检查只检查主程序，不要求 Auto 在线。

```bash
docker compose logs -f manager           # 查看日志
docker compose ps                       # 查看运行及健康状态
docker compose up -d --build             # 更新代码后重新构建并启动
docker compose down                     # 停止并移除容器，保留宿主机 ./data
```

也可以单独构建镜像，再按上述 Compose 文件启动：

```bash
docker build -t clinepass-manager:local .
docker compose up -d
```

### 直接运行与 Auto 部署

```bash
make build                # 构建前端和 bin/server；不安装任何浏览器依赖
make build-auto           # 编译 bin/auto
make start                # 安装并启动 clinepass-manager.service
make start-auto           # 准备自动化依赖，安装并启动 clinepass-auto.service
# 或一次处理两套服务：
make start-all
```

生产默认主服务 `:9999`，Auto 监听 `:9998`。没有 systemd 时，上述 `start` 命令前台运行对应进程；需在不同终端启动两个服务。只想直接运行二进制可执行 `./bin/server` 和 `./bin/auto`。

两台服务器分别部署：主服务器执行 `docker compose up -d --build`（也可直接运行 `make start`），自动化服务器执行 `make start-auto`。在主程序的设置 → Auto 连接中填写 Auto 地址（如 `https://auto.example.com`）及连接密钥，测试并保存后立即生效。Auto 首次启动会生成密钥，写入自己的数据目录下的 `auto-token` 文件，并在首次启动日志显示一次；也可通过 `AUTO_TOKEN` 预先指定。跨公网连接使用 HTTPS 反向代理，Auto 端口可仅允许主服务器访问。

浏览器只访问主程序，主程序携带保存的密钥调用 Auto，包括任务日志流。Auto 的全部接口均要求 Bearer 认证。自动化设置保存在远程 Auto，连接信息和转发设置保存在主程序。停止 Auto 后，负载均衡、账号池、用量统计和仪表盘继续运行；`GET /api/auto/status` 可查看连接状态。

## 独立数据与成功账号导入

主程序使用 `DATA_DIR`（默认 `./data`），Auto 使用 `AUTO_DATA_DIR`（默认 `./auto/data`），各自保存 `manager.db`，无需共享磁盘。Auto 独立保存待提取批次、浏览器资料、支付配置和任务；主程序独立保存可用账号池、负载均衡配置和统计历史。

接通后在自动化页面添加待提取账号，执行提取或自动支付。手动支付后先「检查已支付」，待订阅验证完成，再点击「导入已成功账号」；可导入全部结果或单个批次。仅已确认付款且具有 API Key、Cookie 的账号可导入。账号按邮箱去重，远端和本地使用各自的 ID，重复导入跳过未变更结果，更新后的凭据可再次导入。导入不会带入 Auto 的浏览器代理。

账号池的手动 Cookie 续期会把所需账号发送到 Auto 执行，主程序保存对应关系并定期取回成功结果；主程序重启后仍会继续检查。Auto 每日续期产生的新凭据也会自动回传给已关联的本地账号，不会自动新增账号或覆盖更新的本地凭据。切换 Auto 地址后，只同步当前连接的结果。

从旧版共享数据库升级：等待任务结束，停止两端并备份旧 `data`，将旧数据库及浏览器资料的完整副本放到 Auto 服务器的 `AUTO_DATA_DIR`，主程序保留原数据库。此后两端分别写入自己的数据目录。旧 `worker/.venv` 应在 `auto/worker/.venv` 重新建立；显式配置的 `LOGIN_PYTHON` 路径也要调整。任务队列和实时日志仍在 Auto 内存中，重启前等待任务完成。只更新 Python 时重启 `clinepass-auto` 即可。

## 配置

启动配置在 `config.yaml`，也可用 `CONFIG_FILE=/path/to.yaml` 指定。优先级为环境变量 > 配置文件 > 默认值；网页保存的业务设置会覆盖对应的启动初始值。监听地址变更需重启。

| 配置 / 环境变量 | 默认值 | 用途 |
|---|---|---|
| `addr` / `ADDR` | `:9999` | 主程序地址，`make api` / `make dev` 使用 `:8081` |
| `auto_addr` / `AUTO_ADDR` | `:9998` | Auto 监听地址 |
| `auto_url` / `AUTO_URL` | 空 | 主程序连接 Auto 的初始地址，推荐在网页保存 |
| `auto_token` / `AUTO_TOKEN` | 空 | Auto 认证密钥；Auto 未指定时自动生成，主程序需填写匹配值 |
| `data_dir` / `DATA_DIR` | `./data` | 主程序数据目录 |
| `auto_data_dir` / `AUTO_DATA_DIR` | `./auto/data` | Auto 独立数据目录，不受 `DATA_DIR` 影响 |
| `login_engine` / `LOGIN_ENGINE` | `python` | auto 的执行引擎，支持 `python` / `go` |
| `login_python` / `LOGIN_PYTHON` | 自动检测 | 优先使用 `auto/worker/.venv/bin/python`，Windows 使用 `Scripts/python.exe` |
| `CLOAKBROWSER_BINARY_PATH` | 空 | auto 使用现有浏览器，跳过下载 |
| `CLOAKBROWSER_CACHE_DIR` | `$HOME/.cloakbrowser` | auto 浏览器缓存目录 |
| `CLOAKBROWSER_LICENSE_KEY` | 空 | auto 的 Cloak License 初始值 |

设置按标签区分 Auto 连接、主程序转发与用量、远程自动化、浏览器、短信和卡台。主程序与 Auto 的代理分别设置，连接地址和密钥保存在主程序数据库中；网页保存连接后会覆盖对应的启动初始值，无需重启。

## 仪表盘与历史数据

`GET /api/dashboard?range=30d` 提供仪表盘数据，支持 `24h`、`7d`、`30d`、`90d`。请求趋势按 UTC 小时或自然日汇总（包含当前时段），请求活跃度固定显示近 180 天；账号、模型和密钥可用性为当前状态。首包耗时与输出速度只统计成功的流式请求。

请求历史汇总独立保存，清空请求明细、14 天日志保留期及 50,000 条明细上限均不会清除仪表盘历史。升级时只能回填尚存的日志，已被清理的数据无法恢复，页面会注明历史不完整。

计费来自上游同步的账号日账单，可能包括网关外的使用，且账单日期与 UTC 请求分桶可能有时区差异；它不是逐次请求扣费。未同步的日期和模型显示为空，`24h` 不把日账单拆成小时费用。账号过期后已同步账单仍保留。

## 模型目录自动同步

主程序启动时从 [ClinePass 官方文档的 Models 表格](https://docs.cline.bot/getting-started/clinepass#models)同步模型名称和 ID，之后每小时刷新。`/v1/models`、管理界面的模型选择和仪表盘模型数量共用同步后的目录；文档新增或删除模型后，下一次同步会更新整个列表，无需重启或重新构建镜像。只解析 Models 表格，不会将下线说明、调用示例或价格表中的模型误加回来。

成功结果及 HTTP 缓存标记保存在主程序 SQLite 中，重启会先加载上次成功的目录。同步失败或文档格式异常时保留原列表并重试；全新安装且尚未成功同步时，暂用程序随附的模型目录。同步只获取公开文档，不需要 Auto 在线或账号支付凭据。

设置 → 用量同步中的「模型目录同步」可查看来源、模型数量、最近成功时间及错误，并手动刷新。这与账号的「模型用量刷新」分别处理。管理接口为 `GET /api/models/sync`（状态）和 `POST /api/models/sync`（立即刷新）。

## 测试和工具

```bash
go test ./...             # 主程序、自动化与共享存储单测
make worker-test          # 原 Python 执行器单测
make pay-tool             # 生成 auto/login-tool/dist 付款助手
make install-pw           # 仅 Go 回退引擎需要的 Playwright 驱动
```

谷歌批次格式为 `邮箱----密码----辅助邮箱`；微软为 `邮箱----密码`。截图和浏览器配置分别位于 Auto 数据目录的 `screenshots/` 和 `profiles/`。不要把真实账密、Cookie、连接密钥或代理密码提交到仓库。
