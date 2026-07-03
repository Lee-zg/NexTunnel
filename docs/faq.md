# FAQ

## NexTunnel 和 FRP/NPS 有什么区别？

NexTunnel 同样支持把内网服务通过中继暴露到远端端口，但它额外提供桌面端可视化、Control Plane、Dashboard、Relay Admin API、NAT/STUN 诊断、真实 TUN 预检和发布前生产验证工具链。

v0.6.4-alpha 的生产可用核心是自部署 Relay/Dashboard、桌面 TCP/HTTP 隧道、客户端监控、服务端安装脚本和诊断闭环。P2P、TUN、eBPF 和 Edge 能力按验证工具链与当前平台条件明确边界。

## 最少需要开放哪些端口？

最小 Relay TCP：

| 端口 | 协议 | 用途 |
| --- | --- | --- |
| `7000` | TCP | Relay 控制连接和工作连接 |

完整部署通常还需要：

| 端口 | 协议 | 用途 |
| --- | --- | --- |
| `7443` | UDP | QUIC Relay |
| `8080` | TCP | Dashboard，建议只通过 HTTPS 反代开放 |
| `9090` | TCP | Control Plane，建议限制来源 |
| `3478` | UDP | NAT Detector / STUN |

不要把 `7001/tcp` Relay Admin API 暴露到公网。

## Relay Token、Control Token、Dashboard Secret 有什么区别？

| 名称 | 用途 |
| --- | --- |
| `RELAY_AUTH_TOKEN` | 桌面客户端连接 Relay 的共享认证 |
| `RELAY_ADMIN_TOKEN` | Dashboard 调 Relay Admin API 的 Bearer Token |
| `CONTROL_PLANE_API_TOKEN` | Control Plane API Bearer Token |
| `DASHBOARD_SECRET_KEY` | Dashboard 登录 token 签名密钥 |
| `DASHBOARD_ADMIN_PASSWORD` | Dashboard 管理员初始密码 |

这些值都应该使用强随机字符串，不要使用示例值。

## Dashboard 可以直接用公网 HTTP 吗？

不建议。生产环境应使用 HTTPS 反向代理，并设置：

```dotenv
DASHBOARD_ALLOWED_ORIGINS=https://dashboard.example.com
```

没有可用域名证书时，使用 `scripts/verify-dashboard-ssh.ps1` 通过 SSH 隧道验证 Dashboard API，避免管理员密码经过公网 HTTP。

## 国内服务器下载 Release 很慢怎么办？

优先使用本地包：

```bash
sudo ./install.sh install \
  --package-url /tmp/nextunnel-server-linux-amd64.tar.gz \
  --sha256 <sha256>
```

或自建 COS/CDN：

```bash
sudo ./install.sh install \
  --version v0.6.4-alpha \
  --release-base-url https://cos.example.com/nextunnel/v0.6.4-alpha \
  --sha256 <sha256>
```

也可使用可信自建 GitHub 代理：

```bash
sudo ./install.sh install \
  --version v0.6.4-alpha \
  --github-proxy https://your-proxy.example.com/
```

## Windows 缺少 Wintun 怎么处理？

Windows 系统路由 TUN 需要官方、匹配架构的 `wintun.dll`。处理方式：

1. 优先安装带内置 Wintun 的 Windows 自定义安装包。
2. 便携包可把 `wintun.dll` 放到 `NexTunnel.exe` 同目录。
3. 设置环境变量 `NEXTUNNEL_WINTUN_DLL` 后重新打包。
4. 在桌面端网络页点击“修复 Wintun”。
5. 首次创建适配器需要管理员权限。

## `netsh interface ipv4 set subinterface nextunnel0 mtu=1420` 报错怎么办？

如果出现：

```text
文件名、目录名或卷标语法不正确
```

通常表示 Windows 没有识别到 `nextunnel0` 接口，而不是路径错误。检查：

- Wintun DLL 是否就绪。
- 是否以管理员身份运行 NexTunnel。
- Control Plane 下发的 `virtual-interface` 是否为本机真实适配器名。
- 网络页是否能显示真实 TUN 就绪。

## macOS 系统路由 TUN 是生产可用吗？

v0.6.4-alpha 中 macOS P2P/Relay 可用。系统路由 TUN 需要安装 signed/notarized pkg，pkg 会安装 `/Library/PrivilegedHelperTools/nextunnel-helper` 和 `com.nextunnel.helper` LaunchDaemon；DMG 不启用 System TUN。没有 helper 验证报告时，只能标注为外部阻塞或预览限制。

## Linux TUN 需要什么权限？

需要：

- `/dev/net/tun`
- `CAP_NET_ADMIN`
- 容器环境需挂载 TUN 设备并授予网络管理权限

验证前可检查：

```bash
ls -l /dev/net/tun
ip link
```

## 为什么 Dashboard 客户端页为空？

检查：

- Relay 是否启用 `-admin-listen`。
- Relay Admin Token 是否与 Dashboard 配置一致。
- `DASHBOARD_RELAY_ADMIN_URL` 是否从 Dashboard 进程所在环境可达。
- Relay Admin API 是否只监听 `127.0.0.1`，而 Dashboard 运行在容器中导致访问不到。

容器部署推荐：

```dotenv
DASHBOARD_RELAY_ADMIN_URL=http://relay-server:7001
```

## Control Plane 路由下发失败怎么办？

检查：

- 桌面端设置中 Control Plane URL 是否正确。
- `CONTROL_PLANE_API_TOKEN` 是否与桌面端一致。
- 节点是否已注册。
- `/api/v1/nodes/{id}/routes` 是否返回虚拟 IP 和路由。
- Control Plane 的 `virtual-subnet` 是否与本机网络/VPN 冲突。

## 如何验证生产部署？

按 [生产验证手册](./deploy/production-verification.md) 执行。外部条件包括：

- Dashboard HTTPS 域名和有效证书。
- Windows 管理员权限和 Wintun DLL。
- macOS signed/notarized pkg、LaunchDaemon helper 或验证环境 `sudo -n`。
- Linux eBPF 节点具备 root、clang、CAP_BPF/CAP_NET_ADMIN。
- Edge/Anycast 需要真实 Control Plane 或多地域节点。

验证脚本会输出 JSON 到 `dist/verification/`。发布说明只能把已有 JSON 报告支撑的能力标成“真实环境功能验收通过”或“生产压测通过”；缺少域名、证书、驱动、权限或真实节点时应标成“外部阻塞”。

## 可以把真实 token 写进 issue 或日志吗？

不可以。提交问题时请脱敏：

```text
Relay Token: <redacted>
Control Plane Token: <redacted>
Dashboard Password: <redacted>
```

桌面端配置导出默认会脱敏敏感字段，只有手动开启“包含敏感字段”才会导出 token。
