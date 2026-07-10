# Dashboard 运维手册

Dashboard 是 NexTunnel 服务端的 Web 管理台，覆盖节点、客户端、流量、ACL、告警、审计、用户和运行配置状态。

## 启动参数

```bash
dashboard \
  -listen 127.0.0.1:8080 \
  -secret-key <strong-dashboard-secret> \
  -admin-user admin \
  -admin-password <strong-password> \
  -store-path /var/lib/nextunnel/dashboard.db \
  -static-dir /opt/nextunnel/web/dashboard \
  -allowed-origins https://dashboard.example.com \
  -relay-admin-url http://127.0.0.1:7001 \
  -relay-admin-token <strong-relay-admin-token> \
  -audit-log /var/log/nextunnel/dashboard-audit.jsonl
```

常用参数：

| 参数 | 说明 |
| --- | --- |
| `-listen` | Dashboard HTTP/HTTPS 监听地址 |
| `-allowed-origins` | CORS 白名单，逗号分隔 |
| `-secret-key` | token 签名密钥 |
| `-admin-user` | 默认管理员用户名 |
| `-admin-password` | 初始化管理员密码 |
| `-token-expiry` | token 有效期 |
| `-store-path` | SQLite 数据库路径；为空则内存存储 |
| `-static-dir` | Dashboard 前端静态资源目录 |
| `-tls-cert` / `-tls-key` | 直接启用 HTTPS 的证书和私钥 |
| `-audit-log` | JSON Lines 审计日志路径 |
| `-relay-admin-url` | Relay Admin API 地址 |
| `-relay-admin-token` | Relay Admin API Bearer Token |

生产环境建议由 Nginx/OpenResty/Caddy 终止 HTTPS，再反代到 `127.0.0.1:8080`。

## 登录

浏览器访问：

```text
https://dashboard.example.com
```

默认管理员来自启动参数或安装脚本：

```text
用户名：admin
密码：安装时的 DASHBOARD_ADMIN_PASSWORD
```

登录接口：

```bash
curl -X POST https://dashboard.example.com/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"<strong-password>"}'
```

## RBAC 权限

角色：

| 角色 | 说明 |
| --- | --- |
| `admin` | 完整管理权限，含用户和审计 |
| `operator` | 运维权限，可管理节点、客户端、ACL、告警 |
| `viewer` | 只读权限，不能修改或断开客户端 |

权限矩阵：

| 资源 | admin | operator | viewer |
| --- | :---: | :---: | :---: |
| 节点 | 读写删 | 读写删 | 只读 |
| 客户端 | 查看/断开 | 查看/断开 | 只读 |
| Public Endpoint | 只读 | 只读 | 只读 |
| Endpoint Policy | 读写删 | 读写删 | 只读 |
| HTTP 请求日志 | 只读 | 只读 | 只读 |
| ACL | 读写删 | 读写删 | 只读 |
| 告警 | 读写删 | 读写 | 只读 |
| 告警规则 | 读写删 | 只读 | 只读 |
| 用户 | 读写删 | 无 | 无 |
| 审计 | 只读 | 无 | 无 |
| 配置状态 | 只读 | 只读 | 只读 |

## 页面说明

| 页面 | 功能 |
| --- | --- |
| 总览 | 服务状态、节点概览、告警概览 |
| 节点 | Control Plane 节点列表和详情 |
| 客户端 | Relay 在线客户端、代理列表、断开客户端 |
| Public Endpoint | 公开 HTTP Endpoint、访问策略和请求日志 |
| 流量 | 节点和客户端流量条形图 |
| ACL | 访问控制规则列表、新增、删除 |
| 告警 | 告警列表、确认告警 |
| 审计 | 按用户、资源、动作、结果和时间过滤 |
| 设置 | HTTPS、Relay Admin API、CORS、审计、存储路径和版本状态 |

## 客户端监控

Dashboard 通过 Relay Admin API 获取在线客户端：

```dotenv
RELAY_ADMIN_LISTEN=127.0.0.1:7001
RELAY_ADMIN_TOKEN=<strong-relay-admin-token>
DASHBOARD_RELAY_ADMIN_URL=http://127.0.0.1:7001
DASHBOARD_RELAY_ADMIN_TOKEN=<strong-relay-admin-token>
```

API：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/v1/clients` | Dashboard 聚合后的客户端列表 |
| `DELETE` | `/api/v1/clients/{id}` | 通过 Relay Admin API 断开客户端 |

Relay Admin 不可用时，客户端页会显示配置或连接错误。检查：

```bash
curl -H "Authorization: Bearer <strong-relay-admin-token>" \
  http://127.0.0.1:7001/api/v1/admin/health
```

## Public Endpoint

Public Endpoint 由 Relay 的 HTTP Gateway 按 Host 头路由到桌面端 HTTP 隧道。TCP 隧道仍使用远端端口监听，不受 Public Gateway 影响。

Relay 示例：

```bash
relay \
  -bind 0.0.0.0 \
  -control-port 7000 \
  -auth-token <strong-relay-token> \
  -require-auth \
  -admin-listen 127.0.0.1:7001 \
  -admin-token <strong-relay-admin-token> \
  -public-http-listen 0.0.0.0:80 \
  -domain-suffix apps.example.com \
  -tls-mode off
```

Endpoint API：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/v1/endpoints` | 查看当前公开 Endpoint |
| `GET` | `/api/v1/endpoint-policies` | 查看 Endpoint 访问策略 |
| `POST` | `/api/v1/endpoint-policies` | 新增或更新策略 |
| `DELETE` | `/api/v1/endpoint-policies/{id}` | 删除策略 |
| `GET` | `/api/v1/http-requests?limit=100` | 查看请求日志 |

策略字段：

| 字段 | 说明 |
| --- | --- |
| `auth_mode` | `none`、`basic_auth`、`bearer_token` |
| `basic_username` / `basic_password` | Basic Auth 凭据；密码只写入，不在列表响应中回显 |
| `bearer_token` | Bearer Token；只写入，不在列表响应中回显 |
| `allowed_ips` / `denied_ips` | 单 IP 或 CIDR |
| `not_before` / `not_after` | 策略生效时间窗 |
| `rate_limit_per_minute` | 每 IP 每分钟请求上限 |
| `max_concurrent` | 每 IP 最大并发请求数 |

创建 Basic Auth 策略示例：

```bash
curl -X POST https://dashboard.example.com/api/v1/endpoint-policies \
  -H "Authorization: Bearer <dashboard-token>" \
  -H "Content-Type: application/json" \
  -d '{"id":"basic-dev","name":"Dev Basic","auth_mode":"basic_auth","basic_username":"dev","basic_password":"<strong-password>","rate_limit_per_minute":60}'
```

通过 CLI 发布本机服务：

```bash
nextunnel desktop publish \
  --http 3000 \
  --subdomain demo \
  --auth basic \
  --access-policy-id basic-dev
```

请求日志默认记录时间、Host、Path、方法、状态码、延迟、上下行字节、客户端 IP、策略结果和隧道名；当前不保存请求 body。

## ACL 管理

ACL 规则包含：

| 字段 | 说明 |
| --- | --- |
| `id` | 规则 ID |
| `source` | 来源节点或通配 |
| `target` | 目标节点或通配 |
| `action` | `allow` / `deny` |
| `protocol` | 协议 |
| `priority` | 优先级 |
| `enabled` | 是否启用 |

示例：

```bash
curl -X POST https://dashboard.example.com/api/v1/acl \
  -H "Authorization: Bearer <dashboard-token>" \
  -H "Content-Type: application/json" \
  -d '{"id":"allow-web","source":"*","target":"web-node","action":"allow","protocol":"tcp","priority":100,"enabled":true}'
```

## 告警

Dashboard 支持告警列表和确认：

```bash
curl -H "Authorization: Bearer <dashboard-token>" \
  https://dashboard.example.com/api/v1/alerts

curl -X POST \
  -H "Authorization: Bearer <dashboard-token>" \
  https://dashboard.example.com/api/v1/alerts/<alert_id>/ack
```

外部系统可以通过 `/api/v1/metrics` 推送指标并触发告警规则。

## 审计日志

启用：

```bash
dashboard -audit-log /var/log/nextunnel/dashboard-audit.jsonl ...
```

查询：

```bash
curl -H "Authorization: Bearer <dashboard-token>" \
  "https://dashboard.example.com/api/v1/audit?resource=users&action=create&result=success&limit=50"
```

过滤条件：

| 参数 | 说明 |
| --- | --- |
| `actor` | 操作用户 |
| `resource` | 资源，例如 `users`、`clients`、`acl` |
| `action` | `create`、`update`、`delete` 等 |
| `result` | `success` / `failure` |
| `start` / `end` | 时间范围 |
| `limit` | 返回数量 |

## 配置状态

接口：

```bash
curl -H "Authorization: Bearer <dashboard-token>" \
  https://dashboard.example.com/api/v1/config/status
```

状态页会显示：

- Dashboard HTTPS 是否启用。
- Relay Admin API 是否配置和可达。
- CORS 白名单。
- 审计日志是否启用、可查询。
- 存储路径。
- 当前版本。

## HTTPS 与 CORS

推荐反向代理：

```nginx
server {
    listen 443 ssl http2;
    server_name dashboard.example.com;

    ssl_certificate     /etc/letsencrypt/live/dashboard.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/dashboard.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

对应配置：

```dotenv
DASHBOARD_ALLOWED_ORIGINS=https://dashboard.example.com
```

## 验证

公网 HTTPS 验证：

```powershell
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/verify-dashboard.ps1 `
  -BaseUrl "https://dashboard.example.com" `
  -Username "admin" `
  -Password "<strong-password>" `
  -AllowedOrigin "https://dashboard.example.com" `
  -ReportPath "dist/verification/dashboard-https-latest.json"
```

无 HTTPS 域名时：

```powershell
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/verify-dashboard-ssh.ps1 `
  -SshHost "server.example.com" `
  -User "root" `
  -IdentityFile "$env:USERPROFILE\.ssh\id_ed25519" `
  -RemoteDashboardPort 8080 `
  -ReportPath "dist/verification/dashboard-ssh-latest.json"
```

SSH 隧道方式避免管理员密码经过公网 HTTP。
