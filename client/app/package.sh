#!/usr/bin/env bash
set -euo pipefail

APP_NAME="pa-app"
export LC_ALL=C
export LANG=C

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PACKAGE_NAME="$APP_NAME-linux-x64"
PACKAGE_ROOT="${PACKAGE_ROOT:-$SCRIPT_DIR/package}"
DIST_DIR="$PACKAGE_ROOT/$PACKAGE_NAME"
ARCHIVE="${ARCHIVE:-$SCRIPT_DIR/$PACKAGE_NAME.tar.gz}"

cd "$SCRIPT_DIR"

# /api 的转发目标是构建期定死的（见 build.sh）。前后端部署在同一台机器、后端用默认端口时不用管；
# 否则打包前带上：APP_API=http://10.0.0.2:10012 ./package.sh
echo "building $APP_NAME (APP_API=${APP_API:-http://127.0.0.1:10012}, BASE_PATH=${BASE_PATH-/assistant})..."
"$SCRIPT_DIR/build.sh"

rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

cp -R .next "$DIST_DIR/.next"
rm -rf "$DIST_DIR/.next/cache"
# next.config.mjs 里开了 output: "standalone"，但这个包按 `next start` 部署
# （见下面生成的 start.sh），.next/standalone 里那份 node_modules 副本只会让 tar 白白变大。
rm -rf "$DIST_DIR/.next/standalone"
[ -d public ] && cp -R public "$DIST_DIR/"

for file in package.json package-lock.json next.config.mjs; do
  [ -f "$file" ] && cp "$file" "$DIST_DIR/"
done

cat > "$DIST_DIR/start.sh" <<'EOF'
#!/bin/sh
set -eu

APP_NAME="pa-app"
PORT="${PORT:-7906}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PID_FILE="$SCRIPT_DIR/$APP_NAME.pid"
LOG_DIR="${LOG_DIR:-$SCRIPT_DIR/logs}"
LOG_FILE="${LOG_FILE:-$LOG_DIR/$APP_NAME.log}"
LOCK_STAMP="$SCRIPT_DIR/node_modules/.pa-lock-sum"

cd "$SCRIPT_DIR"
mkdir -p "$LOG_DIR"

if [ -f "$PID_FILE" ]; then
  PID="$(cat "$PID_FILE")"
  if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
    echo "$APP_NAME is already running, pid: $PID"
    exit 0
  fi
  rm -f "$PID_FILE"
fi

if command -v lsof >/dev/null 2>&1; then
  PORT_PID="$(lsof -ti "TCP:$PORT" -sTCP:LISTEN 2>/dev/null || true)"
  if [ -n "$PORT_PID" ]; then
    echo "port $PORT is already in use by pid: $PORT_PID"
    exit 1
  fi
fi

# 新包解到老目录上时 node_modules 还是上一版的：lockfile 变了才重装，没变就直接用。
LOCK_SUM="$(cksum package-lock.json | awk '{print $1}')"
if [ ! -d node_modules ] || [ "$(cat "$LOCK_STAMP" 2>/dev/null || true)" != "$LOCK_SUM" ]; then
  npm ci --omit=dev
  echo "$LOCK_SUM" > "$LOCK_STAMP"
fi

NEXT_TELEMETRY_DISABLED=1 nohup ./node_modules/.bin/next start -p "$PORT" >> "$LOG_FILE" 2>&1 &
PID="$!"
echo "$PID" > "$PID_FILE"

sleep 2
if kill -0 "$PID" 2>/dev/null; then
  echo "$APP_NAME started, pid: $PID, port: $PORT, log: $LOG_FILE"
else
  rm -f "$PID_FILE"
  echo "$APP_NAME failed to start, see log: $LOG_FILE" >&2
  exit 1
fi
EOF

cat > "$DIST_DIR/stop.sh" <<'EOF'
#!/bin/sh
set -eu

APP_NAME="pa-app"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PID_FILE="$SCRIPT_DIR/$APP_NAME.pid"

if [ ! -f "$PID_FILE" ]; then
  echo "$APP_NAME is not running"
  exit 0
fi

PID="$(cat "$PID_FILE")"
if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
  kill "$PID"
  i=0
  while kill -0 "$PID" 2>/dev/null && [ "$i" -lt 10 ]; do
    sleep 1
    i=$((i + 1))
  done
  if kill -0 "$PID" 2>/dev/null; then
    echo "$APP_NAME did not stop cleanly (pid $PID); escalate with: kill -9 $PID" >&2
    exit 1
  fi
  echo "$APP_NAME stopped, pid: $PID"
else
  echo "$APP_NAME pid file exists, but process is not running"
fi

rm -f "$PID_FILE"
EOF

chmod +x "$DIST_DIR/start.sh" "$DIST_DIR/stop.sh"

tar -czf "$ARCHIVE" -C "$PACKAGE_ROOT" "$PACKAGE_NAME"

echo "package created: $ARCHIVE"
