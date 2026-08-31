#!/usr/bin/env bash
# ゴールデンベースイメージ作成スクリプト
# イメージは「OS + 開発パッケージ」のみを焼き込み、
# 実行時設定 (xstartup / kasmvnc.yaml / code-server 等) は
# templates/lxd-siv3d/files/ から起動時に配布する (GitOps)。
set -euo pipefail

BASE_NAME="osgsuken-base"
IMAGE_ALIAS="osgsuken-base-img"
PACKAGE_DIR="/home/yugo/dev/club-cloud-ide/packages"
KASMVNC_DEB="$PACKAGE_DIR/kasmvncserver_noble_1.5.0_amd64.deb"
KASMVNC_URL="https://github.com/kasmtech/KasmVNC/releases/download/v1.5.0/kasmvncserver_noble_1.5.0_amd64.deb"

# 0. KasmVNC deb の準備 (ローカルにあれば使い、なければダウンロード)
if [ ! -f "$KASMVNC_DEB" ]; then
    mkdir -p "$PACKAGE_DIR"
    curl -fsSL -o "$KASMVNC_DEB" "$KASMVNC_URL"
fi

# 1. 既存ベースコンテナとイメージをクリーンアップ
lxc delete "$BASE_NAME" --force 2>/dev/null || true
lxc image delete "$IMAGE_ALIAS" 2>/dev/null || true

# 2. Ubuntu 24.04 から初期コンテナを起動
lxc launch ubuntu:24.04 "$BASE_NAME" -c security.nesting=true
sleep 3

# 3. KasmVNC deb を転送
lxc file push "$KASMVNC_DEB" "$BASE_NAME/tmp/kasmvncserver.deb"

# 4. パッケージインストール
lxc exec "$BASE_NAME" -- bash -c '
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive

# 日本最速ミラー & 全コンポーネント
if [ -f /etc/apt/sources.list.d/ubuntu.sources ]; then
    sed -i "s@http://archive.ubuntu.com/@http://jp.archive.ubuntu.com/@g" /etc/apt/sources.list.d/ubuntu.sources
    sed -i "s@Components: main.*@Components: main restricted universe multiverse@g" /etc/apt/sources.list.d/ubuntu.sources
else
    sed -i "s@http://archive.ubuntu.com/@http://jp.archive.ubuntu.com/@g" /etc/apt/sources.list
    sed -i "s@main restricted@main restricted universe multiverse@g" /etc/apt/sources.list
fi

# タイムゾーン & ロケール
ln -fs /usr/share/zoneinfo/Asia/Tokyo /etc/localtime
echo "Asia/Tokyo" > /etc/timezone
apt-get update
apt-get install -y --no-install-recommends locales
locale-gen ja_JP.UTF-8
update-locale LANG=ja_JP.UTF-8

# 開発環境 (日本語入力・KDE Plasma・KasmVNC依存)
# ※ kinit (klauncher) / kio-extras / plasma-integration / xdg-utils は
#    タスクバーからのアプリ起動に必須。--no-install-recommends だと欠落する。
apt-get install -y --no-install-recommends \
    build-essential g++ gdb cmake ninja-build pkg-config git curl wget unzip \
    ca-certificates sudo python3 python3-pip mingw-w64 htop nodejs \
    plasma-desktop plasma-workspace breeze breeze-icon-theme qml-module-org-kde-kirigami2 \
    kactivitymanagerd kinit kio kio-extras xdg-utils plasma-integration \
    qml-module-qtquick-controls qml-module-qtquick-controls2 \
    kwin-x11 dolphin konsole \
    fonts-noto-cjk fonts-takao-gothic dbus-x11 libjpeg-turbo8 libvpx9 libwebp7 \
    fcitx5 fcitx5-mozc fcitx5-frontend-qt5 fcitx5-frontend-gtk3 kde-config-fcitx5 \
    ssl-cert xauth x11-xkb-utils x11-utils xinit mesa-utils

# KasmVNC 本体 (依存不足で dpkg -i が失敗しても、apt-get install -f で解決する)
dpkg -i /tmp/kasmvncserver.deb || true
apt-get install -f -y
rm -f /tmp/kasmvncserver.deb

# snakeoil SSL 証明書 (KasmVNC 内部暗号化用)
make-ssl-cert generate-default-snakeoil --force-overwrite
chmod 0640 /etc/ssl/private/ssl-cert-snakeoil.key
chown root:ssl-cert /etc/ssl/private/ssl-cert-snakeoil.key

# 一般ユーザー coder (UID 1000, sudo NOPASSWD)
userdel -r ubuntu 2>/dev/null || true
useradd -m -s /bin/bash -G sudo,ssl-cert -u 1000 coder
echo "coder ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/coder
chmod 0440 /etc/sudoers.d/coder

# KasmVNC パスワード (BasicAuth は -disableBasicAuth で無効化済みだが
# ファイルの存在自体はチェックされるため作成する)
mkdir -p /etc/kasmvnc /home/coder/.vnc /home/coder/.config/kasmvnc
echo -e "suken123\nsuken123\n" | vncpasswd -u coder -o /etc/kasmvnc/kasmpasswd
chmod 0644 /etc/kasmvnc/kasmpasswd
cp /etc/kasmvnc/kasmpasswd /home/coder/.kasmpasswd
cp /etc/kasmvnc/kasmpasswd /home/coder/.vnc/kasmpasswd

# 詳細設定は apply.sh が起動時に配布する。xstartup だけ空で置いておく
# (vncserver は xstartup の存在を要求するため)
touch /home/coder/.vnc/xstartup
mkdir -p /home/coder/workspace
chown -R coder:coder /home/coder
'

# 5. KasmVNC WebUI パッチ (自動ログイン & 安定画像レンダリング)
#    画面描画モード (WebCodecs/WebGL) の互換性問題へのワークアラウンド
cat > /tmp/kasmvnc-ui-patch.js << 'NODE_EOF'
const fs = require("fs");
const file = "/usr/share/kasmvnc/www/assets/ui-BOjwDkC7.js";
if (fs.existsSync(file)) {
    let code = fs.readFileSync(file, "utf8");
    if (code.includes("credentials:{password:e}")) {
        code = code.replace("credentials:{password:e}", "credentials:{username:hr(\"username\")||\"coder\",password:e||hr(\"password\")||\"suken123\"}");
    }
    code = code.replace("o.initSetting(\"fallback_image_mode\",!1),o.initSetting(ht.STREAM_MODE,Ee.pseudoEncodingStreamingModeJpegWebp)", "o.forceSetting(\"fallback_image_mode\",!0,!1),o.forceSetting(ht.STREAM_MODE,Ee.pseudoEncodingStreamingModeJpegWebp,!1),o.forceSetting(\"video_rendering_mode\",\"canvas2d\",!1)");
    fs.writeFileSync(file, code, "utf8");
}
NODE_EOF
lxc file push /tmp/kasmvnc-ui-patch.js "$BASE_NAME/tmp/kasmvnc-ui-patch.js"
lxc exec "$BASE_NAME" -- node /tmp/kasmvnc-ui-patch.js 2>/dev/null || true
rm -f /tmp/kasmvnc-ui-patch.js

# 6. コンテナ停止 & ゴールデンイメージの公開
lxc stop "$BASE_NAME"
lxc publish "$BASE_NAME" --alias "$IMAGE_ALIAS"
lxc delete "$BASE_NAME" --force 2>/dev/null || true
echo "ゴールデンイメージ $IMAGE_ALIAS を作成しました"
