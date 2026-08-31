const http = require('http');
const crypto = require('crypto');

const PORT = 8090;
const ISSUER = `http://localhost:${PORT}`;

// RSA 鍵ペア (JWT 署名用)
const { privateKey, publicKey } = crypto.generateKeyPairSync('rsa', {
    modulusLength: 2048,
    publicKeyEncoding: { type: 'spki', format: 'pem' },
    privateKeyEncoding: { type: 'pkcs8', format: 'pem' }
});

// JWK 生成
const jwk = {
    kty: 'RSA',
    use: 'sig',
    alg: 'RS256',
    kid: 'mock-key-1',
    n: crypto.createPublicKey(publicKey).export({ format: 'jwk' }).n,
    e: crypto.createPublicKey(publicKey).export({ format: 'jwk' }).e
};

// JWT 生成
function createJwt(payload) {
    const header = { alg: 'RS256', typ: 'JWT', kid: 'mock-key-1' };
    const encHeader = Buffer.from(JSON.stringify(header)).toString('base64url');
    const encPayload = Buffer.from(JSON.stringify(payload)).toString('base64url');
    const data = `${encHeader}.${encPayload}`;
    const sign = crypto.createSign('RSA-SHA256');
    sign.update(data);
    const signature = sign.sign(privateKey, 'base64url');
    return `${data}.${signature}`;
}

// 認証コードの一時保存
const authCodes = new Map();
// デバイスコードの一時保存 (SSH Device Flow用)
const deviceCodes = new Map();

// ダミー部員アカウント
const MOCK_USERS = [
    { id: '1001', username: 'tanaka', name: '田中 太郎', email: 'tanaka@school.ed.jp' },
    { id: '1002', username: 'suzuki', name: '鈴木 花子', email: 'suzuki@school.ed.jp' },
    { id: '1003', username: 'sato', name: '佐藤 健一', email: 'sato@school.ed.jp' },
    { id: '9999', username: 'advisor', name: '情報科 顧問 (管理者)', email: 'advisor@school.ed.jp' }
];

const server = http.createServer((req, res) => {
    let parsedUrl;
    try {
        parsedUrl = new URL(req.url, ISSUER);
    } catch (e) {
        parsedUrl = new URL('/', ISSUER);
    }
    const pathname = parsedUrl.pathname;

    // 0. ポータル案内画面 (/)
    if (pathname === '/') {
        res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
        return res.end(`
            <!DOCTYPE html>
            <html lang="ja">
            <head>
                <meta charset="UTF-8">
                <meta name="viewport" content="width=device-width, initial-scale=1.0">
                <title>学校ポータル - 数学研究部 SSO 認証センター</title>
                <style>
                    body {
                        font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
                        background: #0f172a;
                        color: #f8fafc;
                        display: flex;
                        align-items: center;
                        justify-content: center;
                        min-height: 100vh;
                        margin: 0;
                    }
                    .card {
                        background: #1e293b;
                        padding: 36px;
                        border-radius: 16px;
                        box-shadow: 0 20px 25px -5px rgba(0,0,0,0.5);
                        width: 100%;
                        max-width: 480px;
                        border: 1px solid #334155;
                        text-align: center;
                    }
                    h1 { margin: 0 0 8px 0; font-size: 1.5rem; color: #38bdf8; }
                    p { color: #94a3b8; font-size: 0.95rem; margin-bottom: 24px; }
                    .main-btn {
                        display: block;
                        padding: 16px;
                        background: #0284c7;
                        color: #fff;
                        text-decoration: none;
                        font-size: 1.1rem;
                        font-weight: bold;
                        border-radius: 10px;
                        transition: background 0.2s;
                        margin-bottom: 20px;
                    }
                    .main-btn:hover { background: #0369a1; }
                    .info {
                        padding: 14px;
                        background: #0f172a;
                        border-radius: 8px;
                        border: 1px solid #334155;
                        font-size: 0.85rem;
                        color: #94a3b8;
                        text-align: left;
                        line-height: 1.6;
                    }
                </style>
            </head>
            <body>
                <div class="card">
                    <h1>🏫 学校 SSO 認証センター</h1>
                    <p>数学研究部 クラウド演習室 認証プロバイダー</p>
                    
                    <a class="main-btn" href="http://localhost:7080/">
                        👉 クラウド演習室ポータルへ入室 (ポート 7080)
                    </a>

                    <div class="info">
                        ℹ️ <b>認証サーバー状態:</b> 正常稼働中 (ポート 8090)<br>
                        • <b>OIDC Discovery:</b> /.well-known/openid-configuration<br>
                        • <b>SSH SSO 承認:</b> /device
                    </div>
                </div>
            </body>
            </html>
        `);
    }

    // 1. OIDC Discovery
    if (pathname === '/.well-known/openid-configuration') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        return res.end(JSON.stringify({
            issuer: ISSUER,
            authorization_endpoint: `${ISSUER}/auth`,
            token_endpoint: `${ISSUER}/token`,
            userinfo_endpoint: `${ISSUER}/userinfo`,
            jwks_uri: `${ISSUER}/jwks`,
            response_types_supported: ['code'],
            subject_types_supported: ['public'],
            id_token_signing_alg_values_supported: ['RS256'],
            scopes_supported: ['openid', 'email', 'profile']
        }));
    }

    // 2. JWKS (公開鍵エンドポイント)
    if (pathname === '/jwks') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        return res.end(JSON.stringify({ keys: [jwk] }));
    }

    // 3. 認可エンドポイント (/auth)
    if (pathname === '/auth') {
        const redirectUri = parsedUrl.searchParams.get('redirect_uri');
        const state = parsedUrl.searchParams.get('state');
        const selectedUser = parsedUrl.searchParams.get('select_user');

        if (selectedUser) {
            const user = MOCK_USERS.find(u => u.username === selectedUser) || MOCK_USERS[0];
            const code = crypto.randomBytes(16).toString('hex');
            authCodes.set(code, { user, redirectUri, createdAt: Date.now() });

            const target = new URL(redirectUri);
            target.searchParams.set('code', code);
            if (state) target.searchParams.set('state', state);

            res.writeHead(302, { Location: target.toString() });
            return res.end();
        }

        // ログイン選択画面
        res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
        return res.end(`
            <!DOCTYPE html>
            <html lang="ja">
            <head>
                <meta charset="UTF-8">
                <meta name="viewport" content="width=device-width, initial-scale=1.0">
                <title>学校ポータル SSO ログイン</title>
                <style>
                    body {
                        font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
                        background: #0f172a;
                        color: #f8fafc;
                        display: flex;
                        align-items: center;
                        justify-content: center;
                        min-height: 100vh;
                        margin: 0;
                    }
                    .card {
                        background: #1e293b;
                        padding: 32px;
                        border-radius: 16px;
                        width: 100%;
                        max-width: 440px;
                        border: 1px solid #334155;
                        text-align: center;
                    }
                    h1 { font-size: 1.4rem; color: #38bdf8; margin: 0 0 8px 0; }
                    p { font-size: 0.9rem; color: #94a3b8; margin-bottom: 24px; }
                    .user-btn {
                        display: block;
                        width: 100%;
                        padding: 14px;
                        margin-bottom: 12px;
                        background: #0284c7;
                        color: #fff;
                        text-decoration: none;
                        border-radius: 8px;
                        font-weight: bold;
                        box-sizing: border-box;
                        transition: background 0.2s;
                    }
                    .user-btn:hover { background: #0369a1; }
                </style>
            </head>
            <body>
                <div class="card">
                    <h1>🏫 学校 SSO ログイン</h1>
                    <p>ログインする部員アカウントを選択してください</p>
                    ${MOCK_USERS.map(u => `
                        <a class="user-btn" href="/auth?select_user=${u.username}&redirect_uri=${encodeURIComponent(redirectUri || '')}&state=${encodeURIComponent(state || '')}">
                            👤 ${u.name} (${u.email})
                        </a>
                    `).join('')}
                </div>
            </body>
            </html>
        `);
    }

    // 4. トークンエンドポイント (/token)
    if (pathname === '/token' && req.method === 'POST') {
        let body = '';
        req.on('data', chunk => { body += chunk; });
        req.on('end', () => {
            const params = new URLSearchParams(body);
            const code = params.get('code');
            const authData = authCodes.get(code);

            if (!authData) {
                res.writeHead(400, { 'Content-Type': 'application/json' });
                return res.end(JSON.stringify({ error: 'invalid_grant' }));
            }

            authCodes.delete(code);
            const now = Math.floor(Date.now() / 1000);
            const user = authData.user;

            const idTokenPayload = {
                iss: ISSUER,
                sub: user.id,
                aud: 'school-cloud-client',
                exp: now + 3600,
                iat: now,
                preferred_username: user.username,
                name: user.name,
                email: user.email,
                email_verified: true
            };

            const idToken = createJwt(idTokenPayload);

            res.writeHead(200, { 'Content-Type': 'application/json' });
            return res.end(JSON.stringify({
                access_token: crypto.randomBytes(16).toString('hex'),
                token_type: 'Bearer',
                expires_in: 3600,
                id_token: idToken
            }));
        });
        return;
    }

    // 5. SSH Device Flow 承認画面 (/device)
    if (pathname === '/device') {
        const code = parsedUrl.searchParams.get('code');
        const user = parsedUrl.searchParams.get('user') || 'tanaka';
        const action = parsedUrl.searchParams.get('action');

        if (action === 'approve' && code) {
            deviceCodes.set(code, { status: 'approved', username: user, approvedAt: Date.now() });
            res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
            return res.end(`
                <!DOCTYPE html>
                <html lang="ja">
                <head>
                    <meta charset="UTF-8">
                    <title>SSH 承認完了</title>
                    <style>
                        body { font-family: sans-serif; background: #0f172a; color: #f8fafc; display: flex; align-items: center; justify-content: center; height: 100vh; margin: 0; }
                        .card { background: #1e293b; padding: 36px; border-radius: 16px; text-align: center; border: 1px solid #334155; }
                        h1 { color: #4ade80; margin-bottom: 12px; }
                    </style>
                </head>
                <body>
                    <div class="card">
                        <h1>✅ SSH セッションを承認しました！</h1>
                        <p>ターミナルに戻ると、専用マシン（LXD）のシェルが開始されています。<br>このウィンドウは閉じて構いません。</p>
                    </div>
                </body>
                </html>
            `);
        }

        res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
        return res.end(`
            <!DOCTYPE html>
            <html lang="ja">
            <head>
                <meta charset="UTF-8">
                <title>SSH 接続の承認</title>
                <style>
                    body { font-family: sans-serif; background: #0f172a; color: #f8fafc; display: flex; align-items: center; justify-content: center; height: 100vh; margin: 0; }
                    .card { background: #1e293b; padding: 36px; border-radius: 16px; text-align: center; max-width: 440px; border: 1px solid #334155; }
                    h1 { color: #38bdf8; margin-bottom: 12px; }
                    .btn { display: block; padding: 14px; background: #0284c7; color: #fff; text-decoration: none; border-radius: 8px; font-weight: bold; margin-top: 20px; }
                    .btn:hover { background: #0369a1; }
                </style>
            </head>
            <body>
                <div class="card">
                    <h1>🔐 SSH 接続の承認</h1>
                    <p>部員 <b>${user}</b> の専用マシンへの接続要求です。</p>
                    <p>端末コード: <code>${code}</code></p>
                    <a class="btn" href="/device?code=${code}&user=${user}&action=approve">
                        ✅ 承認して SSH セッションを開始
                    </a>
                </div>
            </body>
            </html>
        `);
    }

    // 6. SSH デバイスコード状態確認 API (/api/device/status)
    if (pathname === '/api/device/status') {
        const code = parsedUrl.searchParams.get('code');
        const data = deviceCodes.get(code);
        res.writeHead(200, { 'Content-Type': 'application/json' });
        return res.end(JSON.stringify(data || { status: 'pending' }));
    }

    // 7. UserInfo エンドポイント (/userinfo)
    if (pathname === '/userinfo') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        return res.end(JSON.stringify({
            sub: '1001',
            preferred_username: 'tanaka',
            name: '田中 太郎',
            email: 'tanaka@school.ed.jp',
            email_verified: true
        }));
    }

    res.writeHead(404);
    res.end('Not Found');
});

server.listen(PORT, '0.0.0.0', () => {
    console.log(`[Mock OIDC] 学校 SSO 認証サーバーが起動しました: ${ISSUER}`);
});
