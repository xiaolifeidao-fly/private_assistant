#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

cd "${SCRIPT_DIR}"
if [[ ! -x node_modules/.bin/next ]]; then
  npm install
fi

# next.config.mjs 里 /api 的 rewrite 目标（MANAGER_API）是构建期写进 routes-manifest 的，
# 后端地址不是默认的 127.0.0.1:10013 时，要在这一步带上：MANAGER_API=http://... ./build.sh
NEXT_TELEMETRY_DISABLED=1 npm run build

echo "Built ${SCRIPT_DIR}/.next"
