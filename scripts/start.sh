#!/usr/bin/env bash
# 開発機で Coder サーバー + mock-OIDC を起動する (本番は deploy/install.sh を使用)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# 既存プロセスの掃除 (二重起動によるポート衝突を防ぐ)
pkill -f "mock-oidc.js" 2>/dev/null || true
pkill -f "portal.js" 2>/dev/null || true
pkill -f "coder server" 2>/dev/null || true
# Coder の built-in PostgreSQL が孤児として残っていると ポート33977 で衝突する
pkill -f "coderv2/postgres" 2>/dev/null || true
sleep 1

# LXD NAT 修復 (Docker が iptables FORWARD を DROP にする問題への対処)
LXD_BRIDGE="$(lxc network list --format csv 2>/dev/null | cut -d, -f1 | grep '^lxdbr0$' | head -1)"
if [ -n "$LXD_BRIDGE" ]; then
    LXD_NET="$(lxc network get "$LXD_BRIDGE" ipv4.address 2>/dev/null | cut -d/ -f1)"
    sudo iptables -t nat -C POSTROUTING -s "$LXD_NET" ! -d "$LXD_NET" -j MASQUERADE 2>/dev/null || \
        sudo iptables -t nat -A POSTROUTING -s "$LXD_NET" ! -d "$LXD_NET" -j MASQUERADE
    sudo iptables -C FORWARD -i "$LXD_BRIDGE" -o "$LXD_BRIDGE" -j ACCEPT 2>/dev/null || \
        sudo iptables -I FORWARD -i "$LXD_BRIDGE" -o "$LXD_BRIDGE" -j ACCEPT
fi

# mock-OIDC (開発用 SSO)
node "$SCRIPT_DIR/mock-oidc.js" &

# Coder サーバー (coder.yaml を明示指定, ポート7081で待ち受け)
# client_secret は環境変数で注入 (coder.yaml には書かない)
# ポータル (portal.js) が 7080 で受けて Coder へプロキシする
export CODER_HTTP_ADDRESS="0.0.0.0:7081"
export CODER_ACCESS_URL="http://localhost:7081"
export CODER_OIDC_CLIENT_SECRET="school-cloud-secret"
coder server --config "$ROOT_DIR/coder.yaml" &
CODER_PID=$!

# 部員向けポータル (VS Code / デスクトップ の2ボタン入口)
export CODER_URL="http://localhost:7081"
node "$SCRIPT_DIR/portal.js" &

# Ctrl+C で両方停止
trap 'kill $CODER_PID 2>/dev/null' INT TERM
wait $CODER_PID
