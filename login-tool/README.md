# ClinePass 付款助手

员工把后台导出的支付链接 Excel 放到本目录，运行程序，按提示一条一条用 Cookie 打开 https://app.cline.bot/dashboard 付款。

## 给员工（Windows）

1. 把 `pay.exe` 和 Excel（`.xlsx`）放在同一个文件夹
2. 双击 `pay.exe`，或在该文件夹打开命令行运行 `pay.exe`
3. 按回车开始。第一次会自动下载 Chrome（大约 150MB），之后会复用 `chrome` 目录
4. 浏览器打开后，在页面里手动付款
5. 付完回到命令行按回车：关掉当前浏览器，打开下一条
6. 输入 `q` 再回车可以中途退出

不要安装 Go、Python 或 Chrome。程序会自己下载 Chrome for Testing。

## 本机打包

在仓库根目录：

```bash
make pay-tool
```

生成：

- `login-tool/dist/pay-linux`（本机测试）
- `login-tool/dist/pay.exe`（发给 Windows 员工）

只列账号、不打开浏览器：

```bash
./login-tool/dist/pay-linux -list
```
