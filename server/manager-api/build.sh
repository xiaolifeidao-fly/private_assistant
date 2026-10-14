#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# 不能 GOWORK=off：go.mod 里没有 require/replace，common / service / migrations
# 全靠 server/go.work 找到，关掉 workspace 就编不过。
cd "${ROOT_DIR}"
mkdir -p bin
go build -o bin/manager-api .

echo "Built ${ROOT_DIR}/bin/manager-api"
