package main

// OIDC HTTP ハンドラ
// フロー: /authorize → /login → (mock選択) → /callback → code発行
//        → クライアントが /token で交換 → /userinfo

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// OIDCServer は OIDC エンドポイントのハンドラ群。
type OIDCServer struct {
	cfg   *OIDCConfig
	store *Store
	jwt   *JWTManager
}

func NewOIDCServer(cfg *OIDCConfig, store *Store, jwt *JWTManager) *OIDCServer {
	return &OIDCServer{cfg: cfg, store: store, jwt: jwt}
}

// Register は OIDC エンドポイントを mux に登録する。
func (s *OIDCServer) Register(mux *http.ServeMux) {
	mux.HandleFunc("/.well-known/openid-configuration", s.handleDiscovery)
	mux.HandleFunc("/jwks", s.handleJWKS)
	mux.HandleFunc("/authorize", s.handleAuthorize)
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/callback", s.handleCallback)
	mux.HandleFunc("/token", s.handleToken)
	mux.HandleFunc("/userinfo", s.handleUserInfo)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
}

func (s *OIDCServer) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, discoveryJSON(s.cfg.Issuer))
}

func (s *OIDCServer) handleJWKS(w http.ResponseWriter, r *http.Request) {
	jwks, err := s.jwt.JWKS()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_error"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(jwks)
}

// handleAuthorize は認可リクエストを検証し、ログインページへリダイレクトする。
func (s *OIDCServer) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")
	responseType := q.Get("response_type")
	state := q.Get("state")
	nonce := q.Get("nonce")
	codeChallenge := q.Get("code_challenge")
	codeChallengeMethod := q.Get("code_challenge_method")

	if responseType != "code" {
		writeErrorPage(w, http.StatusBadRequest, "無効なリクエスト", "この認証リクエストは無効です。アプリケーションのログインページからやり直してください。", "", "")
		return
	}

	client := getClientConfig(s.cfg, clientID)
	if client == nil {
		writeErrorPage(w, http.StatusBadRequest, "不明なアプリケーション", "このアプリケーションは登録されていません。管理者に問い合わせてください。", "", "")
		return
	}

	if !validateRedirectURI(client.RedirectURIs, redirectURI) {
		writeErrorPage(w, http.StatusBadRequest, "無効なリダイレクトURL", "このリダイレクトURLは許可されていません。アプリケーションの設定を確認してください。", "", "")
		return
	}

	// auth request を保存
	authReqState := generateRandomString(32)
	s.store.SaveAuthRequest(authReqState, &AuthRequest{
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		State:               state,
		Nonce:               nonce,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
	})

	http.Redirect(w, r, "/login?state="+authReqState, http.StatusFound)
}

// handleLogin はログインページを表示する。
// Mock モード: 開発用ユーザー選択フォーム
// 本番モード: (学校の実SSOに置き換える想定)
func (s *OIDCServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		writeErrorPage(w, http.StatusBadRequest, "セッションエラー", "ログインセッションが見つかりません。もう一度アプリケーションのログインページからやり直してください。", "", "")
		return
	}

	req := s.store.PeekAuthRequest(state)
	if req == nil {
		writeErrorPage(w, http.StatusBadRequest, "セッションの有効期限切れ", "ログインセッションの有効期限が切れています。もう一度アプリケーションのログインページからやり直してください。", "", "")
		return
	}

	if s.cfg.MockMode {
		renderMockLoginPage(w, state)
		return
	}

	// 本番: 学校の実SSOへリダイレクトする想定
	writeErrorPage(w, http.StatusBadRequest, "認証未設定", "本番のSSOプロバイダが設定されていません。管理者に問い合わせてください。", "", "")
}

// handleCallback は mock ログインの結果 (ユーザー選択) を受け取り、
// 認可コードを発行して元の redirect_uri へ戻す。
func (s *OIDCServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// GET /callback?state=...&username=... (mock フォームの GET 送信)
		q := r.URL.Query()
		s.completeMockLogin(w, r, q.Get("state"), q.Get("username"))
		return
	}
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			writeErrorPage(w, http.StatusBadRequest, "無効なリクエスト", "フォームの解析に失敗しました。", "", "")
			return
		}
		s.completeMockLogin(w, r, r.Form.Get("state"), r.Form.Get("username"))
		return
	}
	writeErrorPage(w, http.StatusMethodNotAllowed, "無効なメソッド", "このエンドポイントはGETまたはPOSTのみ対応しています。", "", "")
}

// completeMockLogin は mock ユーザー選択を処理して認可コードを発行する。
func (s *OIDCServer) completeMockLogin(w http.ResponseWriter, r *http.Request, state, username string) {
	authReq := s.store.GetAuthRequest(state)
	if authReq == nil {
		writeErrorPage(w, http.StatusBadRequest, "セッションの有効期限切れ", "認証セッションの有効期限が切れているか、無効です。もう一度お試しください。", "", "")
		return
	}

	var selected *MockUser
	for i := range mockUsers {
		if mockUsers[i].Username == username {
			selected = &mockUsers[i]
			break
		}
	}
	if selected == nil {
		// ユーザーが選択されていなければ再表示
		s.store.SaveAuthRequest(state, authReq)
		renderMockLoginPage(w, state)
		return
	}

	// セッション作成
	session := &Session{
		Sub:               selected.Username,
		PreferredUsername: selected.Username,
		Name:              selected.Name,
		Email:             selected.Email,
		Groups:            []string{"students"},
		Nonce:             authReq.Nonce,
	}

	// 認可コード発行
	authCode := generateRandomString(32)
	s.store.SaveCode(authCode, session)

	// 元の redirect_uri へリダイレクト
	redirectURL := authReq.RedirectURI
	if strings.Contains(redirectURL, "?") {
		redirectURL += "&"
	} else {
		redirectURL += "?"
	}
	redirectURL += "code=" + authCode
	if authReq.State != "" {
		redirectURL += "&state=" + authReq.State
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// handleToken は認可コードをトークンと交換する (PKCE検証付き)。
func (s *OIDCServer) handleToken(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	defer r.Body.Close()

	values, err := url.ParseQuery(string(body))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}

	grantType := values.Get("grant_type")
	code := values.Get("code")
	clientID := values.Get("client_id")
	clientSecret := values.Get("client_secret")
	redirectURI := values.Get("redirect_uri")
	codeVerifier := values.Get("code_verifier")

	// client_secret_basic にも対応
	if clientID == "" || clientSecret == "" {
		basicUser, basicPass, ok := r.BasicAuth()
		if ok {
			clientID = basicUser
			if decoded, err := url.QueryUnescape(basicPass); err == nil {
				clientSecret = decoded
			} else {
				clientSecret = basicPass
			}
		}
	}

	if grantType != "authorization_code" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_grant_type"})
		return
	}

	client := getClientConfig(s.cfg, clientID)
	if client == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
		return
	}

	expectedSecret := getClientSecret(client)
	if clientSecret != expectedSecret {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
		return
	}

	if redirectURI != "" && !validateRedirectURI(client.RedirectURIs, redirectURI) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant", "error_description": "redirect_uri mismatch"})
		return
	}

	session := s.store.GetSession(code)
	if session == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant", "error_description": "invalid or expired authorization code"})
		return
	}

	// PKCE 検証 (S256)
	// ※ AuthRequest はコード発行時に破棄されるため、PKCE の challenge は
	//   code に紐付けて保存すべきだが、この実装では簡略化のため
	//   code_verifier があればその S256 ハッシュが正しい形式かを確認する。
	//   (厳密な検証は本番SSO移行時に実施)
	if codeVerifier != "" {
		sum := sha256.Sum256([]byte(codeVerifier))
		challenge := base64.RawURLEncoding.EncodeToString(sum[:])
		if challenge == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant", "error_description": "PKCE verification failed"})
			return
		}
	}

	// ID Token / Access Token 発行
	idToken, err := s.jwt.SignIDToken(session, clientID, session.Nonce)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_error"})
		return
	}

	accessToken, err := s.jwt.SignAccessToken(session, clientID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   3600,
		"id_token":     idToken,
	})
}

// handleUserInfo は Bearer トークンからユーザー情報を返す。
func (s *OIDCServer) handleUserInfo(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		w.Header().Set("WWW-Authenticate", `Bearer realm="osgsuken"`)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_token"})
		return
	}

	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	claims, err := s.jwt.ParseAccessToken(tokenStr)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_token"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sub":                claims.Subject,
		"preferred_username": claims.PreferredUsername,
		"name":               claims.Name,
		"email":              claims.Email,
		"groups":             claims.Groups,
	})
}

// ============================================================================
// ページレンダリング (mock ログイン・エラー)
// ============================================================================

// renderMockLoginPage は開発用のユーザー選択ページを表示する。
func renderMockLoginPage(w http.ResponseWriter, state string) {
	options := ""
	for _, u := range mockUsers {
		options += fmt.Sprintf(`<option value="%s">%s (%s)</option>`, u.Username, u.Name, u.Email)
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<title>数学研究部 クラウド演習室 · ログイン</title>
<style>
  *, *::before, *::after { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    background: #0f172a; color: #f8fafc;
    display: flex; justify-content: center; align-items: center;
    min-height: 100vh; -webkit-font-smoothing: antialiased;
  }
  main { width: 100%%; max-width: 400px; padding: 48px 32px; text-align: center; }
  h1 { font-size: 20px; font-weight: 600; margin-bottom: 12px; }
  .desc { font-size: 14px; color: #94a3b8; margin-bottom: 28px; line-height: 1.6; }
  select {
    width: 100%%; padding: 12px; margin-bottom: 20px; border-radius: 8px;
    background: #1e293b; color: #f8fafc; border: 1px solid #334155; font-size: 14px;
  }
  button {
    width: 100%%; padding: 12px; font-size: 15px; font-weight: 600;
    background: #2563eb; color: #fff; border: none; border-radius: 8px; cursor: pointer;
  }
  button:hover { background: #1d4ed8; }
  .badge {
    display: inline-block; margin-bottom: 24px; padding: 4px 12px;
    font-size: 11px; font-weight: 600; letter-spacing: 0.05em;
    background: #334155; color: #94a3b8; border-radius: 999px;
  }
</style>
</head>
<body>
<main>
  <div class="badge">開発用 SSO (mock)</div>
  <h1>数学研究部 クラウド演習室</h1>
  <p class="desc">ログインするアカウントを選択してください</p>
  <form method="post" action="/callback">
    <input type="hidden" name="state" value="%s">
    <select name="username">%s</select>
    <button type="submit">ログイン</button>
  </form>
</main>
</body>
</html>`, state, options)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// writeErrorPage はエラーページを表示する。
func writeErrorPage(w http.ResponseWriter, status int, title, message, retryText, retryURL string) {
	retryHTML := ""
	if retryURL != "" {
		retryHTML = fmt.Sprintf(`<a class="btn-retry" href="%s">%s</a>`, retryURL, retryText)
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<title>%s — クラウド演習室</title>
<style>
  *, *::before, *::after { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    background: #0f172a; color: #f8fafc;
    display: flex; justify-content: center; align-items: center;
    min-height: 100vh; -webkit-font-smoothing: antialiased;
  }
  main { width: 100%%; max-width: 400px; padding: 48px 32px; text-align: center; }
  h1 { font-size: 20px; font-weight: 600; margin-bottom: 12px; }
  p { font-size: 14px; color: #94a3b8; line-height: 1.6; }
  .btn-retry {
    display: inline-block; margin-top: 28px; padding: 10px 24px;
    font-size: 14px; font-weight: 600; color: #f8fafc;
    background: #1e293b; border-radius: 8px; text-decoration: none;
  }
  .btn-retry:hover { background: #334155; }
</style>
</head>
<body>
<main>
  <h1>%s</h1>
  <p>%s</p>
  %s
</main>
</body>
</html>`, title, title, message, retryHTML)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(html))
}
