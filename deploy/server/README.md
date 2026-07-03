# NexTunnel 服务端一键部署

本目录提供服务端生产部署方案，覆盖 Relay Server、Control Plane、NAT Detector 和 Dashboard。生产部署默认使用 GitHub Release 发布包，不在服务器上拉源码构建；脚本会下载服务端包、生成运行配置、安装二进制并启动服务。

## Release 包约定

服务端 Release 包默认从项目仓库发布页下载：

```text
https://github.com/Lee-zg/NexTunnel/releases/latest/download/nextunnel-server-linux-amd64.tar.gz
https://github.com/Lee-zg/NexTunnel/releases/latest/download/nextunnel-server-linux-arm64.tar.gz
https://github.com/Lee-zg/NexTunnel/releases/latest/download/nextunnel-server-windows-amd64.zip
```

包内至少包含：

```text
nextunnel-server/
  bin/relay-server
  bin/control-plane
  bin/nat-detector
  bin/dashboard
  bin/nextunnel
  web/dashboard/
  deploy/server/install.sh
  deploy/server/install.ps1
  .env.example
  README.md
```

`.github/workflows/release.yml` 会在推送 `v*` tag 时生成服务端包、独立 CLI 包、桌面端包、独立 `install.sh` / `install.ps1` 和对应 `.sha256` 文件。服务端包内置 `bin/nextunnel`，安装后可使用统一 CLI 管理服务。

## 统一 CLI

当前 `v0.6.4-alpha` Release 同时发布独立 CLI 包：

```text
nextunnel-cli-v0.6.4-alpha-linux-amd64.tar.gz
nextunnel-cli-v0.6.4-alpha-linux-arm64.tar.gz
nextunnel-cli-v0.6.4-alpha-windows-amd64.zip
```

常用命令：

```bash
nextunnel server status
nextunnel server health
nextunnel server restart
nextunnel server logs --follow
nextunnel config set-context prod --server http://127.0.0.1:9090 --token <control-token> --dashboard http://127.0.0.1:8080 --dashboard-token <dashboard-token>
nextunnel remote node list
nextunnel remote acl list
nextunnel doctor
```

桌面端启动后会创建仅当前用户可读的本机控制文件，CLI 可通过本机控制 API 操作运行中的桌面端：

```powershell
nextunnel desktop status
nextunnel desktop connect --relay 127.0.0.1:7000 --token <relay-token>
nextunnel desktop nat detect
nextunnel desktop network apply
nextunnel desktop disconnect
```

## 方式一：Linux 一键部署（推荐）

公开仓库可直接从 tag 源码下载 Linux 脚本。该地址保持 LF 换行，适合在 Linux 服务器直接执行：

```bash
curl -fL -o /tmp/nextunnel-install.sh \
  https://raw.githubusercontent.com/Lee-zg/NexTunnel/v0.6.4-alpha/deploy/server/install.sh
chmod +x /tmp/nextunnel-install.sh
sudo /tmp/nextunnel-install.sh install --version v0.6.4-alpha
```

```bash
cd deploy/server
chmod +x install.sh
sudo ./install.sh install
```

非交互部署：

```bash
cd deploy/server
sudo NON_INTERACTIVE=true \
  NEXTUNNEL_PUBLIC_HOST=your-domain.example \
  RELAY_AUTH_TOKEN='replace-with-strong-token' \
  CONTROL_PLANE_API_TOKEN='replace-with-strong-token' \
  DASHBOARD_ADMIN_PASSWORD='replace-with-strong-password' \
  ./install.sh install
```

指定版本或源地址：

```bash
sudo ./install.sh install --version v0.6.4-alpha
sudo ./install.sh install --package-url https://mirror.example.com/nextunnel-server-linux-amd64.tar.gz
sudo ./install.sh install --package-url /tmp/nextunnel-server-linux-amd64.tar.gz
sudo ./install.sh install --package-url file:///tmp/nextunnel-server-linux-amd64.tar.gz --sha256 <sha256>
```

手动上传包部署时，建议同时指定公网 IP 或域名，否则默认 `127.0.0.1` 只适合本机测试，外部客户端无法连接：

```bash
sudo /tmp/nextunnel-install.sh install \
  --package-url /tmp/nextunnel-server-linux-amd64.tar.gz \
  --public-host <腾讯云公网IP或域名>
```

腾讯云等国内服务器访问 GitHub Release 慢时，推荐把 Release 资产同步到腾讯云 COS/CDN 后指定下载基址：

```bash
sudo NEXTUNNEL_RELEASE_BASE_URL=https://cos.example.com/nextunnel/v0.6.4-alpha \
  ./install.sh install --version v0.6.4-alpha --sha256 <sha256>
```

如果只是临时加速 GitHub 下载，可以使用可信的自建代理。`--github-proxy` 只会改写脚本自动生成的 GitHub Release 下载地址；显式传入 `--package-url` 或 `--release-base-url` 时不会再使用代理：

```bash
sudo ./install.sh install --version v0.6.4-alpha \
  --github-proxy https://your-proxy.example.com/

sudo ./install.sh install --version v0.6.4-alpha \
  --github-proxy 'https://your-proxy.example.com/?url={url}'
```

常用管理命令：

```bash
sudo ./install.sh status
sudo ./install.sh logs
sudo ./install.sh health
sudo ./install.sh restart
sudo ./install.sh update --version v0.6.4-alpha
sudo ./install.sh down
sudo ./install.sh uninstall
sudo ./install.sh uninstall --purge
```

Linux 默认路径：

| 用途 | 路径 |
|:---|:---|
| 二进制 | `/opt/nextunnel/bin` |
| CLI 命令 | `/usr/local/bin/nextunnel`，可通过 `NEXTUNNEL_CLI_LINK_PATH` 或 `--cli-link` 修改 |
| 配置 | `/etc/nextunnel/server.env` |
| 数据 | `/var/lib/nextunnel/control-plane.db` |
| Dashboard 数据 | `/var/lib/nextunnel/dashboard.db` |
| Dashboard 静态资源 | `/opt/nextunnel/web/dashboard` |
| 服务 | `nextunnel-relay.service`、`nextunnel-control-plane.service`、`nextunnel-nat-detector.service`、`nextunnel-dashboard.service` |
| 日志 | `journalctl -u nextunnel-relay.service -u nextunnel-control-plane.service -u nextunnel-nat-detector.service -u nextunnel-dashboard.service` |

安装完成后，服务端包内置脚本会写入 `/opt/nextunnel/deploy/server/.env`，后续管理不需要重复传入安装参数：

```bash
sudo /opt/nextunnel/deploy/server/install.sh status
sudo /opt/nextunnel/deploy/server/install.sh health
sudo /opt/nextunnel/deploy/server/install.sh up
sudo /opt/nextunnel/deploy/server/install.sh logs --no-log-follow --log-lines 80
```

`up` 和 `restart` 会在启动 systemd 服务后等待服务进入 active 状态，并自动执行健康检查；如果端口被占用或 HTTP 健康端点不可用，脚本会输出端口占用和最近服务日志。

同一台机器需要保留多套测试安装时，可使用独立服务前缀和端口，避免覆盖默认生产服务：

```bash
sudo ./install.sh install \
  --package-url /tmp/nextunnel-server-linux-amd64.tar.gz \
  --install-dir /opt/nextunnel-test \
  --config-dir /etc/nextunnel-test \
  --data-dir /var/lib/nextunnel-test \
  --service-prefix nextunnel-test \
  --relay-port 27000 \
  --relay-quic-port 27443 \
  --control-plane-port 29090 \
  --dashboard-port 28080 \
  --nat-port 23478
```

不要把 systemd 模式安装目录放在 `/tmp`、`/var/tmp` 或 `/dev/shm` 下；服务启用了隔离和自动重启，临时目录会导致二进制或数据目录在服务命名空间中不可见。测试环境也建议使用 `/opt/nextunnel-test`、`/etc/nextunnel-test`、`/var/lib/nextunnel-test` 这类持久路径。

## 方式二：Windows / PowerShell 部署

PowerShell 版本用于 Windows Server 或本地验证，默认下载 `nextunnel-server-windows-amd64.zip`，解压到 `ProgramData\NexTunnel\server`，通过 PID 文件管理三个进程。

```powershell
Set-Location .\deploy\server
.\install.ps1 -Action install
```

非交互部署：

```powershell
$env:NEXTUNNEL_PUBLIC_HOST = "your-domain.example"
$env:RELAY_AUTH_TOKEN = "replace-with-strong-token"
$env:CONTROL_PLANE_API_TOKEN = "replace-with-strong-token"
$env:DASHBOARD_ADMIN_PASSWORD = "replace-with-strong-password"
.\install.ps1 -Action install -NonInteractive
```

指定源地址：

```powershell
.\install.ps1 -Action install -Version v0.6.4-alpha
.\install.ps1 -Action install -PackageUrl "https://mirror.example.com/nextunnel-server-windows-amd64.zip"
.\install.ps1 -Action install -PackageUrl "C:\Temp\nextunnel-server-windows-amd64.zip" -PackageSha256 "<sha256>"
.\install.ps1 -Action install -Version v0.6.4-alpha -ReleaseBaseUrl "https://cos.example.com/nextunnel/v0.6.4-alpha" -PackageSha256 "<sha256>"
.\install.ps1 -Action install -Version v0.6.4-alpha -GithubProxy "https://your-proxy.example.com/"
```

常用管理命令：

```powershell
.\install.ps1 -Action status
.\install.ps1 -Action logs
.\install.ps1 -Action health
.\install.ps1 -Action restart
.\install.ps1 -Action update -Version v0.6.4-alpha
.\install.ps1 -Action down
.\install.ps1 -Action uninstall
```

## 方式三：配置文件部署

可复制 `.env.example` 为 `.env` 后编辑，再执行脚本：

```bash
cd deploy/server
cp .env.example .env
vim .env
sudo ./install.sh install
```

配置优先级：

```text
命令行选项 > 环境变量 > deploy/server/.env > 交互输入/默认值
```

关键变量：

| 变量 | 说明 |
|:---|:---|
| `NEXTUNNEL_REPOSITORY` | GitHub Release 仓库，默认 `Lee-zg/NexTunnel` |
| `NEXTUNNEL_VERSION` | Release 版本，默认 `latest` |
| `NEXTUNNEL_RELEASE_BASE_URL` | 自定义 Release 下载基址 |
| `NEXTUNNEL_GITHUB_PROXY` | 可选 GitHub 下载代理，仅改写脚本自动生成的 GitHub Release URL |
| `NEXTUNNEL_PACKAGE_URL` | 完整服务端包地址，优先级最高 |
| `NEXTUNNEL_PACKAGE_SHA256` | 可选，服务端包 SHA256 校验值 |
| `NEXTUNNEL_SERVICE_PREFIX` | Linux systemd 服务名前缀，默认 `nextunnel` |
| `NEXTUNNEL_CLI_LINK_PATH` | Linux `nextunnel` CLI 系统软链接路径，设置 `none` 可跳过 |
| `NEXTUNNEL_LOG_FOLLOW` | `logs` 是否持续跟随，默认 `true` |
| `NEXTUNNEL_LOG_LINES` | `logs` 默认历史行数，默认 `200` |
| `NEXTUNNEL_SERVICE_START_TIMEOUT_SECONDS` | 等待服务 active 和健康端点可用的超时时间，默认 `30` |
| `NEXTUNNEL_PUBLIC_HOST` | 客户端访问的公网 IP 或域名 |
| `RELAY_AUTH_TOKEN` | Relay 客户端共享认证令牌 |
| `RELAY_ADMIN_LISTEN` | Relay 管理 API 监听地址，systemd 默认 `127.0.0.1:7001` |
| `RELAY_ADMIN_TOKEN` | Relay 管理 API Bearer Token，用于 Dashboard 客户端监控 |
| `CONTROL_PLANE_API_TOKEN` | Control Plane Bearer Token |
| `DASHBOARD_ENABLED` | 是否启动 Dashboard，默认 `true` |
| `DASHBOARD_PORT` | Dashboard HTTP 端口，默认 `8080` |
| `DASHBOARD_SECRET_KEY` | Dashboard token 签名密钥 |
| `DASHBOARD_ADMIN_USER` | Dashboard 默认管理员用户名 |
| `DASHBOARD_ADMIN_PASSWORD` | Dashboard 默认管理员密码，首次初始化必需 |
| `DASHBOARD_RELAY_ADMIN_URL` | Dashboard 访问 Relay 管理 API 的内网/本机地址，默认 `http://127.0.0.1:7001` |
| `DASHBOARD_RELAY_ADMIN_TOKEN` | Dashboard 访问 Relay 管理 API 的 Bearer Token，缺省复用 `RELAY_ADMIN_TOKEN` |

## 方式四：1Panel 部署配置

1Panel 官方文档显示，`主机 - 终端` 可直接连接本地服务器并执行命令；`计划任务` 支持 Shell 脚本和手动执行；`容器 - 编排` 支持 Web 编辑器、路径选择和编排模版三种 Compose 创建方式。

参考：

- [1Panel 终端文档](https://1panel.cn/docs/v1/user_manual/hosts/terminal/)
- [1Panel 计划任务文档](https://1panel.cn/docs/v1/user_manual/cronjobs/)
- [1Panel 容器编排文档](https://1panel.cn/docs/v1/user_manual/containers/compose/)

### 1Panel 方式 A：终端执行一键脚本（推荐）

1. 在 1Panel 打开 `主机 - 终端`，连接当前服务器。
2. 准备脚本目录，例如上传仓库中的 `deploy/server/install.sh` 到 `/opt/nextunnel-deploy/install.sh`。
3. 执行：

```bash
cd /opt/nextunnel-deploy
chmod +x install.sh
sudo NON_INTERACTIVE=true \
  NEXTUNNEL_PUBLIC_HOST=your-domain.example \
  RELAY_AUTH_TOKEN='replace-with-strong-token' \
  CONTROL_PLANE_API_TOKEN='replace-with-strong-token' \
  DASHBOARD_ADMIN_PASSWORD='replace-with-strong-password' \
  ./install.sh install --version v0.6.4-alpha
```

如果使用自建源：

```bash
sudo ./install.sh install --package-url https://mirror.example.com/nextunnel-server-linux-amd64.tar.gz --sha256 <sha256>
```

### 1Panel 方式 B：文件上传离线包

1. 在 1Panel `主机 - 文件` 上传服务端包到 `/opt/packages/nextunnel-server-linux-amd64.tar.gz`。
2. 在 `主机 - 终端` 执行：

```bash
cd /opt/nextunnel-deploy
sudo NON_INTERACTIVE=true \
  NEXTUNNEL_PUBLIC_HOST=your-domain.example \
  RELAY_AUTH_TOKEN='replace-with-strong-token' \
  CONTROL_PLANE_API_TOKEN='replace-with-strong-token' \
  DASHBOARD_ADMIN_PASSWORD='replace-with-strong-password' \
  ./install.sh install --package-url /opt/packages/nextunnel-server-linux-amd64.tar.gz
```

安装完成后如果提示 `nextunnel: command not found`，说明当前使用的是旧安装脚本或 CLI 链接未创建。可先直接执行 `/opt/nextunnel/bin/nextunnel server status`，再补建软链接：

```bash
sudo ln -sfn /opt/nextunnel/bin/nextunnel /usr/local/bin/nextunnel
nextunnel server status
```

如果服务无法连接，优先检查：

```bash
sudo /opt/nextunnel/deploy/server/install.sh health
sudo /opt/nextunnel/deploy/server/install.sh logs
sudo systemctl status nextunnel-relay.service nextunnel-control-plane.service nextunnel-nat-detector.service nextunnel-dashboard.service
```

外部客户端连接还需要确认腾讯云安全组和系统防火墙已放行 `7000/tcp`、`7443/udp`、`3478/udp`；Dashboard 需要 `8080/tcp`。

### 1Panel 方式 C：计划任务升级

在 `计划任务` 创建 Shell 脚本任务，可手动执行或定时执行升级命令：

```bash
cd /opt/nextunnel-deploy
sudo ./install.sh update --version v0.6.4-alpha
sudo ./install.sh health
```

建议不要把 token 写入计划任务脚本；首次安装后 token 已保存在 `/etc/nextunnel/server.env`，升级只需要指定版本或包地址。

### 1Panel 方式 D：容器编排模板

`docker-compose.yml` 不再执行源码构建，仅作为后续官方镜像发布后的 1Panel Compose 模板。发布镜像后可在 1Panel `容器 - 编排` 中选择 Web 编辑或路径选择部署：

```bash
cp deploy/server/.env.example deploy/server/.env
# 编辑 NEXTUNNEL_IMAGE、NEXTUNNEL_PUBLIC_HOST、RELAY_AUTH_TOKEN、RELAY_ADMIN_TOKEN、DASHBOARD_RELAY_ADMIN_TOKEN、CONTROL_PLANE_API_TOKEN、DASHBOARD_SECRET_KEY、DASHBOARD_ADMIN_PASSWORD
docker compose --env-file deploy/server/.env -f deploy/server/docker-compose.yml up -d
```

当前可连接生产部署仍优先使用 Release 包 + systemd。

## 默认端口

| 服务 | 协议 | 默认端口 | 说明 |
|:---|:---:|:---:|:---|
| Relay Server | TCP | 7000 | 客户端控制连接和工作连接 |
| Relay QUIC | UDP | 7443 | QUIC Relay 传输 |
| Relay Admin | TCP/HTTP | 7001 | Dashboard 客户端监控接口，默认只监听本机或容器内网 |
| Dashboard | TCP/HTTP | 8080 | Web 管理控制台 |
| Control Plane | TCP/HTTP | 9090 | 节点、ACL、密钥 API |
| NAT Detector | UDP | 3478 | STUN/TURN NAT 检测 |

生产服务器需要在防火墙、安全组或 1Panel 防火墙中放行客户端和管理入口端口。`7001/tcp` 是 Relay 管理 API，默认只给 Dashboard 内部访问，不应对公网开放。仅试用 TCP Relay 时，至少放行 `7000/tcp`。

## 连接信息

部署完成后，脚本会输出：

```text
Relay TCP:       <host>:7000
Relay QUIC UDP:  <host>:7443
NAT UDP:         <host>:3478
Control Plane:   http://<host>:9090
Dashboard:       http://<host>:8080
Relay Token:     <generated-token>
Control Token:   <generated-token>
```

桌面端先使用 `Relay TCP` 地址和 `Relay Token` 进行最小连接验证。

## 健康检查

```bash
sudo ./install.sh health
```

健康检查包含：

- `GET /healthz` 验证 Control Plane。
- TCP 端口探测验证 Relay Server。
- `GET /api/v1/admin/health` 验证 Relay 管理 API。
- `GET /api/v1/health` 验证 Dashboard。

NAT Detector 使用 UDP 服务，通常通过客户端 STUN 检测流程验证。

## 生产验收

服务健康检查通过后，建议回到仓库根目录执行 `docs/deploy/production-verification.md` 中的验收命令，覆盖 Dashboard HTTPS/CORS/鉴权、真实 TUN/路由、Linux eBPF XDP 和多地域 Edge/Anycast 演练。验证报告默认写入 `dist/verification/`，用于发布前归档。

## 安全建议

- 生产环境必须替换 `.env.example` 中的默认 token。
- 建议使用 `NEXTUNNEL_PACKAGE_SHA256` 或 `--sha256` 校验离线包和镜像源下载包。
- 建议将 Control Plane 放在反向代理、VPN 或内网访问范围后面。
- 建议将 Dashboard 放在 HTTPS 反向代理后面，并设置强密码和最小 CORS 白名单。
- Relay 管理 API 只应监听本机或容器内网，并使用强随机 `RELAY_ADMIN_TOKEN` / `DASHBOARD_RELAY_ADMIN_TOKEN`。
- 建议使用系统防火墙、云安全组或 1Panel 防火墙只开放必要端口。
