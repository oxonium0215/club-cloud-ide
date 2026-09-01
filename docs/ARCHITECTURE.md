# システム設計

## 概要

部員の iPad ブラウザから、学校アカウント (SSO) で専用の Linux 開発環境へアクセスする。

| 役割 | 技術 | 概要 |
| :--- | :--- | :--- |
| ポータル | Go (portal/) | OIDC認証・LXD直接管理・リバースプロキシ・アイドル自動停止を1バイナリで提供 |
| 仮想化基盤 | LXDクラスタ | PC1 (マスター) / PC2 (ワーカー) にコンテナを分散配置。OSごと永続化 |
| ブラウザエディタ | code-server | ポート 13337 |
| デスクトップGUI | LXQt / KasmVNC | WebRTC / H.264 による 60fps ストリーミング。ポート 6080 |
| フレームワーク | Siv3D | C++ ゲーム制作。WebAssembly / Windows exe を同一ソースから出力 |

## 構成図

```
[部員の iPad / ブラウザ]
        │ HTTP (SSO)
        ▼
[ポータル (Go)]  http://osgsuken.local:7080
  │  OIDC 認証 (ポート 8090) + 2ボタン入口
  │  コンテナの作成/起動/停止を LXD API で直接制御
  │  /proxy/ でコンテナ内サービスへリバースプロキシ
  ▼
[LXD コンテナ osgsuken-<user>]
  ├── code-server  (VS Code)   :13337
  └── KasmVNC      (LXQt) :6080
  - ゴールデンイメージ osgsuken-base-img から起動
  - コンテナ内ユーザー: osgsuken (/home/osgsuken)
  - 設定は起動時に Git リポジトリから配布 (焼き込みしない)
```

## ユーザーフロー

1. 部員が `http://osgsuken.local:7080` を開く → ポータル
2. 「学校アカウントでログイン」→ OIDC (開発時は mock、本番は実 SSO)
3. 2ボタン画面 (VS Code / デスクトップ) が表示される
4. ボタン押下時、コンテナの状態に応じて:
   - **ない** → ロード画面 → 自動作成 → 起動 → 遷移
   - **停止中** → ロード画面 → 自動起動 → 遷移
   - **起動中** → そのまま遷移
5. 最終アクティビティから60分で自動シャットダウン (heartbeat + アイドル監視)

## 設定配布の設計 (GitOps)

ゴールデンイメージには **OS とパッケージのみ**を焼き込み、実行時設定は配布する。

- 設定ファイルは `templates/lxd-siv3d/files/` に実体として置く
- コンテナ作成時、cloud-init がリポジトリを clone して `apply.sh` を実行
- `apply.sh` が設定を配布し、`entrypoint.sh` が code-server / KasmVNC を起動
- 設定変更はイメージ再ビルド不要で、コンテナ再作成時に自動反映される

```
templates/lxd-siv3d/
└── files/           # 配布する設定ファイル群
    ├── apply.sh         # 配布スクリプト (install で配置)
    ├── entrypoint.sh    # code-server / KasmVNC 起動
    ├── xstartup         # LXQt セッション起動 (クラッシュ時自動再起動)
    ├── kasmvnc.yaml     # KasmVNC 設定
    ├── fcitx5-profile   # 日本語入力 (Mozc)
    └── ...              # デスクトップ設定
```

## セッション安定化

タスクバーからアプリを起動するとデスクトップセッションが落ちる問題への対策 (KDE 時代の教訓):

- `XDG_RUNTIME_DIR` (`/run/user/1000`) を xstartup / entrypoint で自動作成
- `xstartup` は `exec` ではなく監視ループで再起動 (セッションが落ちても Xvnc は維持)
- LXD の `processes` 制限を 1024 に引き上げ
- 軽量デスクトップ LXQt を採用し、リソース消費とビルド時間を削減

## 本番/開発の構成差

| 項目 | 開発機 | 本番 (PC1/PC2) |
| :--- | :--- | :--- |
| OIDC | mock (ポータル内蔵) | 学校の実 SSO (Google/Microsoft) |
| ポータル | `npm start` (Go ビルド) | systemd 管理 |
| 同期 | 手動 | GitOps (systemd timer, 2分間隔) |
| 秘密情報 | 不要 | 環境変数で注入 |
