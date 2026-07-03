# 网络健康与 TUN

网络页用于查看当前连接、P2P、NAT、真实 TUN、Wintun 和虚拟网络路由状态。它面向故障恢复，不要求用户理解所有底层协议。

## 运行态信息

网络页会展示：

- 平台名称
- 生产数据面模式
- 真实内核 TUN 是否就绪
- 是否需要管理员权限
- Wintun DLL 状态
- macOS LaunchDaemon helper 状态
- NAT 类型和公网映射
- 虚拟 IP、网关、子网、MTU
- 最近执行的系统路由命令

## NAT 探测

点击“检测”会执行 STUN 探测。结果包括：

| 字段 | 含义 |
| --- | --- |
| NAT 类型 | 当前网络对 P2P 的友好程度 |
| Public Address | STUN 看到的公网映射地址 |
| Local Address | 本地 UDP 地址 |

STUN 失败通常表示 UDP 出站被限制、STUN 地址不可达或本地网络策略阻断。

## Windows Wintun

Windows 系统路由 TUN 需要：

- 官方 `wintun.dll`
- DLL 架构与 NexTunnel 进程一致
- 管理员权限
- 可创建或复用 `nextunnel0` 适配器

网络页会显示 DLL 路径和架构状态。缺失时可以执行“修复 Wintun”。需要管理员权限时，可以使用“以管理员身份重启修复”入口。

## macOS LaunchDaemon Helper

macOS 不需要第三方 TUN 驱动，系统内置 `utun`。生产阻塞点是普通桌面进程不能稳定创建 `utun` 和写系统路由，因此 System TUN 由 signed/notarized pkg 安装的 LaunchDaemon helper 处理：

- helper 路径：`/Library/PrivilegedHelperTools/nextunnel-helper`
- LaunchDaemon：`/Library/LaunchDaemons/com.nextunnel.helper.plist`
- Socket：`/var/run/nextunnel/helper.sock`
- 权限：root 运行，socket 为 `root:admin 0660`

网络页会显示 helper 是否运行、协议版本、socket 路径和签名状态。未安装 pkg 时，macOS 只能使用 Relay/P2P 能力，不应声明系统路由 TUN 生产可用。

## 应用虚拟网络路由

桌面端会从 Control Plane 拉取配置：

```json
{
  "node_id": "desktop-node-1",
  "virtual_ip": "10.7.0.2",
  "subnet": "10.7.0.0/24",
  "gateway": "10.7.0.1",
  "interface": "nextunnel0",
  "mtu": 1420,
  "routes": [
    {
      "destination": "10.7.0.0/24",
      "gateway": "10.7.0.1",
      "interface": "nextunnel0",
      "metric": 100
    }
  ]
}
```

Windows 上应用路由前会确认目标接口存在。若接口不存在，桌面端会提示修复 Wintun 或以管理员身份启动。

典型 Windows 命令形态：

```powershell
netsh interface ipv4 set subinterface interface=nextunnel0 mtu=1420 store=active
netsh interface ip set address name=nextunnel0 static 10.7.0.2 255.255.255.0
netsh interface ipv4 add route prefix=10.7.0.0/24 interface=nextunnel0 nexthop=10.7.0.1 metric=100 store=active
```

Linux 会使用 `ip` 命令。macOS 安装 helper 后由 LaunchDaemon 执行受控的 `ifconfig`/`route` 操作；未安装 helper 时，验证环境可用 `sudo -n`，桌面生产体验不依赖交互式 sudo。

## 常见错误

### `netsh ... subinterface nextunnel0 ... 文件名、目录名或卷标语法不正确`

这通常不是路径问题，而是 Windows 没有识别到目标网络接口。处理顺序：

1. 网络页确认 Wintun 是否就绪。
2. 确认以管理员身份运行。
3. 确认 Control Plane 下发的 `virtual-interface` 是 `nextunnel0`，或改成实际适配器名。
4. 重试“应用路由”。

### macOS 应用 TUN 失败

v0.6.4-alpha 中 macOS 系统路由 TUN 需要 root/sudo、授权 helper 或 LaunchDaemon。没有这些外部条件时，只声明 P2P/Relay 可用，系统路由 TUN 按预览能力处理。

### Linux TUN 失败

检查：

```bash
ls -l /dev/net/tun
ip link
```

容器环境需要挂载 `/dev/net/tun` 并授予 `NET_ADMIN`。真实生产验收不要用用户态 `netTun` 回退结果代替。

## 重置路由

点击“重置路由”会删除当前状态中记录的路由，并释放桌面端持有的 TUN 句柄。若系统命令失败，网络页会保留错误和已执行命令，便于排障。
