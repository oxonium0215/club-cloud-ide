package main

// Discord OAuth2 ログインのハンドラ。
// フロー:
//   /login/discord        → Discord の認可ページへリダイレクト
//   /auth/discord/callback → コード交換 → ユーザー/ギルド情報取得 →
//                            部員判定 (Guild ID) → ポータルセッション確立

import (
	"log"
	"net/http"
)

// handleDiscordLoginRedirect は Discord 認可ページへリダイレクトする。
func (p *PortalServer) handleDiscordLoginRedirect(w http.ResponseWriter, r *http.Request) {
	if p.discord == nil || !p.discord.Enabled() {
		writeErrorPage(w, http.StatusServiceUnavailable, "認証エラー", "Discord ログインは設定されていません。", "", "")
		return
	}
	http.Redirect(w, r, p.discord.AuthCodeURL(), http.StatusFound)
}

// handleDiscordCallback は Discord OAuth2 のコールバックを処理する。
func (p *PortalServer) handleDiscordCallback(w http.ResponseWriter, r *http.Request) {
	if p.discord == nil || !p.discord.Enabled() {
		writeErrorPage(w, http.StatusServiceUnavailable, "認証エラー", "Discord ログインは設定されていません。", "", "")
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" {
		writeErrorPage(w, http.StatusBadRequest, "認証エラー", "認証コードがありません。もう一度お試しください。", "", "")
		return
	}
	if !p.discord.VerifyState(state) {
		writeErrorPage(w, http.StatusBadRequest, "認証エラー", "セキュリティ検証に失敗しました。もう一度お試しください。", "", "")
		return
	}

	// アクセストークン交換
	accessToken, err := p.discord.Exchange(code)
	if err != nil {
		log.Printf("discord token exchange failed: %v", err)
		writeErrorPage(w, http.StatusInternalServerError, "認証エラー", "Discord との接続に失敗しました。もう一度お試しください。", "", "")
		return
	}

	// ユーザー情報取得
	user, err := p.discord.FetchUser(accessToken)
	if err != nil {
		log.Printf("discord fetch user failed: %v", err)
		writeErrorPage(w, http.StatusInternalServerError, "認証エラー", "ユーザー情報を取得できませんでした。", "", "")
		return
	}

	// 部員判定: 指定 Guild のメンバーか確認
	member, err := p.discord.IsGuildMember(accessToken)
	if err != nil {
		log.Printf("discord guild check failed: %v", err)
		writeErrorPage(w, http.StatusInternalServerError, "認証エラー", "所属サーバーを確認できませんでした。", "", "")
		return
	}
	if !member {
		writeErrorPage(w, http.StatusForbidden, "アクセス拒否", "部活動の Discord サーバーに参加していないため、利用できません。", "", "")
		return
	}

	// ポータルセッション用 JWT を発行 (OIDC セッションと同じ形式)
	username := normalizeDiscordUsername(user.Username)
	session := &Session{
		Sub:               user.ID,
		PreferredUsername: username,
		Name:              user.DisplayName(),
		Groups:            []string{"students"},
	}
	token, err := p.jwt.SignAccessToken(session, p.cfg.ClientID)
	if err != nil {
		log.Printf("discord session sign failed: %v", err)
		writeErrorPage(w, http.StatusInternalServerError, "認証エラー", "セッションの作成に失敗しました。", "", "")
		return
	}

	// セッション確立 (OIDC フローと同じ)
	ps := &PortalSession{
		Token:    token,
		Username: username,
		Name:     user.DisplayName(),
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
