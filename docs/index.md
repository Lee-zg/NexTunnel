---
layout: home

hero:
  name: NexTunnel
  text: 内网穿透、P2P 直连优先与可视化运维
  tagline: 桌面客户端 · Relay 自动降级 · Control Plane · Dashboard · 生产验证工具链
  image:
    src: /logo.png
    alt: NexTunnel
  actions:
    - theme: brand
      text: 10 分钟快速开始
      link: /guide/getting-started
    - theme: alt
      text: 服务端部署
      link: /deploy/server
    - theme: alt
      text: 桌面端指南
      link: /desktop/overview
    - theme: alt
      text: CLI 手册
      link: /cli/overview

features:
  - title: 类 FRP/NPS 的快速穿透
    details: 通过 Relay TCP/QUIC 把本地 TCP/HTTP 服务映射到远端端口，适合开发调试和自部署服务暴露。
  - title: 桌面端完整工作流
    details: 管理服务端实例、连接 Relay、扫描本机端口、创建隧道、观察流量、查看运行日志。
  - title: Control Plane 路由下发
    details: 节点注册、IPAM、ACL、密钥、Peer 查询和虚拟网络路由统一由控制面管理。
  - title: Dashboard 运维闭环
    details: Web 管理台覆盖节点、客户端、流量、ACL、告警、审计、RBAC 和运行配置状态。
  - title: Windows Wintun 诊断
    details: 网络页展示 Wintun、管理员权限、真实 TUN 和路由注入状态，并提供修复入口。
  - title: 统一 CLI
    details: nextunnel CLI 覆盖服务端安装管理、远端 Control Plane/Dashboard 操作和本机桌面端控制。
  - title: 发布与生产验证
    details: Dashboard、P2P/TUN、Edge/Anycast、eBPF Linux 验证脚本输出 JSON 报告，适合发布前归档。
  - title: 明确 Beta 边界
    details: v0.6.4-alpha 标注 macOS 系统路由 TUN、HTTPS 域名证书和 eBPF 压测等外部依赖条件。
---

## 推荐阅读顺序

1. [快速开始](./guide/getting-started.md)：从空服务器到第一条隧道。
2. [服务端部署](./deploy/server.md)：了解 `install.sh`、`install.ps1`、Docker Compose、端口和 HTTPS。
3. [桌面端能力总览](./desktop/overview.md)：连接、隧道、端口扫描、网络健康、日志和设置。
4. [Dashboard 运维](./dashboard/operations.md)：登录、RBAC、客户端治理、ACL、告警和审计。
5. [FAQ](./faq.md)：常见部署、Wintun、TUN、证书和下载问题。

<style>
:root {
  --vp-c-brand-1: #00d5d5;
  --vp-c-brand-2: #00ffff;
  --vp-c-brand-3: #8a2be2;
  --vp-home-hero-name-color: transparent;
  --vp-home-hero-name-background: -webkit-linear-gradient(120deg, #1da1f2 20%, #8a2be2 72%);
  --vp-home-hero-image-background-image: linear-gradient(135deg, #00ffff 20%, #8a2be2 80%);
  --vp-home-hero-image-filter: blur(48px);
}
</style>
