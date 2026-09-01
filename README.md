# 数学研究部 クラウド開発環境 (Club Cloud IDE)

iPad を所有する部員に対して、複数台の型落ちデスクトップPCのリソースを自動的に割り当てる開発環境システム。

- ブラウザ上の **VS Code (code-server)** と **LXQt デスクトップ (KasmVNC)** を提供
- 学校アカウント (SSO / OIDC) による認証と、一人一台の専用 Linux 環境 (LXD) の自動プロビジョニング
- 最終アクティビティから 60 分で自動シャットダウン (リソース自動解放)

## 構成

```
[ポータル (Go)] 2ボタン入口 (VS Code / デスクトップ) :7080
   │  OIDC SSO 認証 + LXD 直接管理 + リバースプロキシ
   └── [LXDクラスタ] 一人一台の専用コンテナ (完全永続化)
         ├── code-server     (VS Code, :13337)
         └── KasmVNC         (LXQt デスクトップ, :6080)
```

詳細な仕様・運用は [`docs/`](docs/) を参照。

| ドキュメント | 内容 |
| :--- | :--- |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | システム設計・技術選定 |
| [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) | 本番導入手順 (GitOps) |
| [`docs/TROUBLESHOOTING.md`](docs/TROUBLESHOOTING.md) | トラブルシューティング |

## クイックスタート (開発機)

```bash
# ゴールデンベースイメージの作成 (LXQt + KasmVNC 入り)
npm run build-image

# 開発環境の起動 (Go ポータル + 内蔵 mock-OIDC)
npm start

# ブラウザで開く
open http://localhost:7080
```

## 本番導入

本番PC (PC1/PC2) では pull 型 GitOps により自動同期される。

```bash
git clone https://github.com/oxonium0215/club-cloud-ide.git && cd club-cloud-ide
sudo GITHUB_URL=https://github.com/oxonium0215/club-cloud-ide.git ./deploy/install.sh
```

詳しくは [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) を参照。

## クリーンアップ

```bash
npm run cleanup
```
