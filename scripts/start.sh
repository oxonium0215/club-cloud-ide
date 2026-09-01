#!/usr/bin/env bash
# 開発機でポータル (OIDC SSO + LXD + Caddy プロキシ) を起動する
# (本番は deploy/install.sh を使用)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# 既存プロセスの掃除 (二重起動によるポート衝突を防ぐ)
pkill -f "portal-bin" 2>/dev/null || true
pkill -f "caddy run" 2>/dev/null || true
sleep 1

# LXD NAT 修復 (Docker が iptables FORWARD を DROP にする問題への対処)
LXD_BRIDGE="$(lxc network list --format csv 2>/dev/null | cut -d, -f1 | grep '^lxdbr0$' | head -1)"
if [ -n "$LXD_BRIDGE" ]; then
    LXD_NET="$(lxc network get "$LXD_BRIDGE" ipv4.address 2>/dev/null | cut -d/ -f1)"
    # Docker が iptables FORWARD を DROP にすると LXD コンテナの外部通信が遮断される。
    # nft テーブル (デフォルトの iptables) に LXD 用ルールを明示追加する。
    sudo iptables -t nat -C POSTROUTING -s "$LXD_NET" ! -d "$LXD_NET" -j MASQUERADE 2>/dev/null || \
        sudo iptables -t nat -A POSTROUTING -s "$LXD_NET" ! -d "$LXD_NET" -j MASQUERADE
    sudo iptables -C FORWARD -i "$LXD_BRIDGE" -o "$LXD_BRIDGE" -j ACCEPT 2>/dev/null || \
        sudo iptables -I FORWARD -i "$LXD_BRIDGE" -o "$LXD_BRIDGE" -j ACCEPT
    sudo iptables -C FORWARD -i "$LXD_BRIDGE" -o eth0 -j ACCEPT 2>/dev/null || \
        sudo iptables -I FORWARD -i "$LXD_BRIDGE" -o eth0 -j ACCEPT
    sudo iptables -C FORWARD -i eth0 -o "$LXD_BRIDGE" -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || \
        sudo iptables -I FORWARD -i eth0 -o "$LXD_BRIDGE" -m state --state RELATED,ESTABLISHED -j ACCEPT
fi

# Caddy の存在確認
if ! command -v caddy >/dev/null 2>&1; then
    echo "エラー: Caddy がインストールされていません。" >&2
    echo "  https://caddyserver.com/docs/install を参照してインストールしてください。" >&2
    exit 1
fi

# Caddy の設定検証
caddy validate --config "$ROOT_DIR/deploy/Caddyfile" >/dev/null 2>&1 || {
    echo "エラー: Caddyfile が不正です。" >&2
    caddy validate --config "$ROOT_DIR/deploy/Caddyfile" >&2
    exit 1
}

# Caddy を :7080 で起動 (ポータルのフロント)
caddy run --config "$ROOT_DIR/deploy/Caddyfile" &
CADDY_PID=$!

# Go ポータルを :7081 で起動 (Caddy のバックエンド)
cd "$ROOT_DIR/portal"

# フロントエンド (React + kumo) をビルドして dist/ に配置 (go:embed 用)
if [ -d web ]; then
    echo "フロントエンドをビルド中..."
    (cd web && NODE_ENV=development npm run build)
    rm -rf dist
    cp -r web/dist dist
fi

go build -o portal-bin .

export PORTAL_ADDR=":7081"
export REDIRECT_URI="http://localhost:7080/auth/callback"
export OIDC_CLIENT_SECRET="osgsuken-portal-secret"
export REPO_URL="https://github.com/oxonium0215/club-cloud-ide.git"
./portal-bin &
PORTAL_PID=$!

# どちらかが終了したら両方止める
wait -n "$CADDY_PID" "$PORTAL_PID"
kill "$CADDY_PID" "$PORTAL_PID" 2>/dev/null || true
