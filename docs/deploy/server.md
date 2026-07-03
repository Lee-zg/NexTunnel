# 服务端部署

服务端包含 Relay、Control Plane、NAT Detector 和 Dashboard。推荐生产部署使用 Release 二进制包和一键脚本；Docker Compose 适合试用、1Panel 或已有容器平台。

## 部署方式选择

| 方式 | 适用场景 |
| --- | --- |
| `install.sh` | Linux 生产部署，systemd 管理，推荐 |
| `install.ps1` | Windows 本机服务端部署或测试 |
| `deploy/server/docker-compose.yml` | 容器平台、1Panel、手工 Compose |
| 根目录 `docker-compose.yml` | 本地源码构建试用 |

## Linux install.sh

下载安装脚本：

```bash
curl -fL -o /tmp/nextunnel-install.sh \
  https://github.com/Lee-zg/NexTunnel/releases/download/v0.6.4-alpha/install.sh
chmod +x /tmp/nextunnel-install.sh
```

安装：

```bash
sudo /tmp/nextunnel-install.sh install \
  --version v0.6.4-alpha \
  --public-host example.com \
  --relay-token <strong-relay-token> \
  --control-token <strong-control-token> \
  --dashboard-password <strong-password> \
  --non-interactive
```

本地包安装：

```bash
sudo ./install.sh install \
  --package-url /tmp/nextunnel-server-linux-amd64.tar.gz \
  --sha256 <sha256> \
  --public-host example.com
```

常用操作：

```bash
sudo /opt/nextunnel/deploy/server/install.sh status
sudo /opt/nextunnel/deploy/server/install.sh health
sudo /opt/nextunnel/deploy/server/install.sh logs --no-log-follow --log-lines 80
sudo /opt/nextunnel/deploy/server/install.sh restart
sudo /opt/nextunnel/deploy/server/install.sh update --version v0.6.4-alpha
sudo /opt/nextunnel/deploy/server/install.sh down
sudo /opt/nextunnel/deploy/server/install.sh uninstall
sudo /opt/nextunnel/deploy/server/install.sh uninstall --purge
```

默认路径：

| 路径 | 说明 |
| --- | --- |
| `/opt/nextunnel` | 安装目录 |
| `/etc/nextunnel/server.env` | 运行配置 |
| `/var/lib/nextunnel` | 数据目录 |
| `/opt/nextunnel/bin` | 二进制目录 |
| `/opt/nextunnel/web/dashboard` | Dashboard 静态资源 |
| `/usr/local/bin/nextunnel` | CLI 软链接 |

## Windows install.ps1

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

参数：

| 参数 | 说明 |
| --- | --- |
| `-Action` | `install`、`up`、`down`、`restart`、`status`、`logs`、`health`、`update`、`uninstall`、`config` |
| `-PackageUrl` | 服务端包 URL 或本地路径 |
| `-Version` | Release 版本 |
| `-ReleaseBaseUrl` | 自定义 Release 下载基址 |
| `-GithubProxy` | GitHub 下载代理 |
| `-PackageSha256` | 服务端包 SHA256 |
| `-Architecture` | 架构 |
| `-InstallDir` / `-ConfigDir` / `-DataDir` | 自定义目录 |
| `-PublicHost` | 对外访问地址 |
| `-RelayToken` / `-RelayAdminToken` | Relay token |
| `-ControlToken` | Control Plane token |
| `-DashboardSecret` / `-DashboardPassword` | Dashboard 安全参数 |
| `-DashboardDisabled` | 不启动 Dashboard |
| `-NonInteractive` | 非交互安装 |
| `-Force` | 强制生成配置 |
| `-Purge` | 卸载时清理配置和数据 |

## deploy/server/.env

脚本读取优先级：

```text
命令行参数 > 环境变量 > deploy/server/.env > 交互输入/默认值
```

最小生产示例：

```dotenv
NEXTUNNEL_VERSION=v0.6.4-alpha
NEXTUNNEL_PUBLIC_HOST=example.com

RELAY_CONTROL_PORT=7000
RELAY_QUIC_PORT=7443
RELAY_AUTH_TOKEN=<strong-relay-token>
RELAY_ADMIN_LISTEN=127.0.0.1:7001
RELAY_ADMIN_TOKEN=<strong-relay-admin-token>

CONTROL_PLANE_PORT=9090
CONTROL_PLANE_API_TOKEN=<strong-control-token>

DASHBOARD_ENABLED=true
DASHBOARD_PORT=8080
DASHBOARD_SECRET_KEY=<strong-dashboard-secret>
DASHBOARD_ADMIN_USER=admin
DASHBOARD_ADMIN_PASSWORD=<strong-password>
DASHBOARD_ALLOWED_ORIGINS=https://dashboard.example.com
DASHBOARD_RELAY_ADMIN_URL=http://127.0.0.1:7001
DASHBOARD_RELAY_ADMIN_TOKEN=<strong-relay-admin-token>

NAT_PORT=3478
NAT_REALM=nextunnel.local
```

完整模板见 `deploy/server/.env.example`。

## Docker Compose

```bash
cp deploy/server/.env.example deploy/server/.env
docker compose -f deploy/server/docker-compose.yml --env-file deploy/server/.env up -d
```

注意：

- Compose 模板要求配置强 token。
- Relay Admin API 在容器网络内访问，不映射到宿主公网端口。
- `NEXTUNNEL_DATA_DIR` 默认挂载到数据目录。
- 生产 HTTPS 建议由宿主反向代理提供。

## 端口和防火墙

| 服务 | 协议 | 默认端口 | 是否建议公网开放 |
| --- | --- | --- | --- |
| Relay TCP | TCP | `7000` | 是 |
| Relay QUIC | UDP | `7443` | 按需 |
| Relay Admin | HTTP/TCP | `7001` | 否，仅本机/内网 |
| Control Plane | HTTP/TCP | `9090` | 限制来源或放在内网 |
| Dashboard | HTTP/TCP | `8080` | 仅通过 HTTPS 反代开放 |
| NAT Detector | UDP | `3478` | 按需 |

## HTTPS 反向代理

Dashboard 生产建议通过 HTTPS 访问。Nginx 示例：

```nginx
server {
    listen 443 ssl http2;
    server_name dashboard.example.com;

    ssl_certificate     /etc/letsencrypt/live/dashboard.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/dashboard.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

同步设置：

```dotenv
DASHBOARD_ALLOWED_ORIGINS=https://dashboard.example.com
```

如果没有可用域名或证书，用 `scripts/verify-dashboard-ssh.ps1` 通过 SSH 隧道验收 API，不要把管理员密码发送到公网 HTTP。

## mTLS

Control Plane 和 Relay 支持 mTLS 参数：

```bash
control-plane \
  -listen 0.0.0.0:9090 \
  -tls-ca /path/to/ca.pem \
  -tls-cert /path/to/server.pem \
  -tls-key /path/to/server-key.pem

relay \
  -bind 0.0.0.0 \
  -control-port 7000 \
  -tls-ca /path/to/ca.pem \
  -tls-cert /path/to/server.pem \
  -tls-key /path/to/server-key.pem
```

## 国内服务器下载加速

推荐把 Release 资产同步到 COS/CDN：

```bash
sudo ./install.sh install \
  --version v0.6.4-alpha \
  --release-base-url https://cos.example.com/nextunnel/v0.6.4-alpha \
  --sha256 <sha256>
```

或使用本地包：

```bash
sudo ./install.sh install \
  --package-url /opt/packages/nextunnel-server-linux-amd64.tar.gz \
  --sha256 <sha256>
```

临时代理：

```bash
sudo ./install.sh install \
  --version v0.6.4-alpha \
  --github-proxy https://your-proxy.example.com/
```

生产环境下载远端资产时建议总是配置 SHA256。

## 升级和回滚

升级：

```bash
sudo /opt/nextunnel/deploy/server/install.sh update --version v0.6.4-alpha
sudo /opt/nextunnel/deploy/server/install.sh health
```

回滚到已保存的旧包时，使用旧版本 `--package-url` 重新安装或执行 `update`。

卸载：

```bash
sudo /opt/nextunnel/deploy/server/install.sh uninstall
sudo /opt/nextunnel/deploy/server/install.sh uninstall --purge
```

`--purge` 会删除安装目录、配置和数据，执行前请备份 SQLite 数据和审计日志。

## 生产验收

服务健康检查通过后，继续执行 [生产验证手册](./production-verification.md)：

```bash
sudo /opt/nextunnel/deploy/server/install.sh health
DASHBOARD_URL=https://dashboard.example.com DASHBOARD_PASSWORD=<password> make verify-dashboard
```

真实 TUN、eBPF 和路由验证会修改系统网络状态，只能在授权节点执行。
