# 隧道与端口

隧道页用于把本机服务暴露到 Relay 远端端口。端口管理器会扫描回环地址常用端口，并把扫描结果快速填入隧道表单。

## 创建 TCP 隧道

示例：把本机 Vite 服务暴露到远端 `13000`。

```bash
npx vite --host 127.0.0.1 --port 3000
```

桌面端填写：

| 字段 | 值 |
| --- | --- |
| 名称 | `web-3000` |
| 类型 | `tcp` |
| 本地地址 | `127.0.0.1` |
| 本地端口 | `3000` |
| 远端端口 | `13000` |

启动后访问：

```bash
curl http://example.com:13000
```

## 创建 HTTP 隧道

HTTP 隧道适合 Web 开发服务。没有配置公开域名时，它和 TCP 一样使用 Relay 远端端口。

```text
名称：local-web
类型：http
本地地址：127.0.0.1
本地端口：5173
远端端口：15173
```

如果 Relay 启用了 Public HTTP Gateway，可以把远端端口设为 `0`，并填写公开域名或子域名：

```text
名称：local-web
类型：http
本地地址：127.0.0.1
本地端口：5173
远端端口：0
公开域名：demo
访问策略 ID：basic-dev
请求观测：开启
```

Relay 会根据 `domain-suffix` 把 `demo` 补全为 `demo.<suffix>`，注册成功后隧道列表会显示 `public_url`，可直接复制。访问策略 ID 必须先通过 Dashboard 或 Relay Admin API 创建。

## 启动、停止和编辑

| 操作 | 规则 |
| --- | --- |
| 启动 | 必须先连接 Relay |
| 停止 | 会从运行时代理中移除该隧道 |
| 编辑 | 需要先停止隧道，避免运行态和持久化配置不一致 |
| 删除 | 会删除本地配置，并尝试移除运行时代理 |

如果出现 `server is not connected`，先回到总览页连接 Relay。

## 本机端口扫描

端口扫描只允许回环地址：

- `127.0.0.1`
- `::1`
- `localhost`

扫描默认使用启用的常用端口，限制最大端口数量和连接超时，避免误扫外部网络。

## 常用端口分类

内置端口覆盖：

| 类型 | 示例 |
| --- | --- |
| 开发 | Next.js、Vite、Spring Boot、Laravel、Python |
| 数据库 | MySQL、PostgreSQL、Redis、MongoDB、Elasticsearch |
| 软件服务 | MinIO、Prometheus、Docker API |
| 游戏 | Minecraft Java、Terraria、Steam/Source、Palworld |
| 远程访问 | SSH、Remote Desktop |
| 标准服务 | HTTP、HTTPS |

高风险端口如 SSH、RDP、Docker API 默认不应随意公开。暴露前请确认认证和访问边界。

## 快速创建流程

1. 打开“隧道”页。
2. 点击端口扫描。
3. 选择开放的本机端口。
4. 点击填入或创建隧道。
5. 修改远端端口。
6. 保存并启动。

远端端口默认可以和本地端口一致，但生产环境建议使用明确规划的端口段，避免和已有服务冲突。

## 常见问题

### 隧道启动后访问不到

检查：

- Relay 是否已连接。
- 服务端防火墙是否放行远端端口。
- 本机服务是否只监听 `127.0.0.1` 或目标地址。
- 本地端口是否仍处于开放状态。
- Relay Token 是否与服务端一致。

### 可以扫描公网地址吗？

不可以。桌面端端口扫描被限制在回环地址，避免把本工具变成外部网络扫描器。
