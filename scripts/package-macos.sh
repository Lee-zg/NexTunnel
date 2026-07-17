#!/usr/bin/env bash
set -euo pipefail

VERSION="v0.7.0-beta"
PLATFORM="darwin/universal"
SKIP_FRONTEND="false"
SIGN="false"
NOTARIZE="false"
BUILD_PKG="auto"

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
    -Sign|--sign)
      SIGN="true"
      shift
      ;;
    -Notarize|--notarize)
      NOTARIZE="true"
      shift
      ;;
    -Pkg|--pkg)
      BUILD_PKG="true"
      shift
      ;;
    -SkipPkg|--skip-pkg)
      BUILD_PKG="false"
      shift
      ;;
    *)
      echo "未知参数：$1" >&2
      exit 64
      ;;
  esac
done

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "macOS DMG 只能在 macOS runner 或 macOS 本机打包。" >&2
  exit 1
fi

if [[ "$NOTARIZE" == "true" && "$SIGN" != "true" ]]; then
  echo "启用 notarization 必须同时启用 --sign，避免公证未签名产物。" >&2
  exit 1
fi

if [[ -z "${VERSION// }" ]]; then
  echo "Version 不能为空" >&2
  exit 1
fi

VERSION_BODY="${VERSION#v}"
if [[ -z "$VERSION_BODY" ]]; then
  echo "Version 包含非法字符：$VERSION" >&2
  exit 1
fi
case "${VERSION_BODY:0:1}" in
  [A-Za-z0-9]) ;;
  *)
    echo "Version 包含非法字符：$VERSION" >&2
    exit 1
    ;;
esac
case "$VERSION_BODY" in
  *[!A-Za-z0-9.+-]*)
    echo "Version 包含非法字符：$VERSION" >&2
    exit 1
    ;;
esac
if [[ "$VERSION" != "$VERSION_BODY" && "${VERSION:0:1}" != "v" ]]; then
  echo "Version 包含非法字符：$VERSION" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
DESKTOP_ROOT="$REPO_ROOT/desktop"
FRONTEND_ROOT="$DESKTOP_ROOT/frontend"
DIST_ROOT="$REPO_ROOT/dist"
GO_BUILD_CACHE_ROOT="$REPO_ROOT/.tmp/go-build-cache"
MACOS_PACKAGING_ROOT="$REPO_ROOT/packaging/macos"
NORMALIZED_VERSION="${VERSION#v}"
TARGET_ARCH="${PLATFORM#darwin/}"
TARGET_NAME="nextunnel-${VERSION}-darwin-${TARGET_ARCH}"
STAGING_ROOT="$DIST_ROOT/$TARGET_NAME"
DMG_STAGING="$DIST_ROOT/${TARGET_NAME}-dmg"
DMG_PATH="$DIST_ROOT/${TARGET_NAME}.dmg"
APP_SOURCE="$DESKTOP_ROOT/build/bin/NexTunnel.app"
APP_TARGET="$STAGING_ROOT/NexTunnel.app"
SIGNING_STATE="unsigned-alpha"
HELPER_SIGNED_VALUE="false"
HELPER_BUILD_ROOT="$DIST_ROOT/${TARGET_NAME}-helper-build"
HELPER_TARGET="$STAGING_ROOT/nextunnel-helper"
HELPER_PLIST_SOURCE="$MACOS_PACKAGING_ROOT/com.nextunnel.helper.plist"
HELPER_INSTALL_SCRIPT_SOURCE="$REPO_ROOT/scripts/install-macos-helper.sh"
MACOS_HELPER_RESOURCE_ROOT="$APP_TARGET/Contents/Resources/macos-helper"
HELPER_RESOURCE_TARGET="$MACOS_HELPER_RESOURCE_ROOT/nextunnel-helper"
HELPER_RESOURCE_PLIST_TARGET="$MACOS_HELPER_RESOURCE_ROOT/com.nextunnel.helper.plist"
HELPER_RESOURCE_INSTALL_SCRIPT_TARGET="$MACOS_HELPER_RESOURCE_ROOT/install-helper.sh"
PKG_ROOT="$DIST_ROOT/${TARGET_NAME}-pkgroot"
PKG_SCRIPTS_ROOT="$DIST_ROOT/${TARGET_NAME}-pkg-scripts"
PKG_PATH="$DIST_ROOT/${TARGET_NAME}.pkg"
APP_FALLBACK_BUILD_ROOT="$DIST_ROOT/${TARGET_NAME}-app-build"
APP_BUNDLE_EXECUTABLE="NexTunnel"
APP_BUNDLE_IDENTIFIER="com.nextunnel.desktop"
WAILS_BUILD_TAGS="desktop,wv2runtime.download,production"
MACOS_WAILS_EXTRA_CGO_LDFLAGS="-framework UniformTypeIdentifiers"

if [[ "$NOTARIZE" == "true" ]]; then
  SIGNING_STATE="notarized"
elif [[ "$SIGN" == "true" ]]; then
  SIGNING_STATE="signed"
fi

if [[ "$BUILD_PKG" == "auto" ]]; then
  if [[ "$SIGN" == "true" || "$NOTARIZE" == "true" ]]; then
    BUILD_PKG="true"
  else
    BUILD_PKG="false"
  fi
fi

require_tool() {
  local tool_name="$1"
  if ! command -v "$tool_name" >/dev/null 2>&1; then
    echo "未找到必需命令：$tool_name" >&2
    exit 1
  fi
}

require_env_vars() {
  local var_name
  for var_name in "$@"; do
    if [[ -z "${!var_name:-}" ]]; then
      echo "缺少必需环境变量：$var_name" >&2
      exit 1
    fi
  done
}

preflight_tools_and_secrets() {
  local base_tools=(go npm wails hdiutil shasum awk codesign)
  if [[ "$TARGET_ARCH" == "universal" ]]; then
    base_tools+=(lipo)
  fi
  if [[ "$BUILD_PKG" == "true" ]]; then
    base_tools+=(pkgbuild)
  fi
  if [[ "$SIGN" == "true" ]]; then
    base_tools+=(pkgutil spctl)
    require_env_vars MACOS_DEVELOPER_ID_APPLICATION
    if [[ "$BUILD_PKG" == "true" ]]; then
      require_env_vars MACOS_DEVELOPER_ID_INSTALLER
    fi
  fi
  if [[ "$NOTARIZE" == "true" ]]; then
    base_tools+=(xcrun)
    require_env_vars MACOS_NOTARY_APPLE_ID MACOS_NOTARY_TEAM_ID MACOS_NOTARY_PASSWORD
  fi
  local tool_name
  for tool_name in "${base_tools[@]}"; do
    require_tool "$tool_name"
  done
}

verify_code_signature() {
  local path="$1"
  local label="$2"
  # 签名验证放在打包前执行，避免把不可公证的 helper 或 app 写入 DMG/PKG。
  codesign --verify --strict --verbose=2 "$path"
  codesign --display --verbose=2 "$path" >/dev/null
  echo "$label 签名验证通过：$path"
}

verify_pkg_signature() {
  local pkg_path="$1"
  # pkgutil 先校验证书链；安装评估在公证后再执行，避免 Gatekeeper 状态未刷新。
  pkgutil --check-signature "$pkg_path"
  echo "PKG 签名验证通过：$pkg_path"
}

assess_pkg_install() {
  local pkg_path="$1"
  # spctl 校验最终安装评估，signed/notarized 发布必须通过这一关。
  spctl -a -vv -t install "$pkg_path"
  echo "PKG 安装评估通过：$pkg_path"
}

notarize_and_staple() {
  local artifact_path="$1"
  local label="$2"
  # notarytool --wait 直接等待 Apple 公证结果，stapler validate 确认 ticket 已写入产物。
  xcrun notarytool submit "$artifact_path" \
    --apple-id "$MACOS_NOTARY_APPLE_ID" \
    --team-id "$MACOS_NOTARY_TEAM_ID" \
    --password "$MACOS_NOTARY_PASSWORD" \
    --wait
  xcrun stapler staple "$artifact_path"
  xcrun stapler validate "$artifact_path"
  echo "$label 公证与 stapler 校验通过：$artifact_path"
}

preflight_tools_and_secrets

build_helper_for_arch() {
  local arch="$1"
  local output="$2"
  (
    cd "$DESKTOP_ROOT"
    GOOS=darwin GOARCH="$arch" CGO_ENABLED=0 go build \
      -trimpath \
      -ldflags "-s -w -X main.version=$NORMALIZED_VERSION -X main.signed=$HELPER_SIGNED_VALUE" \
      -o "$output" \
      ./cmd/nextunnel-helper
  )
}

build_macos_helper() {
  rm -rf "$HELPER_BUILD_ROOT"
  mkdir -p "$HELPER_BUILD_ROOT"
  case "$TARGET_ARCH" in
    universal)
      build_helper_for_arch amd64 "$HELPER_BUILD_ROOT/nextunnel-helper-amd64"
      build_helper_for_arch arm64 "$HELPER_BUILD_ROOT/nextunnel-helper-arm64"
      lipo -create "$HELPER_BUILD_ROOT/nextunnel-helper-amd64" "$HELPER_BUILD_ROOT/nextunnel-helper-arm64" -output "$HELPER_TARGET"
      ;;
    amd64|arm64)
      build_helper_for_arch "$TARGET_ARCH" "$HELPER_TARGET"
      ;;
    *)
      echo "不支持的 macOS helper 架构：$TARGET_ARCH" >&2
      exit 1
      ;;
  esac
}

install_macos_helper_resources() {
  if [[ ! -f "$HELPER_PLIST_SOURCE" ]]; then
    echo "未找到 LaunchDaemon plist：$HELPER_PLIST_SOURCE" >&2
    exit 1
  fi
  if [[ ! -f "$HELPER_INSTALL_SCRIPT_SOURCE" ]]; then
    echo "未找到 helper 安装脚本：$HELPER_INSTALL_SCRIPT_SOURCE" >&2
    exit 1
  fi
  mkdir -p "$MACOS_HELPER_RESOURCE_ROOT"
  cp "$HELPER_TARGET" "$HELPER_RESOURCE_TARGET"
  cp "$HELPER_PLIST_SOURCE" "$HELPER_RESOURCE_PLIST_TARGET"
  cp "$HELPER_INSTALL_SCRIPT_SOURCE" "$HELPER_RESOURCE_INSTALL_SCRIPT_TARGET"
  chmod 755 "$HELPER_RESOURCE_TARGET" "$HELPER_RESOURCE_INSTALL_SCRIPT_TARGET"
  chmod 644 "$HELPER_RESOURCE_PLIST_TARGET"
}

write_fallback_app_info_plist() {
  local plist_path="$APP_SOURCE/Contents/Info.plist"
  cat > "$plist_path" <<EOF
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
  <string>12.0</string>
  <key>NSHighResolutionCapable</key>
  <true/>
</dict>
</plist>
EOF
  printf 'APPL????' > "$APP_SOURCE/Contents/PkgInfo"
}

build_desktop_binary_for_arch() {
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
  local app_binary="$APP_SOURCE/Contents/MacOS/$APP_BUNDLE_EXECUTABLE"
  echo "Wails CLI 打包失败，使用 Go 直构 fallback 组装 NexTunnel.app"
  rm -rf "$APP_SOURCE" "$APP_FALLBACK_BUILD_ROOT"
  mkdir -p "$APP_SOURCE/Contents/MacOS" "$APP_SOURCE/Contents/Resources" "$APP_FALLBACK_BUILD_ROOT"
  case "$TARGET_ARCH" in
    universal)
      build_desktop_binary_for_arch amd64 "$APP_FALLBACK_BUILD_ROOT/NexTunnel-amd64"
      build_desktop_binary_for_arch arm64 "$APP_FALLBACK_BUILD_ROOT/NexTunnel-arm64"
      lipo -create "$APP_FALLBACK_BUILD_ROOT/NexTunnel-amd64" "$APP_FALLBACK_BUILD_ROOT/NexTunnel-arm64" -output "$app_binary"
      ;;
    amd64|arm64)
      build_desktop_binary_for_arch "$TARGET_ARCH" "$app_binary"
      ;;
    *)
      echo "不支持的 macOS app 架构：$TARGET_ARCH" >&2
      exit 1
      ;;
  esac
  chmod +x "$app_binary"
  write_fallback_app_info_plist
  if [[ -f "$DESKTOP_ROOT/build/appicon.png" ]]; then
    cp "$DESKTOP_ROOT/build/appicon.png" "$APP_SOURCE/Contents/Resources/appicon.png"
  fi
}

mkdir -p "$DIST_ROOT" "$GO_BUILD_CACHE_ROOT"
export GOCACHE="${GOCACHE:-$GO_BUILD_CACHE_ROOT}"

if [[ "$SKIP_FRONTEND" != "true" ]]; then
  echo "构建桌面端前端"
  (cd "$FRONTEND_ROOT" && npm run build)
fi

echo "打包 macOS 桌面端 $VERSION ($PLATFORM)"
(
  cd "$DESKTOP_ROOT"
  # macOS 15+ SDK 下 Wails 依赖 UTType，显式补充链接框架避免 arm64/universal 链接失败。
  export CGO_LDFLAGS="${CGO_LDFLAGS:+$CGO_LDFLAGS }$MACOS_WAILS_EXTRA_CGO_LDFLAGS"
  if ! wails build \
    -m \
    -s \
    -trimpath \
    -platform "$PLATFORM" \
    -tags "$WAILS_BUILD_TAGS" \
    -o "NexTunnel" \
    -ldflags "-s -w -X main.AppVersion=$NORMALIZED_VERSION"; then
    build_macos_app_bundle_fallback
  fi
)

if [[ ! -d "$APP_SOURCE" ]]; then
  echo "未找到 Wails macOS 产物：$APP_SOURCE" >&2
  exit 1
fi

rm -rf "$STAGING_ROOT" "$DMG_STAGING" "$DMG_PATH" "$PKG_ROOT" "$PKG_SCRIPTS_ROOT" "$PKG_PATH" "$PKG_PATH.sha256"
mkdir -p "$STAGING_ROOT" "$DMG_STAGING"
cp -R "$APP_SOURCE" "$APP_TARGET"
if [[ "$SIGN" == "true" ]]; then
  HELPER_SIGNED_VALUE="true"
fi
build_macos_helper

if [[ "$SIGN" == "true" ]]; then
  # 使用 hardened runtime，为后续 notarization 做准备。
  codesign --force --options runtime --timestamp --sign "$MACOS_DEVELOPER_ID_APPLICATION" "$HELPER_TARGET"
fi

install_macos_helper_resources

if [[ "$SIGN" == "true" ]]; then
  codesign --force --options runtime --timestamp --sign "$MACOS_DEVELOPER_ID_APPLICATION" "$HELPER_RESOURCE_TARGET"
  codesign --force --deep --options runtime --timestamp --sign "$MACOS_DEVELOPER_ID_APPLICATION" "$APP_TARGET"
  verify_code_signature "$HELPER_TARGET" "nextunnel-helper"
  verify_code_signature "$HELPER_RESOURCE_TARGET" "NexTunnel.app 内置 nextunnel-helper"
  verify_code_signature "$APP_TARGET" "NexTunnel.app"
else
  # Wails 生成的 App 已带临时签名；注入 helper 后必须重建资源清单，否则 Gatekeeper 会误报应用已损坏。
  codesign --force --sign - "$HELPER_RESOURCE_TARGET"
  codesign --force --sign - "$APP_TARGET"
  verify_code_signature "$HELPER_RESOURCE_TARGET" "NexTunnel.app 内置 nextunnel-helper（临时签名）"
  verify_code_signature "$APP_TARGET" "NexTunnel.app（临时签名）"
fi

cp -R "$APP_TARGET" "$DMG_STAGING/NexTunnel.app"
ln -s /Applications "$DMG_STAGING/Applications"
cp "$MACOS_PACKAGING_ROOT/README.txt" "$DMG_STAGING/README.txt"

BACKGROUND_SVG="$MACOS_PACKAGING_ROOT/create-dmg-background.svg"
if [[ -f "$BACKGROUND_SVG" ]]; then
  mkdir -p "$DMG_STAGING/.background"
  cp "$BACKGROUND_SVG" "$DMG_STAGING/.background/background.svg"
fi

MANIFEST_PATH="$STAGING_ROOT/MANIFEST.txt"
RELEASE_MANIFEST_PATH="$DIST_ROOT/${TARGET_NAME}.MANIFEST.txt"
cat > "$MANIFEST_PATH" <<EOF
NexTunnel desktop installer
Version: $VERSION
ApplicationVersion: $NORMALIZED_VERSION
Target: $PLATFORM
Installer: dmg
Binary: NexTunnel.app
Wintun: skipped; macOS uses utun
macOSHelper: $HELPER_TARGET
macOSHelperResource: NexTunnel.app/Contents/Resources/macos-helper
macOSHelperLaunchDaemon: /Library/LaunchDaemons/com.nextunnel.helper.plist
macOSHelperInstall: user-admin-authorized
Signing: $SIGNING_STATE
PrunedResources: true
EOF
cp "$MANIFEST_PATH" "$DMG_STAGING/MANIFEST.txt"
cp "$MANIFEST_PATH" "$RELEASE_MANIFEST_PATH"

hdiutil create \
  -volname "NexTunnel ${VERSION}" \
  -srcfolder "$DMG_STAGING" \
  -ov \
  -format UDZO \
  "$DMG_PATH"

if [[ "$NOTARIZE" == "true" ]]; then
  notarize_and_staple "$DMG_PATH" "DMG"
fi

(cd "$DIST_ROOT" && shasum -a 256 "$(basename "$DMG_PATH")" | awk '{print tolower($1) "  " $2}' > "$(basename "$DMG_PATH").sha256")

if [[ "$BUILD_PKG" == "true" ]]; then
  if [[ ! -f "$HELPER_PLIST_SOURCE" ]]; then
    echo "未找到 LaunchDaemon plist：$HELPER_PLIST_SOURCE" >&2
    exit 1
  fi
  mkdir -p "$PKG_ROOT/Applications" "$PKG_ROOT/Library/PrivilegedHelperTools" "$PKG_ROOT/Library/LaunchDaemons" "$PKG_SCRIPTS_ROOT"
  cp -R "$APP_TARGET" "$PKG_ROOT/Applications/NexTunnel.app"
  cp "$HELPER_TARGET" "$PKG_ROOT/Library/PrivilegedHelperTools/nextunnel-helper"
  cp "$HELPER_PLIST_SOURCE" "$PKG_ROOT/Library/LaunchDaemons/com.nextunnel.helper.plist"
  cat > "$PKG_SCRIPTS_ROOT/preinstall" <<'EOF'
#!/bin/sh
set -e
/bin/launchctl bootout system/com.nextunnel.helper >/dev/null 2>&1 || true
exit 0
EOF
  cat > "$PKG_SCRIPTS_ROOT/postinstall" <<'EOF'
#!/bin/sh
set -e
/usr/sbin/chown root:wheel /Library/PrivilegedHelperTools/nextunnel-helper
/bin/chmod 755 /Library/PrivilegedHelperTools/nextunnel-helper
/usr/sbin/chown root:wheel /Library/LaunchDaemons/com.nextunnel.helper.plist
/bin/chmod 644 /Library/LaunchDaemons/com.nextunnel.helper.plist
/bin/mkdir -p /var/run/nextunnel
/usr/sbin/chown root:admin /var/run/nextunnel
/bin/chmod 770 /var/run/nextunnel
/bin/launchctl bootstrap system /Library/LaunchDaemons/com.nextunnel.helper.plist >/dev/null 2>&1 || true
/bin/launchctl enable system/com.nextunnel.helper >/dev/null 2>&1 || true
exit 0
EOF
  chmod +x "$PKG_SCRIPTS_ROOT/preinstall" "$PKG_SCRIPTS_ROOT/postinstall"
  PKGBUILD_ARGS=(
    --root "$PKG_ROOT"
    --scripts "$PKG_SCRIPTS_ROOT"
    --identifier "com.nextunnel.desktop"
    --version "$NORMALIZED_VERSION"
    --install-location "/"
  )
  if [[ "$SIGN" == "true" ]]; then
    PKGBUILD_ARGS+=(--sign "$MACOS_DEVELOPER_ID_INSTALLER")
  fi
  pkgbuild "${PKGBUILD_ARGS[@]}" "$PKG_PATH"
  if [[ "$SIGN" == "true" ]]; then
    verify_pkg_signature "$PKG_PATH"
  fi
  if [[ "$NOTARIZE" == "true" ]]; then
    notarize_and_staple "$PKG_PATH" "PKG"
  fi
  if [[ "$SIGN" == "true" ]]; then
    assess_pkg_install "$PKG_PATH"
  fi
  (cd "$DIST_ROOT" && shasum -a 256 "$(basename "$PKG_PATH")" | awk '{print tolower($1) "  " $2}' > "$(basename "$PKG_PATH").sha256")
  echo "macOS System TUN PKG 已生成：$PKG_PATH"
  echo "SHA256：$PKG_PATH.sha256"
fi

echo "macOS DMG 已生成：$DMG_PATH"
echo "SHA256：$DMG_PATH.sha256"
