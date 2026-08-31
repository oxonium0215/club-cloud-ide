// 部員向けポータル (Coder の入口)
// Coder の認証・ワークスペース管理はそのまま使い、
// ログイン状態に応じて「VS Code / デスクトップ」の2ボタンを表示する。
// ポート 7080 で待ち受け、/ と /portal はポータル、その他は Coder にプロキシする。
const http = require('http');
const fs = require('fs');
const path = require('path');

const PORT = 7080;
const CODER_URL = process.env.CODER_URL || 'http://localhost:7080';
const PORTAL_HTML = path.join(__dirname, 'portal.html');

const server = http.createServer((req, res) => {
    const url = new URL(req.url, `http://localhost:${PORT}`);
    const pathname = url.pathname;

    // ポータル本体
    if (pathname === '/' || pathname === '/portal') {
        fs.readFile(PORTAL_HTML, (err, data) => {
            if (err) {
                res.writeHead(500);
                res.end('Portal file not found');
                return;
            }
            res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
            res.end(data);
        });
        return;
    }

    // それ以外は Coder にプロキシ
    proxyToCoder(req, res);
});

function proxyToCoder(req, res) {
    const upstream = new URL(CODER_URL + req.url);
    const proxyReq = http.request(
        {
            hostname: upstream.hostname,
            port: upstream.port,
            path: upstream.pathname + upstream.search,
            method: req.method,
            headers: { ...req.headers, host: upstream.host },
        },
        (proxyRes) => {
            res.writeHead(proxyRes.statusCode, proxyRes.headers);
            proxyRes.pipe(res);
        }
    );
    proxyReq.on('error', (err) => {
        res.writeHead(502);
        res.end('Coder unreachable: ' + err.message);
    });
    req.pipe(proxyReq);
}

server.listen(PORT, '0.0.0.0', () => {
    console.log(`[portal] http://localhost:${PORT}`);
});
