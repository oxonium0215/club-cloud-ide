#!/usr/bin/env bash
# 本番PC (PC1/PC2) への導入スクリプト
# 使い方: 本番PCで `git clone <repo> && cd club-cloud-ide && sudo ./deploy/install.sh`
# 前提: LXD クラスタとゴールデンイメージ osgsuken-base-img がセットアップ済み
set -euo pipefail

REPO_DIR="/opt/club-cloud-ide"
GITHUB_URL="${GITHUB_URL:?GITHUB_URL を設定してください (例: https://github.com/oxonium0215/club-cloud-ide.git)}"

# 1. リポジトリ配置 (GitOps の同期元)
mkdir -p /opt
if [ ! -d "$REPO_DIR/.git" ]; then
    git clone "$GITHUB_URL" "$REPO_DIR"
else
    git -C "$REPO_DIR" fetch origin main
    git -C "$REPO_DIR" merge origin/main --ff-only
fi

# 2. 同期スクリプトを配置
install -m 0755 "$REPO_DIR/deploy/gitops-sync.sh" /usr/local/bin/osgsuken-gitops-sync.sh

# 3. systemd timer 登録 (2分間隔で同期)
cp "$REPO_DIR/deploy/osgsuken-gitops.service" /etc/systemd/system/
cp "$REPO_DIR/deploy/osgsuken-gitops.timer" /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now osgsuken-gitops.timer

# 4. ゴールデンイメージの確認
if ! lxc image list osgsuken-base-img --format csv 2>/dev/null | grep -q "osgsuken-base-img"; then
    echo "⚠️ ゴールデンイメージ osgsuken-base-img が見つかりません。"
    echo "   開発機で scripts/setup-base-image.sh を実行してイメージを作成してください。"
    exit 1
fi

# 5. Caddy のインストール (リバースプロキシ)
if ! command -v caddy >/dev/null 2>&1; then
    echo "Caddy をインストール中..."
    apt-get update
    apt-get install -y debian-keyring debian-archive-keyring apt-transport-https curl
    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | tee /etc/apt/sources.list.d/caddy-stable.list
    apt-get update
    apt-get install -y caddy
fi

# 6. ポータル (Go) のビルド & systemd サービス登録
cd "$REPO_DIR/portal"
go build -o portal-bin .

install -d /opt/club-cloud-ide/bin
install -m 0755 "$REPO_DIR/portal/portal-bin" /opt/club-cloud-ide/bin/osgsuken-portal

cp "$REPO_DIR/deploy/osgsuken-portal.service" /etc/systemd/system/
cp "$REPO_DIR/deploy/osgsuken-caddy.service" /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now osgsuken-caddy.service
systemctl enable --now osgsuken-portal.service

echo "導入が完了しました。"
echo "  - ポータル:  http://osgsuken.local:7080  (VS Code / デスクトップ の2ボタン入口)"
echo "  - osgsuken-gitops.timer が2分間隔で同期します。"
