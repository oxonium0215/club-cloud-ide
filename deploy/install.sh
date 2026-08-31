#!/usr/bin/env bash
# 本番PC (PC1/PC2) への導入スクリプト
# 使い方: 本番PCで `git clone <repo> && cd club-cloud-ide && sudo ./deploy/install.sh`
# 前提: LXD クラスタと Coder がセットアップ済みであること (docs/DEPLOYMENT.md 参照)
set -euo pipefail

REPO_DIR="/opt/club-cloud-ide"
GITHUB_URL="${GITHUB_URL:?GITHUB_URL を設定してください (例: https://github.com/oxonium0215/club-cloud-ide.git)}"
CODER_TEMPLATE_NAME="lxd-kde-siv3d"

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

# 4. Coder テンプレート登録
if command -v coder >/dev/null 2>&1; then
    cd "$REPO_DIR/templates/lxd-siv3d"
    if coder templates list 2>/dev/null | grep -q "$CODER_TEMPLATE_NAME"; then
        coder templates push "$CODER_TEMPLATE_NAME" --yes
    else
        coder templates create "$CODER_TEMPLATE_NAME" --yes
    fi
fi

# 5. Coder 設定の配置 (client_secret 等の秘密は /etc/coder/env に置く)
install -d /etc/coder
install -m 0644 "$REPO_DIR/coder.yaml" /etc/coder/coder.yaml
# 本番では Coder を 7081 で待ち受け、ポータル (7080) が入口になる
sed -i 's@accessURL: "http://localhost:7081"@accessURL: "http://osgsuken.local:7081"@' /etc/coder/coder.yaml 2>/dev/null || true
sed -i 's@httpAddress: "0.0.0.0:7081"@httpAddress: "0.0.0.0:7081"@' /etc/coder/coder.yaml 2>/dev/null || true
echo "Coder 設定を /etc/coder/coder.yaml に配置しました。CODER_OIDC_CLIENT_SECRET 等は /etc/coder/env で設定してください。"

# 6. Coder サーバー & ポータルの systemd サービス登録
cp "$REPO_DIR/deploy/osgsuken-coder.service" /etc/systemd/system/
cp "$REPO_DIR/deploy/osgsuken-portal.service" /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now osgsuken-coder.service
systemctl enable --now osgsuken-portal.service

echo "導入が完了しました。"
echo "  - ポータル:  http://osgsuken.local:7080  (VS Code / デスクトップ の2ボタン入口)"
echo "  - osgsuken-gitops.timer が2分間隔で同期します。"
