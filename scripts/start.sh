#!/usr/bin/env bash
# 開発機で Go ポータル (OIDC SSO + LXD + 2ボタン入口) を起動する
# (本番は deploy/install.sh を使用)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# 既存プロセスの掃除 (二重起動によるポート衝突を防ぐ)
pkill -f "portal-bin" 2>/dev/null || true
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

# Go ポータルのビルド & 起動
cd "$ROOT_DIR/portal"
go build -o portal-bin .

export OIDC_CLIENT_SECRET="osgsuken-portal-secret"
export REPO_URL="https://github.com/oxonium0215/club-cloud-ide.git"
exec ./portal-bin
