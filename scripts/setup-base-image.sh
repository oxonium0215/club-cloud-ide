#!/usr/bin/env bash
# ゴールデンベースイメージ作成スクリプト
# イメージは「OS + 開発パッケージ」のみを焼き込み、
# 実行時設定 (xstartup / noVNC / code-server 等) は
# templates/lxd-siv3d/files/ から起動時に配布する (GitOps)。
set -euo pipefail

BASE_NAME="osgsuken-base"
IMAGE_ALIAS="osgsuken-base-img"

# 1. 既存ベースコンテナとイメージをクリーンアップ
lxc delete "$BASE_NAME" --force 2>/dev/null || true
lxc image delete "$IMAGE_ALIAS" 2>/dev/null || true

# 2. Ubuntu 24.04 から初期コンテナを起動
#    security.nesting: コンテナ内でのマウントネームスペース許可
#    security.syscalls.intercept.mknod / .mount: コンテナ内で snapd が
#    (firefox 等の snap パッケージ) を動かすために必要
lxc launch ubuntu:24.04 "$BASE_NAME" \
    -c security.nesting=true \
    -c security.syscalls.intercept.mknod=true \
    -c security.syscalls.intercept.mount=true
sleep 3

# 3. パッケージインストール
lxc exec "$BASE_NAME" -- bash -c '
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive

# WSL2 のネスト仮想化では VM 内ネットワークが不安定で、
# 長時間のダウンロード中に途切れることがある。リトライ付き apt を使用する。
apt_retry() {
    local n=0
    until apt-get install -y "$@"; do
        n=$((n+1))
        if [ "$n" -ge 5 ]; then
            echo "apt-get install が 5 回試行しても失敗しました: $*" >&2
            return 1
        fi
        echo "apt-get install の再試行 ($n/5): $*"
        sleep 10
        apt-get update || true
    done
}

# cloud-init の初回セットアップ完了を待つ (sources.list 生成と競合するため)
for i in $(seq 1 60); do
    if [ -f /var/lib/cloud/instance/boot-finished ] || [ -f /etc/apt/sources.list.d/ubuntu.sources ]; then
        break
    fi
    sleep 2
done

# 日本最速ミラー & 全コンポーネント
if [ -f /etc/apt/sources.list.d/ubuntu.sources ]; then
    sed -i "s@http://archive.ubuntu.com/@http://jp.archive.ubuntu.com/@g" /etc/apt/sources.list.d/ubuntu.sources
    sed -i "s@Components: main.*@Components: main restricted universe multiverse@g" /etc/apt/sources.list.d/ubuntu.sources
else
    sed -i "s@http://archive.ubuntu.com/@http://jp.archive.ubuntu.com/@g" /etc/apt/sources.list
    sed -i "s@main restricted@main restricted universe multiverse@g" /etc/apt/sources.list
fi

# ミラー変更後に update を2回実行し、パッケージリストを確実に揃える
apt-get update
apt-get update

# タイムゾーン & ロケール
ln -fs /usr/share/zoneinfo/Asia/Tokyo /etc/localtime
echo "Asia/Tokyo" > /etc/timezone
apt-get update
apt-get install -y locales
locale-gen ja_JP.UTF-8
update-locale LANG=ja_JP.UTF-8

# 開発環境 (負荷分散のため分割インストール)
# ※ 軽量デスクトップ LXQt を採用。KDE Plasma よりパッケージ数が大幅に少なく、
#   ビルド時間が短縮される。日本語入力は fcitx5 で共通。

# [1/3] 開発ツール (コンパイラ・ユーティリティ)
apt_retry \
    build-essential g++ gdb cmake ninja-build pkg-config git curl wget unzip \
    ca-certificates sudo python3 python3-pip mingw-w64 htop nodejs \
    ssl-cert xauth x11-xkb-utils x11-utils xinit mesa-utils

# [2/3] LXQt デスクトップ (軽量)
# ※ lxqt メタパッケージは firefox / thunderbird (snap移行ラッパー) を依存に含み、
#   ラッパーの install hook が LXD コンテナのマウント制限で失敗するため、
#   個別パッケージで構成する。firefox は Mozilla 公式 deb 版を焼き込む。
apt_retry \
    lxqt-session lxqt-panel lxqt-runner lxqt-globalkeys lxqt-qtplugin \
    lxqt-config lxqt-notificationd lxqt-policykit lxqt-themes \
    openbox pcmanfm-qt qterminal featherpad \
    xdg-utils fonts-noto-cjk fonts-takao-gothic dbus-x11

# firefox (Mozilla 公式 deb 版) を焼き込み、タスクバーからネイティブに近い使用感を提供
# ※ Ubuntu の firefox パッケージは snap 移行ラッパーのため LXD コンテナで失敗する。
#   Mozilla 公式 APT リポジトリの deb 版を使えば snap 不要で普通に動く。
install -d -m 0755 /etc/apt/keyrings
curl -fsSL https://packages.mozilla.org/apt/repo-signing-key.gpg -o /etc/apt/keyrings/packages.mozilla.org.asc
echo "deb [signed-by=/etc/apt/keyrings/packages.mozilla.org.asc] https://packages.mozilla.org/apt mozilla main" > /etc/apt/sources.list.d/mozilla.list
cat > /etc/apt/preferences.d/mozilla-firefox << 'EOF'
Package: firefox*
Pin: origin packages.mozilla.org
Pin-Priority: 1000
EOF
apt-get update
apt_retry firefox

# [3/3] 日本語入力 (Fcitx5 + Mozc)
apt_retry \
    fcitx5 fcitx5-mozc fcitx5-frontend-qt5 fcitx5-frontend-gtk3 fcitx5-config-qt

# code-server (VS Code) をイメージに焼き込み、初回起動の待ち時間をなくす
export HOME=/root
curl -fsSL https://code-server.dev/install.sh | sh

# VNC (TigerVNC サーバー + noVNC Web クライアント)
# ※ KasmVNC はサブパス (サブディレクトリ) でのリバースプロキシに対応しておらず、
#   noVNC は接続先を URL クエリで指定できるため、サブパス公開が容易。
apt_retry tigervnc-standalone-server novnc websockify

# スクリーンセーバーを削除 (VNC で画面がブランクになるのを防ぐ)
# LXQt は xscreensaver を自動起動するため、アンインストールが確実
apt-get remove -y xscreensaver xscreensaver-data xscreensaver-gl 2>/dev/null || true

# snakeoil SSL 証明書 (websockify TLS 用)
make-ssl-cert generate-default-snakeoil --force-overwrite
chmod 0640 /etc/ssl/private/ssl-cert-snakeoil.key
chown root:ssl-cert /etc/ssl/private/ssl-cert-snakeoil.key

# 一般ユーザー osgsuken (UID 1000, sudo NOPASSWD)
userdel -r ubuntu 2>/dev/null || true
useradd -m -s /bin/bash -G sudo,ssl-cert -u 1000 osgsuken
echo "osgsuken ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/osgsuken
chmod 0440 /etc/sudoers.d/osgsuken

# VNC パスワード (VNC プロトコル認証用)
mkdir -p /home/osgsuken/.vnc
echo -e "suken123\nsuken123\n" | vncpasswd -f > /home/osgsuken/.vnc/passwd
chmod 0600 /home/osgsuken/.vnc/passwd

# 詳細設定は apply.sh が起動時に配布する。xstartup だけ空で置いておく
# (vncserver は xstartup の存在を要求するため)
touch /home/osgsuken/.vnc/xstartup
mkdir -p /home/osgsuken/workspace
chown -R osgsuken:osgsuken /home/osgsuken
'

# 5. コンテナ停止 & ゴールデンイメージの公開
lxc stop "$BASE_NAME"
lxc publish "$BASE_NAME" --alias "$IMAGE_ALIAS"
lxc delete "$BASE_NAME" --force 2>/dev/null || true
echo "ゴールデンイメージ $IMAGE_ALIAS を作成しました"
