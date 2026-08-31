#!/usr/bin/env bash
# GitOps 同期スクリプト (本番PCで定期実行される)
# リポジトリの更新を pull し、変更に応じて Coder テンプレートを反映する。
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

CHANGED_FILES=$(git diff --name-only "$LOCAL_HASH" "$REMOTE_HASH")

git merge origin/main --ff-only

# 2. Coder テンプレートの反映 (templates/ 変更時のみ)
if echo "$CHANGED_FILES" | grep -q "^templates/"; then
    if command -v coder >/dev/null 2>&1; then
        cd "$REPO_DIR/templates/lxd-siv3d"
        coder templates push lxd-kde-siv3d --yes
    fi
fi

echo "[GitOps] $(date +%FT%T) 同期完了: $REMOTE_HASH"
