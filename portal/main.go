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
	"net/http"
	"net/http/httputil"
	"net/url"
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
	// プロキシ (認証必須)
	mux.HandleFunc("/proxy/", p.handleProxy)
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
func (p *PortalServer) sessionFromRequest(r *http.Request) *PortalSession {
	cookie, err := r.Cookie("osgsuken_session")
	if err != nil {
		return nil
	}
	return p.sessions.Get(cookie.Value)
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

	// 起動待ち (内部サービス readiness はプロキシ側でハンドル)
	writeJSON(w, http.StatusOK, map[string]string{
		"proxy_url": "/proxy/" + app,
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

// handleProxy はコンテナ内サービス (VS Code / KasmVNC) へのリバースプロキシ。
func (p *PortalServer) handleProxy(w http.ResponseWriter, r *http.Request) {
	session := p.sessionFromRequest(r)
	if session == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	// /proxy/vscode → コンテナの :13337
	// /proxy/desktop → コンテナの :6080
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/proxy/"), "/", 2)
	if len(parts) == 0 {
		http.NotFound(w, r)
		return
	}
	app := parts[0]
	rest := ""
	if len(parts) > 1 {
		rest = "/" + parts[1]
	}

	var port string
	switch app {
	case "vscode":
		port = "13337"
	case "desktop":
		port = "6080"
	default:
		http.NotFound(w, r)
		return
	}

	// コンテナが起動しているか確認
	status, err := p.lxd.Status(session.Username)
	if err != nil || status != StatusRunning {
		// 起動中画面を返す (JS が再試行する)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "container_not_ready"})
		return
	}

	// LXD コンテナの IP を解決してプロキシ
	ip, err := p.lxd.ContainerIP(session.Username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	target := &url.URL{Scheme: "http", Host: ip + ":" + port}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = rest
		if req.URL.RawQuery == "" {
			req.URL.RawQuery = r.URL.RawQuery
		}
		req.Host = target.Host
		// サブパス認識用 (code-server / KasmVNC が生成するURLにプレフィックスを含める)
		req.Header.Set("X-Forwarded-Prefix", "/proxy/"+app)
	}
	proxy.ServeHTTP(w, r)
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
