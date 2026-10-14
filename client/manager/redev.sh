#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# next build 会整个重写 .next，正在跑的 next start 读到一半的产物会出错，所以先停再编。
"${SCRIPT_DIR}/stop.sh"
"${SCRIPT_DIR}/build.sh"
"${SCRIPT_DIR}/start.sh"
