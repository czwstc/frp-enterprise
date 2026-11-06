# Docker 部署 frps 企业版

镜像由 GitHub Actions（`.github/workflows/enterprise-docker.yml`）自动构建并推送到 GitHub Container Registry：

| 镜像 | 说明 |
|---|---|
| `ghcr.io/czwstc/frps-enterprise` | frps：鉴权码审批、控制台账号（本地账号 + OIDC）、审计日志 |
| `ghcr.io/czwstc/frpc-enterprise` | 命令行 frpc（支持 `enterprise.authCode`） |

镜像标签：

- `edge`：默认分支的最新提交。
- `dev`、`main`：对应分支的最新提交。
- `X.Y.Z`、`X.Y`、`latest`：推送 `vX.Y.Z` 标签时生成。
- `sha-xxxxxxx`：每个提交。

平台：`linux/amd64`、`linux/arm64`。

## 部署

```bash
cd deploy/docker
cp .env.example .env    # 填写 token、本地账号密码、OIDC 配置
docker compose up -d
docker compose logs -f frps
```

数据保存在 `frps-data` 卷的 `/data` 目录：

- `frps_enterprise.json`：鉴权码和通知。
- `frps_admin.json`：用户和本地账号的二次验证信息。
- `frps_audit.log`：审计日志。

如果 `.env` 里漏填了密码或 client secret，frps 会拒绝启动并报出缺少的变量。

需要修改配置时，挂载自己的 `frps.toml` 到 `/etc/frp/frps.toml`，见 `docker-compose.yml` 中的注释。

## 首次登录

1. 打开控制台 `http://<服务器>:7500`，用 `.env` 中的本地账号和密码登录。
2. 部署后第一次登录会要求绑定动态验证码：用身份验证应用扫描二维码，输入 6 位验证码。
3. 页面会显示 10 个恢复密钥，只显示这一次，请妥善保存。手机丢失时可以用恢复密钥代替验证码登录，每个只能用一次。
4. 之后登录本地账号需要密码加动态验证码（或恢复密钥）。

## OIDC 登录（例如群晖 SSO）

- 在 OIDC 服务端登记回调地址，与 `FRPS_OIDC_REDIRECT_URL` 保持一致。frps 会在这个地址的路径上处理回调，路径可以任意设置，例如 `/relay`。
- 用户名默认依次取 `preferred_username`、`username`、`email` 中第一个存在的 claim，按用户名（不区分大小写）匹配控制台用户。需要指定 claim 时，在 `frps.toml` 里设置 `enterprise.admin.oidc.usernameClaim`。
- 管理员事先在“用户”页登记的用户名可以直接登录。未登记的用户首次登录后会生成一条申请，由其他管理员在“用户”页批准后才能登录。
- 被停用或删除的用户会立即被登出。

## 生产环境建议

- 控制台（7500）放在 HTTPS 反向代理之后，并把回调地址改为 HTTPS。frps 会根据 `X-Forwarded-Proto` 给会话 cookie 加上 `Secure`。
- 如果 OIDC 服务端使用自签名证书，挂载 CA 证书并设置 `enterprise.admin.oidc.trustedCaFile`。
- GHCR 包默认是私有的。服务器需要先执行 `docker login ghcr.io`，或在仓库的 Packages 设置中把包改为公开。

## 推送到 Docker Hub（可选）

在仓库的 Secrets 中设置 `DOCKERHUB_USERNAME` 和 `DOCKERHUB_TOKEN`，工作流会同时推送 `<用户名>/frps-enterprise` 和 `<用户名>/frpc-enterprise`。
