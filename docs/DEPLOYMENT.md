# 本番導入手順 (GitOps)

本番は PC1 (マスター) / PC2 (ワーカー) の 2 台構成。pull 型 GitOps により、GitHub への push だけで全環境に反映される。

## 前提

- LXD クラスタ構築済み (PC1 がリーダー、PC2 が参加)
- Go 1.26 以上がインストール済み (ポータルのビルドに使用)
- ゴールデンイメージ `osgsuken-base-img` が LXD に存在 (作成: `npm run build-image`)

## 導入

各本番PCで:

```bash
git clone https://github.com/oxonium0215/club-cloud-ide.git /opt/club-cloud-ide
cd /opt/club-cloud-ide
sudo GITHUB_URL=https://github.com/oxonium0215/club-cloud-ide.git ./deploy/install.sh
```

`install.sh` が行うこと:

1. リポジトリを `/opt/club-cloud-ide` に配置
2. `deploy/gitops-sync.sh` を `/usr/local/bin` に配置
3. systemd timer `osgsuken-gitops.timer` を登録 (2分間隔で同期)
4. ゴールデンイメージの存在確認
5. ポータル (Go) をビルドし、systemd サービス `osgsuken-portal.service` として登録

## 秘密情報の設定

ポータルの systemd サービスに環境変数として注入する (Git には載せない):

```bash
sudo systemctl edit osgsuken-portal.service
# [Service] セクションに追加:
# Environment=OIDC_CLIENT_SECRET=<学校のOIDCクライアントシークレット>
```

本番では OIDC プロバイダを学校の実 SSO に切り替える。ポータルの起動パラメータで issuer を指定する:

- `OIDC_MOCK=false`
- `OIDC_ISSUER=https://accounts.google.com` (例: Google)
- `OIDC_CLIENT_ID=<クライアントID>`

## 設定変更の反映フロー

```
開発機で push ──▶ GitHub ──▶ 本番PC (osgsuken-gitops.timer, 2分間隔)
                                 ├─ portal/ 変更 → ポータル再ビルド & 再起動
                                 └─ templates/ 変更 → コンテナ再作成時 apply.sh が設定配布
```

## トラブル時の手動同期

```bash
sudo /usr/local/bin/osgsuken-gitops-sync.sh
systemctl status osgsuken-gitops.timer
```
