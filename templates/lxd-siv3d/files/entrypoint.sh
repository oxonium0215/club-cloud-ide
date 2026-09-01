#!/usr/bin/env bash
# ワークスペース起動時に呼ばれるエントリポイント。
# (apply.sh が設定配布済みなので、ここでは起動のみ)
set -e
export HOME=/home/osgsuken
export USER=osgsuken

# X11 ロック掃除
rm -f /tmp/.X1-lock /tmp/.X11-unix/X1 /home/osgsuken/.vnc/*.pid /home/osgsuken/.vnc/*.log 2>/dev/null || true
mkdir -p /tmp/.X11-unix
chmod 1777 /tmp/.X11-unix 2>/dev/null || true

# XDG_RUNTIME_DIR (LXQt 必須) を用意
mkdir -p /run/user/1000
chown osgsuken:osgsuken /run/user/1000
chmod 0700 /run/user/1000

# 1. code-server (ポート 13337) 起動
#    サブパス認識はポータルのリバースプロキシが付与する
#    X-Forwarded-Prefix: /proxy/vscode ヘッダーによる (code-server 4.x の仕組み)。
#    --bind-addr 0.0.0.0: プロキシがコンテナIP経由でアクセスするため
sudo -u osgsuken code-server /home/osgsuken/workspace &

# 2. TigerVNC サーバー (ポート 5901) 起動
#    -localhost を付けず 0.0.0.0 でリッスン (websockify が同一コンテナ内から接続)
#    -SecurityTypes VncAuth: パスワード認証 (~/.vnc/passwd)
sudo -u osgsuken vncserver :1 -geometry 1280x800 -depth 24 -SecurityTypes VncAuth -localhost no &

# 3. noVNC (websockify) をポート 6080 で起動
#    --web: noVNC の静的ファイル (vnc.html 等) を配信
#    --listen 6080: プロキシがコンテナIP経由でアクセスするため 0.0.0.0 で待受
#    localhost:5901: TigerVNC へのブリッジ
websockify --web /usr/share/novnc --listen 6080 --heartbeat 30 localhost:5901 &

# 全プロセスの終了を待つ
wait
