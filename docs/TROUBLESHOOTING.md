# トラブルシューティング

## `requester is not authorized to access the object` ログ

未ログイン状態でブラウザがポータル画面を開いた際に記録される。未認証ユーザーに対する正常なアクセス制御であり、不具合ではない。SSO ログイン後に解消される。

## VNC デスクトップが真っ暗になる / 真っ黒になる

### 原因1: 間違ったベースイメージ

`osgsuken-base-img` ではなく素の Ubuntu が使われていると、KasmVNC も LXQt も入らない。

- **対処:** `osgsuken-base-img` (LXQt + KasmVNC 入りゴールデンイメージ) を参照する。作成は `npm run build-image`。

### 原因2: LXD コンテナの外部通信断 (Docker との iptables 競合)

Docker が `iptables FORWARD` ポリシーを `DROP` に設定すると、コンテナからの apt が遮断され、イメージ構築が失敗する。

- **対処:** `scripts/start.sh` が起動時に NAT ルールを追加する。手動修復:
  ```bash
  sudo iptables -t nat -A POSTROUTING -s 10.10.10.0/24 ! -d 10.10.10.0/24 -j MASQUERADE
  sudo iptables -I FORWARD -i lxdbr0 -o lxdbr0 -j ACCEPT
  sudo iptables -I FORWARD -i lxdbr0 -o eth0 -j ACCEPT
  sudo iptables -I FORWARD -i eth0 -o lxdbr0 -m state --state RELATED,ESTABLISHED -j ACCEPT
  ```

### 原因3: KasmVNC WebUI の HTTP Basic 認証 (401) によるログイン画面ループ

`$HOME/.kasmpasswd` による BasicAuth が有効だと認証ダイアログが要求され、失敗すると真っ暗なままになる。

- **対処:** vncserver 起動時に `-disableBasicAuth` を付与する (KasmVNC issue #259)。`$HOME/.kasmpasswd` は残しておくこと (存在チェックされるため)。

### 原因4: 画面描画モード (WebCodecs / WebGL) の互換性問題

KasmVNC 1.5.0 デフォルトの動画エンコードが一部ブラウザで描画されない。

- **対処:** `setup-base-image.sh` 内のパッチで以下を強制する。
  - `fallback_image_mode: true` (静止画像モード)
  - `stream_mode: jpeg_webp` (動画エンコード無効)
  - `video_rendering_mode: canvas2d` (Canvas2D 描画)
  ※ パッチは `/usr/share/kasmvnc/www/assets/ui-*.js` に適用される。

## タスクバーのアイコンを押すと VNC 接続が切れる

タスクバーからアプリを起動するとデスクトップセッションがクラッシュし、Xvnc ごと終了して接続が切れる。以下の複合原因と対策 (KDE 時代の教訓):

- **原因1: 必須パッケージ欠落** — `--no-install-recommends` で推奨パッケージ (アプリ起動に必須) が入らない。
  - **対処:** `--no-install-recommends` を廃止し、LXQt に移行。
- **原因2: `XDG_RUNTIME_DIR` 未設定** — デスクトップセッションは `/run/user/1000` を必須とする。
  - **対処:** `xstartup` / `entrypoint.sh` が自動作成する。
- **原因3: `exec` 起動** — セッションが落ちると Xvnc ごと終了する構造。
  - **対処:** `xstartup` を監視ループに変更し、セッションを自動再起動。
- **原因4: LXD の `processes` 制限** — 250 ではデスクトップ + アプリ起動で枯渇する。
  - **対処:** `processes = "1024"` に引き上げ。

### 既存コンテナへの応急処置

```bash
# コンテナ内
sudo mkdir -p /run/user/1000 && sudo chown osgsuken:osgsuken /run/user/1000 && sudo chmod 0700 /run/user/1000
# /home/osgsuken/.vnc/xstartup を新バージョンに差し替え、vncserver を再起動
```
