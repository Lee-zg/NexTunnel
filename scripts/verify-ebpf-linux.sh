#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

INTERFACE_NAME="${INTERFACE_NAME:-eth0}"
XDP_MODE="${XDP_MODE:-skb}"
VERIFY_PORT="${VERIFY_PORT:-9}"
REPORT_PATH="${REPORT_PATH:-${ROOT_DIR}/dist/verification/ebpf-benchmark-latest.json}"
OBJECT_PATH="${OBJECT_PATH:-${ROOT_DIR}/xdp_forwarder_bpfel.o}"
SOURCE_PATH="${SOURCE_PATH:-${ROOT_DIR}/server/internal/ebpf/xdp_forwarder.c}"
VERIFY_BINARY="${VERIFY_BINARY:-${ROOT_DIR}/bin/ebpf-verify}"

if [[ ! -f "${SOURCE_PATH}" ]]; then
  SOURCE_PATH="${ROOT_DIR}/xdp_forwarder.c"
fi

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "eBPF XDP 生产验证只能在 Linux 节点执行" >&2
  exit 1
fi

if [[ "${EUID}" -ne 0 ]]; then
  echo "eBPF XDP 挂载需要 root 或等价 CAP_BPF/CAP_NET_ADMIN 权限" >&2
  exit 1
fi

if ! command -v clang >/dev/null 2>&1; then
  echo "缺少 clang，无法编译 XDP BPF 对象" >&2
  exit 1
fi

if [[ ! -x "${VERIFY_BINARY}" ]] && ! command -v go >/dev/null 2>&1; then
  echo "缺少 go，且未找到包内 bin/ebpf-verify，无法运行验证命令" >&2
  exit 1
fi

if [[ ! -f "${SOURCE_PATH}" ]]; then
  echo "缺少 xdp_forwarder.c：${SOURCE_PATH}" >&2
  exit 1
fi

mkdir -p "$(dirname "${REPORT_PATH}")"

# 生产验证使用仓库内 C 程序直接编译，避免误加载旧对象文件。
clang -O2 -g -target bpf -D__TARGET_ARCH_x86 \
  -c "${SOURCE_PATH}" \
  -o "${OBJECT_PATH}"

if [[ -x "${VERIFY_BINARY}" ]]; then
  "${VERIFY_BINARY}" \
    -interface "${INTERFACE_NAME}" \
    -object "${OBJECT_PATH}" \
    -xdp-mode "${XDP_MODE}" \
    -verify-port "${VERIFY_PORT}" \
    -require-kernel=true \
    > "${REPORT_PATH}"
else
  pushd "${ROOT_DIR}/server" >/dev/null
  go run ./cmd/ebpf-verify \
    -interface "${INTERFACE_NAME}" \
    -object "${OBJECT_PATH}" \
    -xdp-mode "${XDP_MODE}" \
    -verify-port "${VERIFY_PORT}" \
    -require-kernel=true \
    > "${REPORT_PATH}"
  popd >/dev/null
fi

cat "${REPORT_PATH}"
