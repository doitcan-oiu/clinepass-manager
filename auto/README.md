# ClinePass Auto

独立 Go 自动化服务，可部署在与主程序不同的服务器。两端通过带密钥的 HTTP API 通信，各自拥有 SQLite 数据库，不共享目录或数据库。

## Windows（CMD / PowerShell）

Windows 不需要安装 `make`。首次使用先安装 Go 1.26+ 和 Python 3.10+，安装 Python 时启用加入 PATH 的选项，然后重新打开终端。确认 `go version` 和 `py -3 --version` 能显示版本；没有 `py` 启动器时可以使用 `python --version`。

在 `auto` 目录只运行一个文件：

```bat
start.cmd
```

PowerShell 使用 `./start.cmd`，也可以双击 `start.cmd`。脚本会自动定位仓库目录，无需安装 `make` 或手动激活 Python 环境。

`start.cmd` 会按顺序完成以下步骤，任何一步失败都会停止，并显示需要处理的错误：

1. 检查 Go 是否可用；Go 版本要求由构建时的 `go.mod` 校验。
2. 创建或复用 `auto/worker/.venv`，检查 Python 3.10+，安装 `requirements.txt` 中的执行器依赖，并验证 CloakBrowser、Playwright 可以导入。
3. 重新编译当前代码，生成仓库根目录下的 `bin/auto.exe`，避免更新代码后继续运行旧程序。
4. 前台启动 Auto，按 Ctrl+C 停止。第一次启动还会按浏览器设置准备 CloakBrowser。

已有虚拟环境会复用，已满足版本要求的依赖不会重复安装。更新后先停止旧 Auto，再运行 `start.cmd` 即可；默认每次都会检查依赖并构建，因此服务器需要可以访问缺少的依赖和 Go 模块下载源。保留完整仓库，Auto 需要读取根目录配置及 `auto/worker` 执行器。

一键启动统一使用本机的 `auto/worker/.venv/Scripts/python.exe`：脚本为本次 Auto 进程设置 `LOGIN_PYTHON`，覆盖终端及 `config.yaml` 中的旧 Python 路径，不改写全局环境变量或配置文件。首次创建环境时，会尝试有效的 `LOGIN_PYTHON`，然后查找 `py -3`、`python`、`python3`。脚本同时将终端代码页和 Python 输入输出设为 UTF-8，保证中文日志正常显示。

保留两个可选的单独操作入口，它们也调用 `start.cmd` 中的同一份逻辑：

- `setup-worker.cmd`（或 `start.cmd --setup-only`）：只准备并验证 Python 执行器，不启动 Auto。
- `build.cmd`（或 `start.cmd --build-only`）：只构建 Go 服务，不安装 Python 依赖或启动 Auto。

`start.cmd --health-check URL` 仅使用已构建的程序进行健康检查，不安装依赖、不构建、不启动服务。

也可以手动完成安装、构建和启动，在 `auto` 目录依次执行：

```bat
py -3 -m venv worker\.venv
worker\.venv\Scripts\python.exe -m pip install -r worker\requirements.txt
go build -o ..\bin\auto.exe .
..\bin\auto.exe
```

如果没有 `py` 启动器但 `python --version` 正常，将第一条命令的 `py -3` 换为 `python`。无需手动激活虚拟环境，Auto 会自动寻找 `worker/.venv/Scripts/python.exe`。完成依赖安装后，临时运行也可用 `go run .`。

服务启动后，在主程序填写 Auto 服务器地址、默认端口 `9998` 及首次启动显示的连接密钥。

### 提示「未找到 Python」

这表示正在运行的 Go 服务没有找到可用 Python。更新到支持一键启动的版本后，`start.cmd` 会在启动前准备并验证执行器，避免带着缺失的 Python 依赖启动。

1. 在 Auto 服务器安装 Python 3.10+，重新打开终端，确认 `py -3 --version` 或 `python --version` 正常。
2. 等待正在执行的任务结束，停止旧 Auto，在 `auto` 目录运行 `start.cmd`。若环境创建、依赖安装或构建失败，先处理输出的错误，然后再次运行同一个文件。
3. 若直接运行 `auto.exe` 或 `go run .`，检查显式设置的 `LOGIN_PYTHON` 环境变量、根目录 `config.yaml` 中的 `login_python`。它们优先于自动查找，不应指向不存在的文件、另一台服务器的路径或未安装依赖的 Python。通常留空即可；需要指定时，使用本机完整路径，例如 `C:\project\clinepass-manager\auto\worker\.venv\Scripts\python.exe`。一键启动已自动使用准备好的虚拟环境。
4. 在 `auto` 目录验证执行器依赖：

```bat
worker\.venv\Scripts\python.exe -c "import cloakbrowser; import playwright.sync_api; print('Worker dependencies OK')"
```

Auto 成功启动后，再回到主程序重试失败批次。已失败的任务不会因为安装了 Python 自动继续。以上依赖验证只检查模块导入，不执行登录或支付任务。

### Windows 远程桌面有头模式

通过 `mstsc` 登录 Auto 服务器后，在该桌面会话的终端运行 `start.cmd`，并在主程序的 Auto 浏览器设置中选择有头模式。Windows 使用自己的桌面，不需要设置 `DISPLAY` 或安装 Xvfb；Auto 保留所选的有头/无头模式。

浏览器窗口属于启动 Auto 的用户会话。先保持远程桌面连接，确认批次能正常弹出浏览器；不要将需要可见窗口的 Auto 改为 Windows 后台服务运行，服务与交互桌面会话隔离（[Microsoft 说明](https://learn.microsoft.com/en-us/windows/win32/services/interactive-services)）。

若旧版本提示 `sudo apt-get install -y xvfb`，更新代码后先停止旧 Auto，再执行 `start.cmd`；一键启动会自动重新编译已修复的代码。

### 启动日志出现 `exit status 77`

旧版本启动时会额外运行一次 `chrome --headless --dump-dom` 自检；这次启动没有直接使用设置中的许可证，并可能早于后台许可证环境准备。即使选择有头模式，自检仍会使用无头模式，因此它的结果不能代表实际批次。

当前版本启动时只检查浏览器文件和本机依赖，由批次按设置启动浏览器并验证许可证；「文件已就绪」不表示已经验证浏览器运行或许可证有效。更新后停止旧 Auto，重新运行 `start.cmd` 即可。

如果实际批次仍报 `77`，请在主程序的 Auto 浏览器设置中检查并保存有效的 CloakBrowser License。官方定义 `77` 为许可证缺失、无效或过期；浏览器已下载或缓存存在不代表许可证仍然有效（[官方退出码说明](https://github.com/CloakHQ/CloakBrowser/blob/main/cloakbrowser/license.py)）。批次中的真实启动错误会继续保留，不会被启动检查的结果覆盖。

## Linux / macOS

```bash
# 仓库根目录
make ensure-env
make auto
# 或
cd auto
go run .
```

## 连接与运行配置

默认监听所有网卡的 `:9998`。用 Auto 服务器上的 `config.yaml` 的 `auto_addr` 或 `AUTO_ADDR` 修改。独立数据目录默认为 `./auto/data`，可通过 `auto_data_dir` / `AUTO_DATA_DIR` 修改，Auto 不使用主程序的 `data_dir` / `DATA_DIR`。

首次启动会生成连接密钥并在日志显示一次，保存于 Auto 数据目录的 `auto-token` 文件，重新启动保持不变。也可以通过 `auto_token` / `AUTO_TOKEN` 预先指定。

使用 systemd 时，推荐把 `auto_addr`、`auto_data_dir`、`auto_token` 写入该服务器的 `config.yaml`，服务与安装器健康检查会读取相同配置。systemd 不会自动继承运行安装脚本的终端环境；如需使用 `AUTO_ADDR`、`AUTO_DATA_DIR`、`AUTO_TOKEN`，请通过 `systemctl edit clinepass-auto` 在 `[Service]` 的 `Environment=` 中设置并重启服务。已有这类 override 时，再次运行安装脚本需向脚本提供相同环境值，安装脚本不会把终端变量写入服务。

服务模板的浏览器 HOME、缓存及运行目录仍放在仓库 `auto/data` 下。若 `auto_data_dir` 指向仓库之外，还须在服务 override 中把该目录加入 `ReadWritePaths=`，允许 systemd 的文件系统保护规则写入该目录。

在主程序的设置 → Auto 连接中填写 Auto 服务器地址（例如 `https://auto.example.com` 或局域网 `http://192.168.1.20:9998`）和连接密钥，然后测试并保存。公网部署应使用 HTTPS 反向代理传输凭据。浏览器、短信、卡台和 Cookie 续期设置存储在远程 Auto，并可从主程序修改。

所有接口，包括 `/api/health`，都要求 `Authorization: Bearer <密钥>`。健康检查返回 `service: auto` 与 `protocol_version: 1`。浏览器仅访问主程序，由主程序代为调用 Auto。

本目录拥有登录/提取/支付、浏览器下载、虚拟卡预热、短信、Cookie 定时续期、任务日志与 SSE，以及支付链接导出。接通后，主程序可创建批次、提取支付链接、发起自动支付、查看任务及日志、验证订阅并导入成功账号。只有实际支付成功、具有 API Key 和 Cookie 的账号才提供给主程序导入；手动支付后须先验证订阅。

调度/API 使用 Go，默认浏览器执行器仍为 `worker/` 中的 Python Cloak 包装；`LOGIN_ENGINE=go` 可选现有 Go 回退引擎。执行器访问卡台时直接访问 Auto，并携带连接密钥。

从旧共享数据库版本迁移：停止旧 Auto 与主程序并备份旧 `data`，把数据库副本及 `profiles/` 等资料复制到 Auto 服务器的 `auto_data_dir`，主程序保留原数据库。此后两端分别运行，主程序通过导入接收后续成功结果，不再共享 SQLite。

只部署一个 auto 进程。任务队列和任务日志保存在进程内，重启前应等待正在执行的任务完成。部署说明与迁移步骤见根目录 README。
