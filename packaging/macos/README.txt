NexTunnel macOS 安装说明

1. 将 NexTunnel.app 拖入 Applications。
2. signed/notarized Release 可直接打开；如使用官方 GitHub Release 的 unsigned alpha DMG，请先核对随附 SHA256，再在“系统设置 > 隐私与安全性”中允许打开。
3. NexTunnel 的 P2P/Relay 功能不需要安装内核组件。
4. 如需 macOS 真实系统路由 TUN，请在应用内“网络”页面点击“安装 Helper”，并按 macOS 提示输入管理员密码；也可执行 sudo nextunnel helper install。
5. signed/notarized PKG 只是更顺滑的一键安装增强，不是开源默认使用前置。
6. 应用内“网络”页面会显示 helper、utun/TUN 状态和可执行修复建议。
