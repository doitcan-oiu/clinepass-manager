# OpenCode Go Manager

个人多账号登录管理：Go + Playwright 驱动 CloakBrowser 完成 OpenCode 谷歌登录，并提取 Cookie / 工作区 ID / API Key / userID / Stripe 支付链接。前端为 **React + Vite + shadcn/ui**。

## 目录

```
cmd/server/                 HTTP 入口
internal/
  api/                      REST + SSE
  browser/                  CloakBrowser 下载、校验、Playwright 启动
  config/                   环境变量
  job/                      登录任务队列（默认串行，免费版 1 并发）
  login/                    登录流程（按属性选择器，不匹配文案）
  model/                    账号与任务模型
  store/                    SQLite
web/                        React + Vite + shadcn/ui 前端
data/                       运行时数据（库、浏览器配置、截图）
相关操作/登录方式.md         原始操作说明
```

## 准备

1. Go 1.22+、Node 20+
2. Linux 需有 Chromium 系统依赖（CloakBrowser 同样需要），例如：

```bash
sudo apt install -y libnss3 libatk-bridge2.0-0 libdrm2 libxkbcommon0 libgbm1 libasound2t64
```

3. 首次启动会自动下载 CloakBrowser 二进制到 `~/.cloakbrowser/`（约 200MB），并用官方 Ed25519 签名校验。

也可预先安装 Playwright driver：

```bash
make install-pw
```

## 启动

一条命令即可（安装依赖，并同时拉起后端和前端）：

```bash
make
```

浏览器打开 http://127.0.0.1:5173 ，`Ctrl+C` 会同时退出两边。后端 `go run` 编译完成并开始监听后，才会启动前端，避免 Vite 代理打到尚未就绪的 `:8080`。开发模式后端固定 `:8080`。

页面：

- `/` 仪表盘
- `/automation` 登录自动化
- `/account` 账号
- `/settings` 全局代理、无头模式

也可以分开跑：`make api`（后端）、`make web`（前端）。

生产构建：

```bash
make build
./bin/server
```

然后打开 http://127.0.0.1:9999

## 账号格式

`邮箱----密码----辅助邮箱`

在前端单个添加或批量导入。不要把真实账号写进仓库。

## 环境变量

| 变量 | 默认 | 说明 |
|---|---|---|
| `ADDR` | `:9999` | 监听地址。`make` / `make api` 开发模式仍用 `:8080` |
| `DATA_DIR` | `./data` | SQLite / 配置文件 / 截图 |
| `INVITE_URL` | `https://opencode.ai/go` | 邀请链接 |
| `HEADLESS` | `true` | 无头模式默认开启，可在「设置」里改 |
| `PROXY` | 空 | 全局代理初始值，可在「设置」里改 |
| `SLOW_MO` | `0` | Playwright 操作延迟（毫秒） |
| `MAX_CONCURRENT` | `1` | 同时登录数。CloakBrowser 免费版限制 1 |
| `CLOAKBROWSER_VERSION` | `146.0.7680.177.5` | 二进制版本 |
| `CLOAKBROWSER_CACHE_DIR` | `~/.cloakbrowser` | 缓存目录 |
| `CLOAKBROWSER_BINARY_PATH` | 空 | 跳过下载，使用本地 chrome |
| `CLOAKBROWSER_LICENSE_KEY` | 空 | Pro/免费 key（可选） |

## 选择器策略

页面语言可能不一致，流程只匹配稳定属性：

- 订阅入口：`a[href="/auth"]`
- Google 登录：`a[href="/google/authorize"]`
- 账号：`#identifierId` / `name=identifier`
- 密码：`name=Passwd`
- 条款：`#gaplustosNext button`
- 授权：`div[jsname="uRHG6"] button`
- 订阅：`button[data-slot="subscribe-button"]`

登录失败时会在 `data/screenshots/<账号ID>.png` 留下截图。
