#!/usr/bin/env bash
set -euo pipefail

VERSION="v0.7.0-beta"
PLATFORM="$(go env GOOS)/$(go env GOARCH)"
SKIP_FRONTEND="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -Version|--version)
      VERSION="${2:?version is required}"
      shift 2
      ;;
    -Platform|--platform)
      PLATFORM="${2:?platform is required}"
      shift 2
      ;;
    -SkipFrontend|--skip-frontend)
      SKIP_FRONTEND="true"
      shift
      ;;
    *)
      echo "未知参数：$1" >&2
      exit 64
      ;;
  esac
done

if [[ -z "${VERSION// }" ]]; then
  echo "Version 不能为空" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
DESKTOP_ROOT="$REPO_ROOT/desktop"
FRONTEND_ROOT="$DESKTOP_ROOT/frontend"
GO_BUILD_CACHE_ROOT="$REPO_ROOT/.tmp/go-build-cache"
NORMALIZED_VERSION="${VERSION#v}"
TARGET_GOOS="${PLATFORM%%/*}"
TARGET_GOARCH="${PLATFORM#*/}"
APP_BUNDLE_EXECUTABLE="${APP_BUNDLE_EXECUTABLE:-NexTunnel}"
APP_BUNDLE_IDENTIFIER="${APP_BUNDLE_IDENTIFIER:-com.nextunnel.desktop}"
WAILS_BUILD_TAGS="${WAILS_BUILD_TAGS:-desktop,wv2runtime.download,production}"
APP_SOURCE="$DESKTOP_ROOT/build/bin/NexTunnel.app"
APP_BINARY="$APP_SOURCE/Contents/MacOS/$APP_BUNDLE_EXECUTABLE"
APP_INFO_PLIST="$APP_SOURCE/Contents/Info.plist"
MACOS_WAILS_EXTRA_CGO_LDFLAGS="-framework UniformTypeIdentifiers"

mkdir -p "$GO_BUILD_CACHE_ROOT"
export GOCACHE="${GOCACHE:-$GO_BUILD_CACHE_ROOT}"

build_frontend_assets() {
  # Wails 绑定文件属于前端类型检查输入，先生成再执行 Vue/Vite 构建。
  (cd "$DESKTOP_ROOT" && wails generate module)
  (cd "$FRONTEND_ROOT" && npm run build)
}

write_macos_info_plist() {
  mkdir -p "$APP_SOURCE/Contents"
  cat > "$APP_INFO_PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>zh_CN</string>
  <key>CFBundleDisplayName</key>
  <string>NexTunnel</string>
  <key>CFBundleExecutable</key>
  <string>$APP_BUNDLE_EXECUTABLE</string>
  <key>CFBundleIdentifier</key>
  <string>$APP_BUNDLE_IDENTIFIER</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>NexTunnel</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>$NORMALIZED_VERSION</string>
  <key>CFBundleVersion</key>
  <string>$NORMALIZED_VERSION</string>
  <key>LSMinimumSystemVersion</key>
  <string>10.13.0</string>
  <key>NSHighResolutionCapable</key>
  <true/>
  <key>NSAppTransportSecurity</key>
  <dict>
    <key>NSAllowsLocalNetworking</key>
    <true/>
  </dict>
  <key>NSHumanReadableCopyright</key>
  <string>Copyright 2026 NexTunnel Contributors</string>
</dict>
</plist>
EOF
  printf 'APPL????' > "$APP_SOURCE/Contents/PkgInfo"
}

set_plist_value() {
  local key="$1"
  local value="$2"
  if /usr/libexec/PlistBuddy -c "Print :$key" "$APP_INFO_PLIST" >/dev/null 2>&1; then
    /usr/libexec/PlistBuddy -c "Set :$key $value" "$APP_INFO_PLIST"
  else
    /usr/libexec/PlistBuddy -c "Add :$key string $value" "$APP_INFO_PLIST"
  fi
}

repair_macos_app_metadata() {
  if [[ ! -f "$APP_INFO_PLIST" ]]; then
    write_macos_info_plist
  fi

  # Wails 的 outputfilename 和 Info.plist 必须一致，否则双击 .app 会找不到入口。
  set_plist_value "CFBundleExecutable" "$APP_BUNDLE_EXECUTABLE"
  set_plist_value "CFBundleShortVersionString" "$NORMALIZED_VERSION"
  set_plist_value "CFBundleVersion" "$NORMALIZED_VERSION"

  local legacy_binary="$APP_SOURCE/Contents/MacOS/nextunnel"
  if [[ ! -x "$APP_BINARY" && -x "$legacy_binary" ]]; then
    mv "$legacy_binary" "$APP_BINARY"
  fi
  if [[ -f "$DESKTOP_ROOT/build/appicon.png" ]]; then
    mkdir -p "$APP_SOURCE/Contents/Resources"
    cp "$DESKTOP_ROOT/build/appicon.png" "$APP_SOURCE/Contents/Resources/appicon.png"
  fi
}

validate_macos_app_bundle() {
  local plist_executable
  if [[ ! -x "$APP_BINARY" ]]; then
    echo "macOS app 缺少可执行文件：$APP_BINARY" >&2
    exit 1
  fi
  plist_executable="$(/usr/libexec/PlistBuddy -c "Print :CFBundleExecutable" "$APP_INFO_PLIST")"
  if [[ "$plist_executable" != "$APP_BUNDLE_EXECUTABLE" ]]; then
    echo "Info.plist CFBundleExecutable=$plist_executable，与 $APP_BUNDLE_EXECUTABLE 不一致" >&2
    exit 1
  fi
}

build_macos_binary_for_arch() {
  local arch="$1"
  local output="$2"
  (
    cd "$DESKTOP_ROOT"
    GOOS=darwin GOARCH="$arch" CGO_ENABLED=1 go build \
      -buildvcs=false \
      -trimpath \
      -tags "$WAILS_BUILD_TAGS" \
      -ldflags "-s -w -X main.AppVersion=$NORMALIZED_VERSION" \
      -o "$output"
  )
}

build_macos_app_bundle_fallback() {
  echo "Wails 构建失败，使用 Go 直构 fallback 组装 NexTunnel.app"
  rm -rf "$APP_SOURCE"
  mkdir -p "$APP_SOURCE/Contents/MacOS" "$APP_SOURCE/Contents/Resources"
  case "$TARGET_GOARCH" in
    amd64|arm64)
      build_macos_binary_for_arch "$TARGET_GOARCH" "$APP_BINARY"
      ;;
    *)
      echo "不支持的 macOS app 架构：$TARGET_GOARCH" >&2
      exit 1
      ;;
  esac
  chmod +x "$APP_BINARY"
  write_macos_info_plist
  repair_macos_app_metadata
}

run_wails_build() {
  (
    cd "$DESKTOP_ROOT"
    if [[ "$TARGET_GOOS" == "darwin" ]]; then
      # macOS 15+ SDK 下补充 UTI framework，保证 Wails/WebKit 链接稳定。
      export CGO_LDFLAGS="${CGO_LDFLAGS:+$CGO_LDFLAGS }$MACOS_WAILS_EXTRA_CGO_LDFLAGS"
    fi
    wails build \
      -m \
      -s \
      -clean \
      -trimpath \
      -platform "$PLATFORM" \
      -tags "$WAILS_BUILD_TAGS" \
      -ldflags "-X main.AppVersion=$NORMALIZED_VERSION" \
      -o "$APP_BUNDLE_EXECUTABLE"
  )
}

if [[ "$SKIP_FRONTEND" != "true" ]]; then
  build_frontend_assets
fi

if run_wails_build; then
  if [[ "$TARGET_GOOS" == "darwin" ]]; then
    repair_macos_app_metadata
    validate_macos_app_bundle
  fi
  exit 0
fi

if [[ "$TARGET_GOOS" != "darwin" ]]; then
  echo "Wails 构建失败，非 macOS 平台不支持 .app fallback。" >&2
  exit 1
fi

build_macos_app_bundle_fallback
validate_macos_app_bundle
