#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PID_FILE="${ROOT_DIR}/run/manager-api.pid"
LOG_DIR="${ROOT_DIR}/logs"
CONFIG_FILE="${ROOT_DIR}/configs/application.properties"

# 端口的算法必须和 stop.sh / main.go 完全一致（PA_SERVER_ADDRESS > configs > 默认），
# 否则「起不来」和「停不掉」会各说各话。
address="${PA_SERVER_ADDRESS:-}"
if [[ -z "${address}" && -f "${CONFIG_FILE}" ]]; then
  address="$(awk -F= '$1 == "server.address" { gsub(/[[:space:]]/, "", $2); print $2; exit }' "${CONFIG_FILE}")"
fi
address="${address:-:10013}"
port="${address##*:}"

if [[ -f "${PID_FILE}" ]] && kill -0 "$(cat "${PID_FILE}")" 2>/dev/null; then
  echo "manager-api is already running (pid $(cat "${PID_FILE}"))"
  exit 0
fi

holder="$(lsof -ti "TCP:${port}" -sTCP:LISTEN 2>/dev/null || true)"
if [[ -n "${holder}" ]]; then
  echo "Port ${port} is already in use by pid(s) ${holder}; manager-api was not started."
  echo "That is usually a go run / manager-api started outside these scripts: kill ${holder}"
  exit 1
fi

if [[ ! -f "${CONFIG_FILE}" && -z "${PA_SQLCONN:-}" ]]; then
  echo "Missing ${CONFIG_FILE}; copy it from configs/application.example.properties first." >&2
  exit 1
fi

if [[ ! -x "${ROOT_DIR}/bin/manager-api" ]]; then
  "${ROOT_DIR}/build.sh"
fi

# 必须在本目录下起：configs/application.properties 和 storage.dir=../data 都是相对工作目录的。
mkdir -p "${ROOT_DIR}/run" "${LOG_DIR}"
cd "${ROOT_DIR}"
nohup env PA_SERVER_ADDRESS="${address}" ./bin/manager-api >>"${LOG_DIR}/manager-api.log" 2>&1 &
echo $! >"${PID_FILE}"

# 启动时要跑迁移、连库，给足时间；进程中途退出就不用再等了。
for _ in {1..30}; do
  if curl -fsS "http://127.0.0.1:${port}/healthz" >/dev/null 2>&1; then
    echo "manager-api started on ${address} (pid $(cat "${PID_FILE}"))"
    exit 0
  fi
  if ! kill -0 "$(cat "${PID_FILE}")" 2>/dev/null; then
    rm -f "${PID_FILE}"
    echo "manager-api exited during startup; last log lines:" >&2
    tail -n 20 "${LOG_DIR}/manager-api.log" >&2
    exit 1
  fi
  sleep 1
done

echo "manager-api did not become ready; inspect ${LOG_DIR}/manager-api.log"
exit 1
