# CLI 命令手册

`nextunnel` 是 NexTunnel 的统一命令行入口，覆盖服务端安装管理、远端 Control Plane/Dashboard 操作、本机桌面端控制和环境诊断。

## 获取 CLI

来源：

- Linux 一键安装后默认创建 `/usr/local/bin/nextunnel`。
- 服务端包内置 `bin/nextunnel`。
- Release 提供独立 CLI 包，适合运维机、CI 或无需安装服务端的管理终端。

基础命令：

```bash
nextunnel version
nextunnel doctor
nextunnel --help
```

全局输出格式：

```bash
nextunnel server status --output table
nextunnel server status --output json
nextunnel remote node list -o json
```

## 命令树

```text
nextunnel
├── version
├── doctor
├── server
│   ├── install
│   ├── update
│   ├── up
│   ├── down
│   ├── restart
│   ├── status
│   ├── health
│   ├── logs
│   ├── paths
│   └── uninstall
├── config
│   ├── path
│   ├── set-context
│   ├── use-context
│   ├── current-context
│   └── get-contexts
├── remote
│   ├── login
│   ├── health
│   ├── node list|inspect
│   ├── acl list
│   └── alert list|ack
└── desktop
    ├── open
    ├── status
    ├── connect
    ├── disconnect
    ├── settings get|set
    ├── nat detect
    └── network apply|reset
```

## server：服务端管理

`nextunnel server` 会调用发布包内置的一键安装脚本，并复用安装目录中的配置。

```bash
nextunnel server paths
nextunnel server install --version v0.6.4-alpha --non-interactive --public-host example.com
nextunnel server status
nextunnel server health
nextunnel server logs --follow
nextunnel server restart
nextunnel server down
nextunnel server up
nextunnel server update --version v0.6.4-alpha
nextunnel server uninstall --purge
```

常用安装参数：

| 参数 | 说明 |
| --- | --- |
| `--version` | Release 版本，例如 `v0.6.4-alpha` |
| `--package-url` | 服务端发布包 URL、本地路径或 `file://` 路径 |
| `--sha256` | 发布包 SHA256 |
| `--release-base-url` | 自定义 Release 下载基址 |
| `--github-proxy` | 可信自建 GitHub 下载代理 |
| `--repository` | Release 仓库，默认 `Lee-zg/NexTunnel` |
| `--arch` | 服务端包架构：`amd64` 或 `arm64` |
| `--public-host` | 客户端访问的公网 IP 或域名 |
| `--relay-port` | Relay TCP 控制端口 |
| `--relay-quic-port` | Relay QUIC UDP 端口 |
| `--control-plane-port` | Control Plane HTTP 端口 |
| `--dashboard-port` | Dashboard HTTP 端口 |
| `--nat-port` | NAT Detector UDP 端口 |
| `--relay-token` | Relay 认证 token |
| `--control-token` | Control Plane API token |
| `--dashboard-secret` | Dashboard 会话密钥 |
| `--dashboard-admin` | Dashboard 管理员用户名 |
| `--dashboard-password` | Dashboard 管理员密码 |
| `--dashboard-origins` | Dashboard CORS 白名单 |
| `--service-user` | Linux systemd 服务运行用户 |
| `--service-group` | Linux systemd 服务运行用户组 |
| `--service-prefix` | Linux systemd 服务名前缀 |
| `--cli-link` | Linux CLI 软链接路径；`none` 表示跳过 |
| `--dashboard-disabled` | 不启动 Dashboard |
| `--non-interactive` | 非交互执行 |
| `--force` | 强制重新生成配置 |
| `--purge` | 卸载时删除配置和数据 |

同机隔离测试：

```bash
sudo nextunnel server install \
  --package-url /tmp/nextunnel-server-linux-amd64.tar.gz \
  --install-dir /opt/nextunnel-test \
  --config-dir /etc/nextunnel-test \
  --data-dir /var/lib/nextunnel-test \
  --service-prefix nextunnel-test \
  --relay-port 27000 \
  --relay-quic-port 27443 \
  --control-plane-port 29090 \
  --dashboard-port 28080 \
  --nat-port 23478 \
  --non-interactive
```

不要把 systemd 模式安装目录放在 `/tmp`、`/var/tmp` 或 `/dev/shm`。服务启用隔离后，临时目录可能在服务命名空间内不可见。

## config：上下文配置

远端命令依赖当前上下文。Control Plane 和 Dashboard 可以分别配置。

```bash
nextunnel config path

nextunnel config set-context prod \
  --server http://example.com:9090 \
  --token <strong-control-token> \
  --dashboard http://example.com:8080 \
  --dashboard-token <dashboard-token>

nextunnel config use-context prod
nextunnel config current-context
nextunnel config get-contexts
```

CLI 配置默认保存到当前用户目录。配置可能包含 token，写入时会使用当前用户私有权限。

## remote：远端管理

登录 Dashboard 并保存 token：

```bash
nextunnel remote login \
  --dashboard http://example.com:8080 \
  --username admin \
  --password <strong-password> \
  --context prod
```

健康检查：

```bash
nextunnel remote health
nextunnel remote health --target control-plane
nextunnel remote health --target dashboard
```

节点：

```bash
nextunnel remote node list
nextunnel remote node inspect <node_id>
```

ACL：

```bash
nextunnel remote acl list
```

告警：

```bash
nextunnel remote alert list
nextunnel remote alert ack <alert_id>
```

`node` 和 `acl` 命令需要 Control Plane 地址；`alert` 命令需要 Dashboard 地址。缺少地址时，CLI 会提示应该使用的 `config` 或 `remote login` 命令。

## desktop：本机桌面端控制

桌面端运行后会创建本机控制文件，CLI 通过该文件访问本机控制 API。

```bash
nextunnel desktop open
nextunnel desktop status
nextunnel desktop settings get
nextunnel desktop settings set \
  --relay example.com:7000 \
  --relay-token <strong-relay-token> \
  --control-plane http://example.com:9090 \
  --control-token <strong-control-token> \
  --stun example.com:3478

nextunnel desktop connect --relay example.com:7000 --token <strong-relay-token>
nextunnel desktop nat detect
nextunnel desktop network apply
nextunnel desktop network reset
nextunnel desktop disconnect
```

显式指定控制文件：

```bash
nextunnel desktop --control-file /path/to/nextunnel-control.json status
```

启动桌面端时指定可执行文件：

```bash
nextunnel desktop open --binary /path/to/NexTunnel
```

## doctor：环境诊断

```bash
nextunnel doctor
```

`doctor` 用于检查本机基本环境和常见前置条件。结合以下命令可完成大部分排障：

```bash
nextunnel server health
nextunnel server logs --follow
nextunnel remote health
nextunnel desktop status
```

## 自动化建议

CI 或脚本中建议：

- 使用 `--output json`。
- 使用 `--non-interactive`。
- 使用 `--package-url` 指向已上传并校验过的包。
- 配置 `--sha256`。
- 不在命令历史中长期保留真实 token；优先用安全的环境变量或密钥系统注入。

示例：

```bash
nextunnel server install \
  --package-url https://mirror.example.com/nextunnel-server-linux-amd64.tar.gz \
  --sha256 <sha256> \
  --public-host example.com \
  --relay-token "$RELAY_AUTH_TOKEN" \
  --control-token "$CONTROL_PLANE_API_TOKEN" \
  --dashboard-password "$DASHBOARD_ADMIN_PASSWORD" \
  --non-interactive \
  --output json
```
