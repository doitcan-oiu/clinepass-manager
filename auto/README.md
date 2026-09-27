# ClinePass Auto

独立 Go 自动化服务，可部署在与主程序不同的服务器。两端通过带密钥的 HTTP API 通信，各自拥有 SQLite 数据库，不共享目录或数据库。

```bash
# 仓库根目录
make ensure-env
make auto
# 或
cd auto
go run .
```

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
