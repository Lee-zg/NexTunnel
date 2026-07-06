#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-}"
case "$ACTION" in
  install|restart|uninstall) ;;
  *)
    echo "用法：install-helper.sh install|restart|uninstall" >&2
    exit 64
    ;;
esac

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_CONTENTS_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
APP_BINARY="$APP_CONTENTS_DIR/MacOS/NexTunnel"
HELPER_BINARY="$SCRIPT_DIR/nextunnel-helper"
PLIST_PATH="$SCRIPT_DIR/com.nextunnel.helper.plist"

if [[ ! -x "$APP_BINARY" ]]; then
  echo "未找到 NexTunnel 应用管理入口：$APP_BINARY" >&2
  exit 1
fi
if [[ ! -x "$HELPER_BINARY" ]]; then
  echo "未找到 nextunnel-helper：$HELPER_BINARY" >&2
  exit 1
fi
if [[ ! -f "$PLIST_PATH" ]]; then
  echo "未找到 LaunchDaemon plist：$PLIST_PATH" >&2
  exit 1
fi

exec "$APP_BINARY" \
  --macos-helper-admin "$ACTION" \
  --helper-binary "$HELPER_BINARY" \
  --plist "$PLIST_PATH"
