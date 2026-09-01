#!/usr/bin/env bash
# GitOps 同期スクリプト (本番PCで定期実行される)
# リポジトリの更新を pull する。コンテナ内の設定は起動時に pull されるため、
# ここではリポジトリの更新のみ行う。
# systemd timer (osgsuken-gitops.timer) から 2分間隔で呼ばれる。
set -euo pipefail

REPO_DIR="/opt/club-cloud-ide"
LOCK_FILE="/tmp/osgsuken-gitops.lock"

# 排他ロック (同時実行防止)
exec 9>"$LOCK_FILE"
flock -n 9 || exit 0

cd "$REPO_DIR"

# 1. リモートの更新確認
git fetch origin main >/dev/null 2>&1 || exit 0

LOCAL_HASH=$(git rev-parse HEAD)
REMOTE_HASH=$(git rev-parse origin/main)

if [ "$LOCAL_HASH" = "$REMOTE_HASH" ]; then
    exit 0
fi

# 2. 最新コミットを取得
git merge origin/main --ff-only

# 3. ポータル (Go) が更新されていれば再ビルド & 再起動
CHANGED_FILES=$(git diff --name-only "$LOCAL_HASH" "$REMOTE_HASH")
if echo "$CHANGED_FILES" | grep -q "^portal/"; then
    cd "$REPO_DIR/portal"
    go build -o portal-bin .
    install -m 0755 portal-bin /opt/club-cloud-ide/bin/osgsuken-portal
    systemctl restart osgsuken-portal.service 2>/dev/null || true
fi

# 4. Caddyfile が更新されていれば Caddy をリロード
if echo "$CHANGED_FILES" | grep -q "^deploy/Caddyfile"; then
    caddy validate --config "$REPO_DIR/deploy/Caddyfile" >/dev/null 2>&1 && \
        systemctl reload osgsuken-caddy.service 2>/dev/null || true
fi

# 5. LXD NAT 修復 (Docker 等が iptables FORWARD を DROP にした場合の復旧)
LXD_BRIDGE="$(lxc network list --format csv 2>/dev/null | cut -d, -f1 | grep '^lxdbr0$' | head -1)"
if [ -n "$LXD_BRIDGE" ]; then
    LXD_NET="$(lxc network get "$LXD_BRIDGE" ipv4.address 2>/dev/null | cut -d/ -f1)"
    iptables -t nat -C POSTROUTING -s "$LXD_NET" ! -d "$LXD_NET" -j MASQUERADE 2>/dev/null || \
        iptables -t nat -A POSTROUTING -s "$LXD_NET" ! -d "$LXD_NET" -j MASQUERADE
    iptables -C FORWARD -i "$LXD_BRIDGE" -o "$LXD_BRIDGE" -j ACCEPT 2>/dev/null || \
        iptables -I FORWARD -i "$LXD_BRIDGE" -o "$LXD_BRIDGE" -j ACCEPT
    iptables -C FORWARD -i "$LXD_BRIDGE" -o eth0 -j ACCEPT 2>/dev/null || \
        iptables -I FORWARD -i "$LXD_BRIDGE" -o eth0 -j ACCEPT
    iptables -C FORWARD -i eth0 -o "$LXD_BRIDGE" -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || \
        iptables -I FORWARD -i eth0 -o "$LXD_BRIDGE" -m state --state RELATED,ESTABLISHED -j ACCEPT
fi

echo "[GitOps] $(date +%FT%T) 同期完了: $REMOTE_HASH"
