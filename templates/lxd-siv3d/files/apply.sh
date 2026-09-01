#!/usr/bin/env bash
# コンテナ内設定の配布スクリプト (GitOps の配布側)
# ゴールデンイメージには OS+パッケージを焼き込み、このスクリプトが
# リポジトリ内の templates/lxd-siv3d/files/ から実行時設定を配布する。
# 設定変更はイメージ再ビルド不要で、再起動時に自動反映される。
# ※ cloud-init の runcmd から root で実行される前提。
set -eu

SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_DIR="${SRC_DIR}"

echo "[apply] 設定を配布中: ${CONFIG_DIR}"

# 配布先ディレクトリの確保
mkdir -p /home/osgsuken/.vnc \
         /home/osgsuken/.config/fcitx5 \
         /home/osgsuken/.config/code-server

# 設定ファイルの配布
install -m 0755 "${CONFIG_DIR}/xstartup"         /home/osgsuken/.vnc/xstartup
install -m 0755 "${CONFIG_DIR}/entrypoint.sh"    /usr/local/bin/entrypoint.sh
install -m 0644 "${CONFIG_DIR}/fcitx5-profile"   /home/osgsuken/.config/fcitx5/profile
install -m 0644 "${CONFIG_DIR}/fcitx5-config"    /home/osgsuken/.config/fcitx5/config
install -m 0644 "${CONFIG_DIR}/code-server.yaml" /home/osgsuken/.config/code-server/config.yaml

# 所有権を osgsuken に戻す
chown -R osgsuken:osgsuken /home/osgsuken/.config /home/osgsuken/.vnc 2>/dev/null || true

echo "[apply] 設定の配布が完了しました"
