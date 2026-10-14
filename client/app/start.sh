#!/usr/bin/env bash
set -euo pipefail

BASE_PATH="${BASE_PATH-/assistant}"
APP_NAME="pa-app"
PORT="${PORT:-7906}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PID_FILE="${SCRIPT_DIR}/run/${APP_NAME}.pid"
LOG_DIR="${LOG_DIR:-${SCRIPT_DIR}/logs}"
LOG_FILE="${LOG_FILE:-${LOG_DIR}/${APP_NAME}.log}"

cd "${SCRIPT_DIR}"

if [[ -f "${PID_FILE}" ]] && kill -0 "$(cat "${PID_FILE}")" 2>/dev/null; then
  echo "${APP_NAME} is already running (pid $(cat "${PID_FILE}"))"
  exit 0
fi

holder="$(lsof -ti "TCP:${PORT}" -sTCP:LISTEN 2>/dev/null || true)"
if [[ -n "${holder}" ]]; then
  echo "Port ${PORT} is already in use by pid(s) ${holder}; ${APP_NAME} was not started."
  echo "That is usually a npm run dev started outside these scripts: kill ${holder}"
  exit 1
fi

# 这里跑的是构建产物，不是 next dev：改完代码要 ./redev.sh，光重启只会把同一份 .next 再跑一遍。
# 看 BUILD_ID 而不是 .next 目录：npm run dev 也会生成 .next，但那份 next start 跑不起来。
if [[ ! -x node_modules/.bin/next || ! -f .next/BUILD_ID ]]; then
  "${SCRIPT_DIR}/build.sh"
fi

mkdir -p "${SCRIPT_DIR}/run" "${LOG_DIR}"
NEXT_TELEMETRY_DISABLED=1 nohup ./node_modules/.bin/next start -p "${PORT}" >>"${LOG_FILE}" 2>&1 &
echo $! >"${PID_FILE}"

for _ in {1..20}; do
  if curl -fsS "http://127.0.0.1:${PORT}${BASE_PATH}/login" >/dev/null 2>&1; then
    echo "${APP_NAME} started on port ${PORT} (pid $(cat "${PID_FILE}"))"
    exit 0
  fi
  if ! kill -0 "$(cat "${PID_FILE}")" 2>/dev/null; then
    rm -f "${PID_FILE}"
    echo "${APP_NAME} exited during startup; last log lines:" >&2
    tail -n 20 "${LOG_FILE}" >&2
    exit 1
  fi
  sleep 1
done

echo "${APP_NAME} did not become ready; inspect ${LOG_FILE}"
exit 1
