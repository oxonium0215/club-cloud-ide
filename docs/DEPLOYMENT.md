# 本番導入手順 (GitOps)

本番は PC1 (マスター) / PC2 (ワーカー) の 2 台構成。pull 型 GitOps により、GitHub への push だけで全環境に反映される。

## 前提

- LXD クラスタ構築済み (PC1 がリーダー、PC2 が参加)
- Coder バイナリがインストール済み
- ゴールデンイメージ `osgsuken-base-img` が LXD に存在 (作成: `npm run build-image`)

## 導入

各本番PCで:

```bash
sudo mkdir -p /etc/coder
sudo install -m 0600 /dev/null /etc/coder/env   # 秘密情報 (後述)

git clone <repo-url> /opt/club-cloud-ide
cd /opt/club-cloud-ide
sudo GITHUB_URL=<repo-url> ./deploy/install.sh
```

`install.sh` が行うこと:

1. リポジトリを `/opt/club-cloud-ide` に配置
2. `deploy/gitops-sync.sh` を `/usr/local/bin` に配置
3. systemd timer `osgsuken-gitops.timer` を登録 (2分間隔で同期)
4. Coder テンプレート `lxd-kde-siv3d` を push / create

## 秘密情報の設定

`/etc/coder/env` に環境変数として置く (Git には載せない):

```bash
sudo tee -a /etc/coder/env <<'EOF'
CODER_OIDC_CLIENT_SECRET=<学校のOIDCクライアントシークレット>
EOF
```

Coder サービスはこのファイルを読み込むよう systemd 側で設定する (`EnvironmentFile=/etc/coder/env`)。

## 本番の coder.yaml

`coder.yaml` の `oidc.issuerURL` を学校の実プロバイダに書き換える:

```yaml
oidc:
  issuerURL: "https://accounts.google.com"   # 例: Google
  clientID: "<クライアントID>"
  emailDomain:
    - "school.ed.jp"
```

## 設定変更の反映フロー

```
開発機で push ──▶ GitHub ──▶ 本番PC (osgsuken-gitops.timer, 2分間隔)
                                 ├─ templates/ 変更 → coder templates push
                                 └─ ワークスペース再起動 → apply.sh が設定配布
```

## トラブル時の手動同期

```bash
sudo /usr/local/bin/osgsuken-gitops-sync.sh
systemctl status osgsuken-gitops.timer
```
