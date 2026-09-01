#!/usr/bin/env bash
# 開発環境のクリーンアップ (コンテナとプロセスの停止)
set -euo pipefail

pkill -f "portal-bin" 2>/dev/null || true

for c in $(lxc list --format csv -c n 2>/dev/null || true); do
    lxc delete "$c" --force 2>/dev/null || true
done
