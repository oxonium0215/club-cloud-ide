package main

// ポータル画面 (2ボタンUI + ロード画面) と cloud-init データ。

import (
	"fmt"
	"net/http"
)

// renderPortalIndex はポータル画面を返す。
// 未ログイン: ログインボタン
// ログイン済み: VS Code / デスクトップ の2ボタン
func renderPortalIndex(w http.ResponseWriter, loggedIn bool, username, name, containerStatus string) {
	html := `<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0, user-scalable=no">
<title>数学研究部 クラウド演習室</title>
<style>
    body {
        margin: 0; min-height: 100vh;
        display: flex; flex-direction: column;
        align-items: center; justify-content: center;
        background: #0f172a; color: #f8fafc;
        font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
        text-align: center; padding: 24px;
    }
    h1 { font-size: 1.5rem; margin: 0 0 6px; }
    .sub { color: #94a3b8; margin-bottom: 8px; }
    .status { color: #64748b; font-size: 0.85rem; margin-bottom: 28px; }
    .apps { display: flex; gap: 20px; flex-wrap: wrap; justify-content: center; }
    .card {
        width: 220px; padding: 32px 20px; border-radius: 16px;
        background: #1e293b; border: 1px solid #334155;
        text-decoration: none; color: #f8fafc; cursor: pointer;
        transition: transform 0.15s, background 0.15s; user-select: none;
    }
    .card:hover { transform: translateY(-4px); background: #334155; }
    .card .icon { font-size: 2.2rem; margin-bottom: 10px; }
    .card .label { font-size: 1.05rem; font-weight: 600; }
    .card .desc { font-size: 0.82rem; color: #94a3b8; margin-top: 6px; }
    .btn-login {
        padding: 14px 32px; border-radius: 12px;
        background: #2563eb; color: #fff; text-decoration: none;
        font-size: 1rem; font-weight: 600; border: none; cursor: pointer;
    }
    .btn-login:hover { background: #1d4ed8; }
    .btn-logout {
        position: absolute; top: 20px; right: 20px;
        padding: 8px 16px; border-radius: 8px;
        background: #1e293b; color: #94a3b8;
        font-size: 0.8rem; text-decoration: none; border: 1px solid #334155;
    }
    .btn-logout:hover { color: #f8fafc; }
    #loading {
        display: none; position: fixed; inset: 0; z-index: 10;
        background: rgba(15, 23, 42, 0.92);
        flex-direction: column; align-items: center; justify-content: center;
    }
    #loading .spinner {
        width: 48px; height: 48px; margin-bottom: 20px;
        border: 4px solid #334155; border-top-color: #2563eb;
        border-radius: 50%; animation: spin 0.9s linear infinite;
    }
    #loading .msg { font-size: 1rem; color: #e2e8f0; }
    #loading .submsg { font-size: 0.85rem; color: #64748b; margin-top: 8px; }
    @keyframes spin { to { transform: rotate(360deg); } }
</style>
</head>
<body>`

	if !loggedIn {
		html += `
<h1>数学研究部 クラウド演習室</h1>
<p class="sub">ログインして開発環境を利用してください</p>
<a class="btn-login" href="/login">学校アカウントでログイン</a>`
	} else {
		html += fmt.Sprintf(`
<a class="btn-logout" href="/logout">ログアウト</a>
<h1>%s さん、こんにちは</h1>
<p class="sub">VS Code か Linux デスクトップを選択してください</p>
<p class="status" id="status">読み込み中...</p>
<div class="apps">
    <div class="card" onclick="launch('vscode')">
        <div class="icon">&#9003;</div>
        <div class="label">VS Code</div>
        <div class="desc">エディタで開発</div>
    </div>
    <div class="card" onclick="launch('desktop')">
        <div class="icon">&#128421;</div>
        <div class="label">デスクトップ</div>
        <div class="desc">KDE Plasma デスクトップ</div>
    </div>
</div>
<div id="loading">
    <div class="spinner"></div>
    <div class="msg" id="loadingMsg">コンテナを準備中...</div>
    <div class="submsg" id="loadingSub">初回は数分かかることがあります</div>
</div>
<script>
const HEARTBEAT_MS = 30000;
let statusEl = document.getElementById('status');

// コンテナ状態の取得
async function refreshStatus() {
    try {
        const res = await fetch('/api/me', { credentials: 'same-origin' });
        if (res.status === 401) { location.href = '/login'; return; }
        const data = await res.json();
        const map = { running: '稼働中', stopped: '停止中', missing: '未作成', starting: '起動中' };
        statusEl.textContent = 'コンテナ: ' + (map[data.container] || data.container);
    } catch (e) {
        statusEl.textContent = '状態を取得できません';
    }
}

// アプリ起動
async function launch(app) {
    const loading = document.getElementById('loading');
    loading.style.display = 'flex';
    document.getElementById('loadingMsg').textContent = 'コンテナを準備中...';

    try {
        const res = await fetch('/api/launch?app=' + app, { credentials: 'same-origin' });
        if (res.status === 401) { location.href = '/login'; return; }
        const data = await res.json();
        if (res.status !== 200) {
            document.getElementById('loadingMsg').textContent = 'エラー: ' + (data.error || '不明なエラー');
            return;
        }
        document.getElementById('loadingMsg').textContent = '起動しました。アプリを開いています...';
        location.href = data.proxy_url;
    } catch (e) {
        document.getElementById('loadingMsg').textContent = '接続エラー';
    }
}

// heartbeat (アイドル判定用)
setInterval(() => {
    fetch('/api/heartbeat', { credentials: 'same-origin', method: 'POST' }).catch(() => {});
}, HEARTBEAT_MS);

refreshStatus();
setInterval(refreshStatus, 10000);
</script>`, name)
	}

	html += `
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, html)
}

// cloudInitData はコンテナ作成時に cloud-init で注入する設定。
// GitOps: 起動時にリポジトリから最新設定を pull する。
const cloudInitData = `#cloud-config
users:
  - name: coder
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
packages: []
runcmd:
  - [sh, -c, "if [ ! -d /home/coder/.config/club-cloud-ide/.git ]; then git clone --depth 1 --branch main REPO_URL_PLACEHOLDER /home/coder/.config/club-cloud-ide; else git -C /home/coder/.config/club-cloud-ide pull --ff-only; fi"]
  - [sh, -c, "chown -R coder:coder /home/coder/.config/club-cloud-ide"]
  - [sh, -c, "sudo -u coder /home/coder/.config/club-cloud-ide/templates/lxd-siv3d/files/apply.sh"]
`
