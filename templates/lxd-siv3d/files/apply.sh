#!/usr/bin/env bash
# コンテナ内設定の配布スクリプト (GitOps の配布側)
# ゴールデンイメージには設定を焼き込まず、このスクリプトが
# リポジトリ内の templates/lxd-siv3d/files/ から配布する。
# 設定変更はイメージ再ビルド不要で、再起動時に自動反映される。
# ※ startup_script から sudo で実行される前提 (coder は NOPASSWD sudo)。
set -eu

SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_DIR="${SRC_DIR}"

echo "[apply] 設定を配布中: ${CONFIG_DIR}"

# 配布先ディレクトリの確保
mkdir -p /home/coder/.vnc \
         /home/coder/.config/kasmvnc \
         /home/coder/.config/fcitx5 \
         /home/coder/.config/code-server \
         /home/coder/.local/share/konsole

# 設定ファイルの配布
install -m 0755 "${CONFIG_DIR}/xstartup"         /home/coder/.vnc/xstartup
install -m 0755 "${CONFIG_DIR}/entrypoint.sh"    /usr/local/bin/entrypoint.sh
install -m 0644 "${CONFIG_DIR}/kasmvnc.yaml"     /home/coder/.config/kasmvnc/kasmvnc.yaml
install -m 0644 "${CONFIG_DIR}/kasmvnc.yaml"     /home/coder/.vnc/kasmvnc.yaml
install -m 0644 "${CONFIG_DIR}/fcitx5-profile"   /home/coder/.config/fcitx5/profile
install -m 0644 "${CONFIG_DIR}/fcitx5-config"    /home/coder/.config/fcitx5/config
install -m 0644 "${CONFIG_DIR}/code-server.yaml" /home/coder/.config/code-server/config.yaml
install -m 0644 "${CONFIG_DIR}/startkderc"       /home/coder/.config/startkderc
install -m 0644 "${CONFIG_DIR}/ksplashrc"        /home/coder/.config/ksplashrc
install -m 0644 "${CONFIG_DIR}/kdeglobals"       /home/coder/.config/kdeglobals
install -m 0644 "${CONFIG_DIR}/kwinrc"           /home/coder/.config/kwinrc
install -m 0644 "${CONFIG_DIR}/konsolerc"        /home/coder/.config/konsolerc
install -m 0644 "${CONFIG_DIR}/profile1.profile" "/home/coder/.local/share/konsole/Profile 1.profile"

# code-server が未導入ならインストール (バージョンはリポジトリ側で追跡)
# ※ インストールスクリプトは $HOME を参照するため、明示的に設定する
if ! command -v code-server >/dev/null 2>&1; then
    echo "[apply] code-server をインストール中..."
    export HOME=/root
    curl -fsSL https://code-server.dev/install.sh | sh
fi

# 所有権を coder に戻す
chown -R coder:coder /home/coder/.config /home/coder/.local /home/coder/.vnc 2>/dev/null || true

echo "[apply] 設定の配布が完了しました"
