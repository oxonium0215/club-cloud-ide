# システム設計

## 概要

部員の iPad ブラウザから、学校アカウント (SSO) で専用の Linux 開発環境へアクセスする。

| 役割 | 技術 | 概要 |
| :--- | :--- | :--- |
| 開発ポータル | Coder | ワークスペースの起動/停止管理。OIDC認証・アイドル検知 |
| 仮想化基盤 | LXDクラスタ | PC1 (マスター) / PC2 (ワーカー) にコンテナを分散配置。OSごと永続化 |
| ブラウザエディタ | code-server | ポート 13337 |
| デスクトップGUI | KDE Plasma / KasmVNC | WebRTC / H.264 による 60fps ストリーミング。ポート 6080 |
| フレームワーク | Siv3D | C++ ゲーム制作。WebAssembly / Windows exe を同一ソースから出力 |

## 構成図

```
[部員の iPad / ブラウザ]
        │ HTTP
        ▼
[ポータル (portal.js)]  http://osgsuken.local:7080
  │  「VS Code」/「デスクトップ」の2ボタン入口
  │  その他のパスは Coder へプロキシ
  ▼
[Coder サーバー]  http://localhost:7081
  ├── code-server  (VS Code)   :13337  ──┐
  └── KasmVNC      (KDE Plasma) :6080  ──┤
                                          ▼
                          [LXD コンテナ osgsuken-<user>]
                          - ゴールデンイメージ osgsuken-base-img から起動
                          - 設定は起動時に Git リポジトリから配布 (焼き込みしない)
```

## 設定配布の設計 (GitOps)

ゴールデンイメージには **OS とパッケージのみ**を焼き込み、実行時設定は配布する。

- 設定ファイルは `templates/lxd-siv3d/files/` に実体として置く
- Coder テンプレートの `coder_file` がワークスペースへ転送し、`startup_script` が `apply.sh` を実行して配布
- 設定変更はイメージ再ビルド不要で、ワークスペース再起動時に自動反映される

```
templates/lxd-siv3d/
├── main.tf          # Coder テンプレート (coder_file で設定転送)
└── files/           # 配布する設定ファイル群
    ├── apply.sh         # 配布スクリプト (install で配置)
    ├── entrypoint.sh    # code-server / KasmVNC 起動
    ├── xstartup         # KDE Plasma セッション起動 (クラッシュ時自動再起動)
    ├── kasmvnc.yaml     # KasmVNC 設定
    ├── fcitx5-profile   # 日本語入力 (Mozc)
    └── ...              # KDE 設定 (kwinrc, kdeglobals 等)
```

## セッション安定化

タスクバーからアプリを起動すると KDE セッションが落ちる問題への対策:

- 必須パッケージ (`kinit` / `kio-extras` / `breeze` / `plasma-integration` / `xdg-utils`) を明示導入
  (`--no-install-recommends` だと欠落する)
- `XDG_RUNTIME_DIR` (`/run/user/1000`) を xstartup / entrypoint で自動作成
- `xstartup` は `exec startplasma-x11` ではなく監視ループで再起動 (セッションが落ちても Xvnc は維持)
- LXD の `processes` 制限を 1024 に引き上げ (KDE は 200 プロセス前後消費)

## 本番/開発の構成差

| 項目 | 開発機 | 本番 (PC1/PC2) |
| :--- | :--- | :--- |
| OIDC | mock-oidc.js (:8090) | 学校の実 SSO (Google/Microsoft) |
| ポータル | `npm start` (Coder 起動) | systemd 管理 |
| 同期 | 手動 | GitOps (systemd timer, 2分間隔) |
| 秘密情報 | 不要 | `/etc/coder/env` に外だし |
