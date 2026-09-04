# 本番導入手順 (GitOps)

本番は PC1 (マスター) / PC2 (ワーカー) の 2 台構成。pull 型 GitOps により、GitHub への push だけで全環境に反映される。

## 前提

- LXD クラスタ構築済み (PC1 がリーダー、PC2 が参加)
- ゴールデンイメージ `osgsuken-base-img` が LXD に存在 (作成: `npm run build-image`)
- 開発ツール: `git` / `go` / `node` / `npm` がインストール済み
  - Go: ポータルのビルド用
  - Node.js: フロントエンド (React + kumo) のビルド用

## アーキテクチャ

```
ブラウザ
  │  :7080
  ▼
Caddy (deploy/Caddyfile)          ← リバースプロキシ + 認証チェック
  ├─ /            → ポータル (React + kumo UI)
  ├─ /api/*       → ポータル
  └─ /proxy/<app>/<ip>/* → ユーザーの LXD コンテナ内サービス
       ├─ vscode   → code-server (:13337)
       └─ desktop  → noVNC + TigerVNC (:6080 → :5901)
```

- ポータル (Go) は `:7081` で動作し、Caddy のバックエンドになる
- フロントエンド (React + kumo) は `go:embed` で Go バイナリに埋め込まれる
- コンテナへの転送は Caddy が行い、`forward_auth` で SSO 認証をチェック

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
5. Caddy をインストール (Cloudflare 公式リポジトリ)
6. フロントエンド (React + kumo) をビルド
7. ポータル (Go) をビルドし、systemd サービスとして登録
   - `osgsuken-caddy.service` (Caddy)
   - `osgsuken-portal.service` (Go ポータル)

## 秘密情報の設定

ポータルの systemd サービスに環境変数として注入する (Git には載せない):

```bash
sudo systemctl edit osgsuken-portal.service
# [Service] セクションに追加:
# Environment=OIDC_CLIENT_SECRET=<学校のOIDCクライアントシークレット>
# Environment=OIDC_SIGNING_KEY=<ポータルのJWT署名用RSA秘密鍵(PEM)>
#   ※ 生成: openssl genpkey -algorithm RSA -out signing-key.pem -pkeyopt rsa_keygen_bits:2048
#   ※ ポータルを再起動してもセッションを維持するため、全PCで同じ鍵を使うこと
```

本番では OIDC プロバイダを学校の実 SSO に切り替える。ポータルの起動パラメータで issuer を指定する:

```bash
sudo systemctl edit osgsuken-portal.service
# Environment=OIDC_MOCK=false
# Environment=OIDC_ISSUER=https://accounts.google.com   (例: Google)
# Environment=OIDC_CLIENT_ID=<クライアントID>
# Environment=REDIRECT_URI=http://osgsuken.local:7080/auth/callback
```

`REDIRECT_URI` は deploy/osgsuken-portal.service のデフォルト (http://localhost:7080/auth/callback) を上書きする。

## 設定変更の反映フロー

```
開発機で push ──▶ GitHub ──▶ 本番PC (osgsuken-gitops.timer, 2分間隔)
                                 ├─ portal/ 変更 → フロントエンド + Go を再ビルド & 再起動
                                 ├─ deploy/Caddyfile 変更 → Caddy リロード
                                 └─ templates/ 変更 → コンテナ再作成時 apply.sh が設定配布
```

## トラブル時の手動同期

```bash
sudo /usr/local/bin/osgsuken-gitops-sync.sh
systemctl status osgsuken-gitops.timer
systemctl status osgsuken-caddy.service
systemctl status osgsuken-portal.service
```
