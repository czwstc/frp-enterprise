# HDIT-FRPE-Client（frpc-gui）

用 [MyGo](https://mygo.egoist.dev) Native UI 写的 frpc 桌面客户端，支持 macOS、Windows 和 Linux。内嵌 frpc，不需要单独安装 frpc。

## 功能

- **登录**：填写服务端地址、TOKEN 和鉴权码。鉴权码可以由管理员在 frps 控制台生成，也可以在客户端点“生成”，再提交给管理员审批。审批通过前客户端处于“等待审批”状态并自动重试。
- **概览**：运行状态、服务端看到的公网 IP、本机 IP、设备 ID 和鉴权码。
- **端口映射**：新增、编辑、删除 TCP/UDP 映射。远端端口如果已被其他客户端占用，或不在服务端允许的范围内，无法保存。
- **服务端端口**：服务端上所有已转发的端口，标注是本机的还是其他客户端的。
- **通知**：服务端发来的通知，收到新通知时弹出提示。
- **日志**：内嵌 frpc 的日志，可按级别筛选、复制。
- **安全**：开启加密锁定后，界面只显示运行状态和“重启”按钮，输入密码后才能看到全部内容。
- **外观**：采用 Pulse 风格（左侧带彩色图标的侧栏、白色卡片、点阵曲线），跟随系统的浅色和深色模式。连接后使用服务端“设置”页配置的名称、图标和强调色，并缓存在本机配置中；还没连接过时显示内置的汇鼎标志（文字占位）。

配置保存在系统用户配置目录下的 `frpc-enterprise/config.enc`，用本机绑定的密钥（AES-GCM，由 scrypt 派生）加密，复制到其他机器上无法解密。设置 `FRPC_GUI_CONFIG` 环境变量可以改用其他路径。

## 开发

需要 Go 1.27.3 或更高版本。本目录是独立的 Go 模块，通过 `replace` 引用仓库根目录的 frp 模块。

```bash
make gui-dev     # 开发模式运行，改代码后自动重启
make gui-test    # 不开窗口的视图测试和 mygo vet
make gui         # 打包当前平台
make gui-cross   # 打包 darwin/universal、windows/amd64、linux/amd64
```

## 服务端

frps 需要开启：

```toml
enterprise.enabled = true
enterprise.storePath = "./frps_enterprise.json"

webServer.port = 7500
webServer.user = "admin"
webServer.password = "admin"
```

然后在 frps 控制台的“鉴权码”页生成和审批鉴权码，在“通知”页向客户端发送通知。
