#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# 先编再停：编译失败时旧进程还在跑，不至于把服务停在半路。
"${ROOT_DIR}/build.sh"
"${ROOT_DIR}/stop.sh"
"${ROOT_DIR}/start.sh"
