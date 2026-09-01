package main

// 数学研究部 クラウド演習室 ポータル (Go)
// - ポート 7080: 部員向けポータル (2ボタン入口 + リバースプロキシ + heartbeat)
// - ポート 8090: OIDC プロバイダ (開発モードは mock ユーザー選択)
//
// 仕様:
// 1. SSO ログイン (OIDC Authorization Code Flow + PKCE)
// 2. ログイン後、VS Code / デスクトップ の2ボタン画面
// 3. ボタン押下時: コンテナ状態に応じて 遷移 / 起動ロード / 作成ロード
// 4. 最終アクティビティから60分で自動シャットダウン

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// 設定
// ============================================================================

type Config struct {
	PortalAddr      string // ポータル HTTP アドレス (default :7080)
	OIDCAddr        string // OIDC プロバイダ HTTP アドレス (default :8090)
	Issuer          string // OIDC issuer URL (default http://localhost:8090)
	ClientID        string
	ClientSecret    string
	RedirectURI     string // OIDC コールバック (ポータル側)
	MockMode        bool
	IdleTimeout     time.Duration // 自動シャットダウンまでのアイドル時間 (default 60m)
	HeartbeatFreq   time.Duration // クライアント heartbeat 間隔
	LXDImage        string
	RepoURL         string // 設定配布元 Git リポジトリ (GitOps)
	ContainerLimits map[string]string
}

func defaultConfig() *Config {
	return &Config{
		PortalAddr:    ":7080",
		OIDCAddr:      ":8090",
		Issuer:        "http://localhost:8090",
		ClientID:      "osgsuken-portal",
		ClientSecret:  "osgsuken-portal-secret",
		RedirectURI:   "http://localhost:7080/auth/callback",
		MockMode:      true,
		IdleTimeout:   60 * time.Minute,
		HeartbeatFreq: 30 * time.Second,
		LXDImage:      "osgsuken-base-img",
		RepoURL:       "https://github.com/yourorg/club-cloud-ide.git",
	}
}

// ============================================================================
// ポータルサーバー
// ============================================================================

// PortalServer は部員向けポータル (2ボタンUI + プロキシ + セッション)。
type PortalServer struct {
	cfg    *Config
	lxd    *ContainerManager
	oidc   *OIDCServer
	client *PortalOIDCClient
	jwt    *JWTManager
	// sessions: ポータル独自セッション (token → PortalSession)
	// ※ 本来は OIDC セッションと分離するが、簡略化のため
	//    OIDC の access_token をポータルのセッションとして使う
	sessions *PortalSessionStore
}

// PortalSession はポータル内のログインセッション。
type PortalSession struct {
	Token           string
	Username        string
	Name            string
	LastSeen        time.Time
	ContainerStatus ContainerStatus
}

// PortalSessionStore はセッションのスレッドセーフなストア。
type PortalSessionStore struct {
	mu       sync.Mutex
	sessions map[string]*PortalSession
}

func NewPortalSessionStore() *PortalSessionStore {
	return &PortalSessionStore{sessions: make(map[string]*PortalSession)}
}

func (s *PortalSessionStore) Get(token string) *PortalSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ps, ok := s.sessions[token]; ok {
		ps.LastSeen = time.Now()
		return ps
	}
	return nil
}

func (s *PortalSessionStore) Set(token string, ps *PortalSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ps.LastSeen = time.Now()
	s.sessions[token] = ps
}

func (s *PortalSessionStore) Delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

// All は全セッションを返す (自動シャットダウン判定用)。
func (s *PortalSessionStore) All() []*PortalSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*PortalSession, 0, len(s.sessions))
	for _, ps := range s.sessions {
		out = append(out, ps)
	}
	return out
}

// RegisterRoutes はポータルのルーティングを登録する。
func (p *PortalServer) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", p.handleRoot)
	mux.HandleFunc("/login", p.handleLoginRedirect)
	mux.HandleFunc("/auth/callback", p.handleAuthCallback)
	mux.HandleFunc("/logout", p.handleLogout)
	mux.HandleFunc("/api/me", p.handleAPIme)
	mux.HandleFunc("/api/launch", p.handleAPILaunch)
	mux.HandleFunc("/api/heartbeat", p.handleAPIHeartbeat)
	// Caddy forward_auth 用: セッション検証エンドポイント
	mux.HandleFunc("/auth/check", p.handleAuthCheck)
}

// handleRoot はポータル画面を返す。
func (p *PortalServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	// /proxy/ 以外は SPA 的にポータル HTML を返す
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	session := p.sessionFromRequest(r)
	if session == nil {
		renderPortalIndex(w, false, "", "", "")
		return
	}
	renderPortalIndex(w, true, session.Username, session.Name, "")
}

// sessionFromRequest は Cookie のセッショントークンからセッションを取得する。
// インメモリストアに無い場合は JWT (Access Token) を検証して再構築する。
// これによりポータル再起動後も有効な Cookie があれば再ログイン不要になる。
func (p *PortalServer) sessionFromRequest(r *http.Request) *PortalSession {
	cookie, err := r.Cookie("osgsuken_session")
	if err != nil {
		return nil
	}
	if ps := p.sessions.Get(cookie.Value); ps != nil {
		return ps
	}
	// ストアに無い → JWT を検証して復元を試みる
	claims, err := p.jwt.ParseAccessToken(cookie.Value)
	if err != nil || claims.PreferredUsername == "" {
		return nil
	}
	ps := &PortalSession{
		Token:    cookie.Value,
		Username: claims.PreferredUsername,
		Name:     claims.Name,
	}
	p.sessions.Set(ps.Token, ps)
	return ps
}

// handleLoginRedirect は OIDC 認証を開始する。
func (p *PortalServer) handleLoginRedirect(w http.ResponseWriter, r *http.Request) {
	authURL := p.client.AuthCodeURL()
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleAuthCallback は OIDC のコールバックを受け取りセッションを確立する。
func (p *PortalServer) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" {
		writeErrorPage(w, http.StatusBadRequest, "認証エラー", "認証コードがありません。もう一度お試しください。", "", "")
		return
	}
	if !p.client.VerifyState(state) {
		writeErrorPage(w, http.StatusBadRequest, "セッションエラー", "state の検証に失敗しました。もう一度ログインしてください。", "", "")
		return
	}

	// OIDC トークン交換
	token, err := p.client.Exchange(code, state)
	if err != nil {
		log.Printf("token exchange failed: %v", err)
		writeErrorPage(w, http.StatusInternalServerError, "認証エラー", "トークンの交換に失敗しました。もう一度お試しください。", "", "")
		return
	}

	// セッション作成
	ps := &PortalSession{
		Token:    token.AccessToken,
		Username: token.PreferredUsername,
		Name:     token.Name,
	}
	p.sessions.Set(ps.Token, ps)

	http.SetCookie(w, &http.Cookie{
		Name:     "osgsuken_session",
		Value:    ps.Token,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   12 * 60 * 60,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

// handleLogout はセッションを破棄する。
func (p *PortalServer) handleLogout(w http.ResponseWriter, r *http.Request) {
	session := p.sessionFromRequest(r)
	if session != nil {
		p.sessions.Delete(session.Token)
	}
	http.SetCookie(w, &http.Cookie{
		Name: "osgsuken_session", Value: "", Path: "/", HttpOnly: true, MaxAge: -1,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

// handleAPIme はログイン中のユーザー情報とコンテナ状態を返す。
func (p *PortalServer) handleAPIme(w http.ResponseWriter, r *http.Request) {
	session := p.sessionFromRequest(r)
	if session == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not_authenticated"})
		return
	}

	status, err := p.lxd.Status(session.Username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	session.ContainerStatus = status

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"username":  session.Username,
		"name":      session.Name,
		"container": status,
	})
}

// handleAPILaunch はコンテナを起動/作成してプロキシURLを返す。
func (p *PortalServer) handleAPILaunch(w http.ResponseWriter, r *http.Request) {
	session := p.sessionFromRequest(r)
	if session == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not_authenticated"})
		return
	}

	app := r.URL.Query().Get("app") // "vscode" | "desktop"
	if app != "vscode" && app != "desktop" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid app"})
		return
	}

	status, err := p.lxd.Status(session.Username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	switch status {
	case StatusMissing:
		// コンテナ作成 → 起動
		userData := strings.ReplaceAll(cloudInitData, "REPO_URL_PLACEHOLDER", p.cfg.RepoURL)
		if err := p.lxd.Create(session.Username, userData); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create: " + err.Error()})
			return
		}
		if err := p.lxd.Start(session.Username); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "start: " + err.Error()})
			return
		}
	case StatusStopped:
		if err := p.lxd.Start(session.Username); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "start: " + err.Error()})
			return
		}
	case StatusRunning:
		// そのまま
	default:
		writeJSON(w, http.StatusConflict, map[string]string{"error": "container is starting"})
		return
	}

	// コンテナの IP を取得し、Caddy の転送先を含む URL を返す。
	// Caddy が /proxy/<app>/<ip>/* を <ip>:<port> に転送する。
	ip, err := p.lxd.ContainerIP(session.Username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	proxyURL := "/proxy/" + app + "/" + ip
	// VS Code は / アクセス時に ./?folder=... へ相対リダイレクトして 404 になりがち。
	// ?folder= を最初から指定するとワークスペースが直接開く。
	if app == "vscode" {
		proxyURL += "/?folder=/home/osgsuken/workspace"
	}
	// デスクトップ (noVNC) は接続先をクエリで指定する。
	// host はポートを含めない (noVNC が自動でポートを付与するため 2重になる)。
	// port を明示して、ws://<host>:<port>/... を生成させる。
	if app == "desktop" {
		host := r.Host
		if h, _, err := net.SplitHostPort(r.Host); err == nil {
			host = h
		}
		proxyURL += "/vnc.html?autoconnect=1&host=" + host + "&port=7080&path=/proxy/desktop/" + ip + "/websockify"
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"proxy_url": proxyURL,
	})
}

// handleAPIHeartbeat はブラウザからの heartbeat を受け取る。
func (p *PortalServer) handleAPIHeartbeat(w http.ResponseWriter, r *http.Request) {
	session := p.sessionFromRequest(r)
	if session == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not_authenticated"})
		return
	}
	// LastSeen は Get() 内で更新される
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleAuthCheck は Caddy の forward_auth 用セッション検証エンドポイント。
// セッションが有効なら 200 (Caddy が転送を続行)、無効なら 401 を返す。
// さらにプロキシ先のコンテナ IP をヘッダーで Caddy に伝える。
func (p *PortalServer) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	session := p.sessionFromRequest(r)
	if session == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not_authenticated"})
		return
	}
	w.Header().Set("X-Osgsuken-User", session.Username)
	w.WriteHeader(http.StatusOK)
}

// ============================================================================
// 自動シャットダウン
// ============================================================================

// startIdleWatcher は最終アクティビティから IdleTimeout 経過したコンテナを
// 自動停止する。1分ごとにチェック。
func (p *PortalServer) startIdleWatcher(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.checkIdleContainers()
			}
		}
	}()
}

func (p *PortalServer) checkIdleContainers() {
	now := time.Now()
	for _, ps := range p.sessions.All() {
		if now.Sub(ps.LastSeen) < p.cfg.IdleTimeout {
			continue
		}
		status, err := p.lxd.Status(ps.Username)
		if err != nil || status != StatusRunning {
			continue
		}
		log.Printf("[idle] stopping container for %s (idle %s)", ps.Username, now.Sub(ps.LastSeen).Round(time.Minute))
		if err := p.lxd.Stop(ps.Username); err != nil {
			log.Printf("[idle] failed to stop %s: %v", ps.Username, err)
		}
	}
}

// ============================================================================
// エントリポイント
// ============================================================================

func main() {
	cfg := defaultConfig()

	// 環境変数での上書き
	if v := os.Getenv("PORTAL_ADDR"); v != "" {
		cfg.PortalAddr = v
	}
	if v := os.Getenv("OIDC_ADDR"); v != "" {
		cfg.OIDCAddr = v
	}
	if v := os.Getenv("OIDC_ISSUER"); v != "" {
		cfg.Issuer = v
	}
	if v := os.Getenv("OIDC_CLIENT_ID"); v != "" {
		cfg.ClientID = v
	}
	if v := os.Getenv("OIDC_CLIENT_SECRET"); v != "" {
		cfg.ClientSecret = v
	}
	if v := os.Getenv("REDIRECT_URI"); v != "" {
		cfg.RedirectURI = v
	}
	if v := os.Getenv("OIDC_MOCK"); v != "" {
		cfg.MockMode = v == "true" || v == "1"
	}
	if v := os.Getenv("REPO_URL"); v != "" {
		cfg.RepoURL = v
	}
	if v := os.Getenv("IDLE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.IdleTimeout = d
		}
	}

	// LXD 接続
	lxd, err := NewContainerManager("/var/snap/lxd/common/lxd/unix.socket", cfg.LXDImage)
	if err != nil {
		log.Fatalf("failed to connect LXD: %v", err)
	}

	// OIDC プロバイダ (開発用)
	oidcCfg := &OIDCConfig{
		Issuer: cfg.Issuer,
		Clients: []ClientConfig{{
			ID:           cfg.ClientID,
			SecretEnv:    "OIDC_CLIENT_SECRET",
			RedirectURIs: []string{cfg.RedirectURI},
		}},
		MockMode: cfg.MockMode,
	}
	store := NewStore()
	jwtMgr, err := NewJWTManager(cfg.Issuer)
	if err != nil {
		log.Fatalf("failed to init JWT manager: %v", err)
	}
	oidcServer := NewOIDCServer(oidcCfg, store, jwtMgr)

	// OIDC サーバー起動
	oidcMux := http.NewServeMux()
	oidcServer.Register(oidcMux)
	go func() {
		log.Printf("[oidc] listening on %s (issuer: %s)", cfg.OIDCAddr, cfg.Issuer)
		if err := http.ListenAndServe(cfg.OIDCAddr, oidcMux); err != nil {
			log.Fatalf("oidc server: %v", err)
		}
	}()

	// OIDC クライアント (ポータル側)
	client, err := NewPortalOIDCClient(cfg)
	if err != nil {
		log.Fatalf("failed to init OIDC client: %v", err)
	}

	// ポータル
	portal := &PortalServer{
		cfg:      cfg,
		lxd:      lxd,
		oidc:     oidcServer,
		client:   client,
		jwt:      jwtMgr,
		sessions: NewPortalSessionStore(),
	}

	portalMux := http.NewServeMux()
	portal.RegisterRoutes(portalMux)

	// 自動シャットダウン監視
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	portal.startIdleWatcher(ctx)

	// OIDC ストアの定期クリーンアップ
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			store.Cleanup()
		}
	}()

	log.Printf("[portal] listening on %s", cfg.PortalAddr)
	if err := http.ListenAndServe(cfg.PortalAddr, portalMux); err != nil {
		log.Fatalf("portal server: %v", err)
	}
}

// 後で使うための import 回避
var _ = json.Marshal
var _ = fmt.Sprintf
