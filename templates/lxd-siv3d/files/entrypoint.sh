#!/usr/bin/env bash
# ワークスペース起動時に呼ばれるエントリポイント。
# (startup_script が apply.sh で設定配布済みなので、ここでは起動のみ)
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
#    サブパス (X-Forwarded-Prefix) はポータルのリバースプロキシが付与する
#    --bind-addr 0.0.0.0: プロキシがコンテナIP経由でアクセスするため
sudo -u osgsuken code-server --auth none --bind-addr 0.0.0.0:13337 /home/osgsuken/workspace &

# 2. KasmVNC サーバー (ポート 6080) 起動
#    -disableBasicAuth: KasmVNC WebUI の HTTP Basic 認証を無効化 (KasmVNC issue #259)
#    -interface 0.0.0.0: ポータルのリバースプロキシがコンテナIP経由でアクセスするため
#    -fg は使わない: 初回起動プロンプト (ユーザー選択) を回避する
#    (プロンプトは .de-was-selected とユーザー設定をイメージに焼き込んで無効化。
#     それでも出る場合は echo "3" で「書き込みユーザーなし」を選択して通過)
echo "3" | sudo -u osgsuken vncserver :1 -geometry 1280x800 -depth 24 -websocketPort 6080 -sslOnly 0 -interface 0.0.0.0 -SecurityTypes None -videoCodec disabled -disableBasicAuth &

# 全プロセスの終了を待つ
wait
