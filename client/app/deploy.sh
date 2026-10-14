#!/bin/sh
set -eu

# 在**服务器**上换一版 pa-app：stop → 解包 → start，一条命令。
# 把 tar.gz、unpack-release.sh 和这个脚本放同一个目录，直接跑 ./deploy.sh。
#
# 做的就是手工那三步，顺序一模一样，只是不用自己敲、也不会敲错顺序：
#   sh <包名>/stop.sh
#   sh unpack-release.sh
#   sh <包名>/start.sh
#
# 必须先停再解包：unpack-release.sh 是覆盖式解包，新的 .next 直接铺在老目录上。
# 老进程还活着的话，它内存里的 BUILD_ID 和磁盘上的 chunk 对不上，页面会一片
# 404 / ChunkLoadError，直到重启 —— 而那段时间里看着像是「新版本坏了」。
export LC_ALL=C
export LANG=C

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ARCHIVE="${1:-}"

if [ -z "$ARCHIVE" ]; then
  ARCHIVE="$(find "$SCRIPT_DIR" -maxdepth 1 -type f -name 'pa-app-*.tar.gz' | sort | sed -n '1p')"
fi
if [ -z "$ARCHIVE" ] || [ ! -f "$ARCHIVE" ]; then
  echo "Usage: $0 [archive.tar.gz]" >&2
  echo "把 tar.gz 放进 $SCRIPT_DIR，或者把路径当参数给我。" >&2
  exit 1
fi

if [ ! -f "$SCRIPT_DIR/unpack-release.sh" ]; then
  echo "同一个目录里没有 unpack-release.sh：$SCRIPT_DIR" >&2
  exit 1
fi

# 要停、要起的是哪个目录，从归档清单里读，不写死包名。
#
# 顺手把包验了：tar 的退出码得单独接住，不能写成 `tar | sed` —— 那样拿到的是
# sed 的退出码。半截的包 tar 会把读得到的那部分照常列出来再报错，吞掉退出码
# 就会带着一份「看着很正常」的清单往下走，等于为了一个坏包白停一次服务。
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/pa-deploy.XXXXXX")"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

if ! tar -tzf "$ARCHIVE" >"$TMP_DIR/entries.txt" 2>"$TMP_DIR/tar-error.txt"; then
  echo "归档读不完整（传坏了？）：$ARCHIVE" >&2
  sed -n '1,3p' "$TMP_DIR/tar-error.txt" >&2
  echo "服务没动过，还在跑。" >&2
  exit 1
fi
# 顶层目录只认「底下还挂着东西的那一个」：先留下带 / 的条目，再取第一段。
#
# 不能把所有顶层条目排个序取第一个。macOS 打的包里会混进 AppleDouble 伴随文件
# `._<包名>` —— 它是**文件**不是目录，而 C 序下 `.`(0x2E) 排在字母前面，取第一个
# 就取到它。那样 RELEASE_DIR 成了 .../._<包名>，那儿没有 stop.sh，deploy 会当成
# 首次部署把「停服务」整步跳过，新文件就铺到了还在跑的老进程脚下。
RELEASE_NAME="$(sed -e 's|^\./||' -e '/^$/d' "$TMP_DIR/entries.txt" \
  | grep '/' \
  | sed -e 's|/.*$||' \
  | grep -v '^\._' \
  | sort -u)"
if [ -z "$RELEASE_NAME" ]; then
  echo "归档是空的，或者里面根本没有顶层目录：$ARCHIVE" >&2
  exit 1
fi
if [ "$(printf '%s\n' "$RELEASE_NAME" | wc -l | tr -d ' ')" != "1" ]; then
  echo "归档里不止一个顶层目录，不知道该停哪个：$ARCHIVE" >&2
  printf '%s\n' "$RELEASE_NAME" >&2
  exit 1
fi
RELEASE_DIR="$SCRIPT_DIR/$RELEASE_NAME"

echo "==> 1/3 停服务"
STOPPED=0
if [ -f "$RELEASE_DIR/stop.sh" ]; then
  if ! sh "$RELEASE_DIR/stop.sh"; then
    echo "没停下来，什么都没动 —— 解包不能压着活着的进程做。" >&2
    exit 1
  fi
  STOPPED=1
else
  echo "$RELEASE_DIR 还不存在，首次部署，跳过"
fi

echo "==> 2/3 解包"
if ! sh "$SCRIPT_DIR/unpack-release.sh" "$ARCHIVE"; then
  echo "" >&2
  if [ "$STOPPED" = "1" ]; then
    echo "解包失败，服务现在是停着的。" >&2
  else
    # 这一支意味着上面那步认定 $RELEASE_DIR 不存在。真是首次部署就没进程可停；
    # 要是明明部署过却走到这儿，说明包名取错了 —— 先核对 $RELEASE_DIR 这个名字。
    echo "解包失败。停服务那步跳过了（当时判定 $RELEASE_DIR 不存在），老进程还在跑。" >&2
  fi
  # 覆盖式解包没有上一版可退：解到一半的目录里新旧文件混着，直接起老版本也不保险。
  echo "目录里现在可能是新旧文件混着的，换个好包重跑 $0。" >&2
  exit 1
fi

echo "==> 3/3 起服务"
# 首次起会 npm ci --omit=dev，之后只在 lockfile 变了才重装，所以第一次会慢一些。
if ! sh "$RELEASE_DIR/start.sh"; then
  echo "" >&2
  echo "新版本没起来，日志在 $RELEASE_DIR/logs/" >&2
  exit 1
fi
