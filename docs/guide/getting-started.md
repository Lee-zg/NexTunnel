# 快速开始

本指南按“部署服务端 -> 配置桌面端 -> 创建隧道 -> 验证访问”的顺序走完最小闭环。命令中的 token 和密码请替换为强随机值。

## 你会得到什么

完成后会有：

- Relay TCP 入口：`example.com:7000`
- Control Plane：`http://example.com:9090`
- Dashboard：`http://example.com:8080`
- 桌面端连接到 Relay
- 一条把远端端口 `13000` 转发到本机 `127.0.0.1:3000` 的 TCP 隧道

## 准备服务器

Linux 服务器需要：

- `amd64` 或 `arm64`
- systemd
- 可访问 GitHub Release，或已手动上传服务端包
- 防火墙或安全组放行 `7000/tcp`

完整能力还建议放行：

| 端口 | 协议 | 用途 |
| --- | --- | --- |
| `7000` | TCP | Relay 控制连接和工作连接 |
| `7443` | UDP | QUIC Relay |
| `8080` | TCP | Dashboard HTTP，生产建议放到 HTTPS 反代后 |
| `9090` | TCP | Control Plane API，生产建议限制来源 |
| `3478` | UDP | NAT Detector / STUN |

不要开放 `7001/tcp` 到公网。它是 Relay Admin API，默认只给 Dashboard 内部访问。

## Linux 一键安装

```bash
curl -fL -o /tmp/nextunnel-install.sh \
  https://github.com/Lee-zg/NexTunnel/releases/download/v0.6.4-alpha/install.sh
chmod +x /tmp/nextunnel-install.sh

sudo /tmp/nextunnel-install.sh install \
  --version v0.6.4-alpha \
  --public-host example.com \
  --relay-token <strong-relay-token> \
  --control-token <strong-control-token> \
  --dashboard-password <strong-password> \
  --non-interactive
```

检查服务：

```bash
sudo /opt/nextunnel/deploy/server/install.sh status
sudo /opt/nextunnel/deploy/server/install.sh health
```

查看日志：

```bash
sudo /opt/nextunnel/deploy/server/install.sh logs --no-log-follow --log-lines 80
```

## Windows 服务端安装

```powershell
Invoke-WebRequest `
  -Uri "https://github.com/Lee-zg/NexTunnel/releases/download/v0.6.4-alpha/install.ps1" `
  -OutFile ".\install.ps1"

.\install.ps1 -Action install `
  -Version v0.6.4-alpha `
  -PublicHost "example.com" `
  -RelayToken "<strong-relay-token>" `
  -ControlToken "<strong-control-token>" `
  -DashboardPassword "<strong-password>" `
  -NonInteractive
```

常用操作：

```powershell
.\install.ps1 -Action status
.\install.ps1 -Action health
.\install.ps1 -Action logs
.\install.ps1 -Action restart
```

## Docker Compose 试用

容器部署适合试用、1Panel 或已有容器平台。生产二进制部署优先使用一键脚本。

```bash
cp deploy/server/.env.example deploy/server/.env
```

编辑 `deploy/server/.env`，至少修改：

```dotenv
RELAY_AUTH_TOKEN=<strong-relay-token>
RELAY_ADMIN_TOKEN=<strong-relay-admin-token>
CONTROL_PLANE_API_TOKEN=<strong-control-token>
DASHBOARD_SECRET_KEY=<strong-dashboard-secret>
DASHBOARD_ADMIN_PASSWORD=<strong-password>
DASHBOARD_RELAY_ADMIN_TOKEN=<strong-relay-admin-token>
```

启动：

```bash
docker compose -f deploy/server/docker-compose.yml --env-file deploy/server/.env up -d
```

## 配置桌面端

1. 打开 NexTunnel 桌面端。
2. 进入“设置 -> 连接”。
3. 新增或编辑服务端实例：

| 字段 | 示例 |
| --- | --- |
| 节点名称 | `生产节点` |
| Relay 地址 | `example.com:7000` |
| Relay Token | `<strong-relay-token>` |
| Control Plane URL | `http://example.com:9090` |
| Control Plane Token | `<strong-control-token>` |
| STUN Server | `example.com:3478` |
| STUN Alt Server | `example.com:3478` |

4. 点击服务端实例检测，确认 Relay、Control Plane、STUN 的状态。
5. 回到总览页点击连接。

## 创建第一条隧道

先在桌面端所在机器启动一个本地服务，例如：

```bash
npx vite --host 127.0.0.1 --port 3000
```

在 NexTunnel 桌面端进入“隧道”，创建：

| 字段 | 值 |
| --- | --- |
| 名称 | `web-3000` |
| 类型 | `tcp` |
| 本地地址 | `127.0.0.1` |
| 本地端口 | `3000` |
| 远端端口 | `13000` |

启动隧道后，从外部访问：

```bash
curl http://example.com:13000
```

如果能看到本地 Web 服务响应，说明最小链路已经跑通。

## 使用 CLI 验证

Linux 一键安装后默认提供 `nextunnel`：

```bash
nextunnel server health
nextunnel server status
```

配置远端上下文：

```bash
nextunnel config set-context prod \
  --server http://example.com:9090 \
  --token <strong-control-token> \
  --dashboard http://example.com:8080

nextunnel remote login \
  --dashboard http://example.com:8080 \
  --username admin \
  --password <strong-password> \
  --context prod

nextunnel config use-context prod
nextunnel remote node list
nextunnel remote health
```

## Dashboard 登录

浏览器打开：

```text
http://example.com:8080
```

默认管理员用户名为 `admin`，密码为安装时传入的 `--dashboard-password`。

生产环境不要长期使用公网 HTTP 暴露 Dashboard。建议配置 HTTPS 反向代理，并把 `DASHBOARD_ALLOWED_ORIGINS` 设置为实际域名。

## Windows TUN 可选验证

虚拟网络路由需要真实 TUN。Windows 上需要：

- 官方、匹配架构的 `wintun.dll`
- 管理员权限
- Control Plane 下发的 `virtual-interface` 与本机适配器名称一致，默认 `nextunnel0`

桌面端网络页会显示 Wintun 状态。缺失 DLL 时可以使用“修复 Wintun”；缺少权限时用管理员身份重启桌面端。

## 国内服务器下载慢

手动上传服务端包后安装：

```bash
sudo ./install.sh install \
  --package-url /tmp/nextunnel-server-linux-amd64.tar.gz \
  --sha256 <sha256> \
  --public-host example.com
```

或使用自建 COS/CDN：

```bash
sudo ./install.sh install \
  --version v0.6.4-alpha \
  --release-base-url https://cos.example.com/nextunnel/v0.6.4-alpha \
  --sha256 <sha256>
```

下一步建议阅读 [服务端部署](../deploy/server.md) 和 [桌面端能力总览](../desktop/overview.md)。
