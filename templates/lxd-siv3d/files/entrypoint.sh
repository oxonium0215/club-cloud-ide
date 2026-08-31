#!/usr/bin/env bash
# ワークスペース起動時に呼ばれるエントリポイント。
# (startup_script が apply.sh で設定配布済みなので、ここでは起動のみ)
set -e
export HOME=/home/coder
export USER=coder

# X11 ロック掃除
rm -f /tmp/.X1-lock /tmp/.X11-unix/X1 /home/coder/.vnc/*.pid /home/coder/.vnc/*.log 2>/dev/null || true
mkdir -p /tmp/.X11-unix
chmod 1777 /tmp/.X11-unix 2>/dev/null || true

# XDG_RUNTIME_DIR (KDE Plasma 必須) を用意
mkdir -p /run/user/1000
chown coder:coder /run/user/1000
chmod 0700 /run/user/1000

# 1. code-server (ポート 13337) 起動
sudo -u coder code-server --auth none --port 13337 /home/coder/workspace &

# 2. KasmVNC サーバー (ポート 6080) 起動
#    -disableBasicAuth: KasmVNC WebUI の HTTP Basic 認証を無効化 (KasmVNC issue #259)
sudo -u coder vncserver :1 -geometry 1280x800 -depth 24 -websocketPort 6080 -sslOnly 0 -interface 127.0.0.1 -SecurityTypes None -videoCodec disabled -disableBasicAuth -fg

wait -n
