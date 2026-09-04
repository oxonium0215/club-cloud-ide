package main

// DiscordOAuthClient は Discord OAuth2 (Authorization Code Flow) クライアント。
// Discord は OIDC 非対応のため、OAuth2 + API (users/@me) でユーザー情報を取得する。
// 部員判定は Guild ID (Discord サーバー ID) のメンバーであることで行う。

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DiscordConfig は Discord 連携の設定。
type DiscordConfig struct {
	Enabled      bool     // Discord ログインを有効にするか
	ClientID     string   // Discord Developer Portal の Client ID
	ClientSecret string   // Discord Developer Portal の Client Secret
	RedirectURI  string   // コールバック URL (http://.../auth/discord/callback)
	GuildID      string   // 部員が所属する Discord サーバー (Guild) の ID
	AllowedRoles []string // 省略可: 許可するロール ID (空なら Guild 全メンバーを許可)
}

const (
	discordAuthorizeURL = "https://discord.com/oauth2/authorize"
	discordTokenURL     = "https://discord.com/api/oauth2/token"
	discordMeURL        = "https://discord.com/api/users/@me"
	discordGuildsURL    = "https://discord.com/api/users/@me/guilds"
)

// DiscordUser は Discord API (users/@me) が返すユーザー情報。
type DiscordUser struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	GlobalName    string `json:"global_name"`
	Discriminator string `json:"discriminator"`
	Avatar        string `json:"avatar"`
}

// DiscordGuild は Discord API (users/@me/guilds) が返すギルド情報。
type DiscordGuild struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DiscordOAuthClient は Discord OAuth2 フローを実装する。
type DiscordOAuthClient struct {
	cfg        *DiscordConfig
	stateMu    sync.Mutex
	pending    map[string]bool
	httpClient *http.Client
}

func NewDiscordOAuthClient(cfg *DiscordConfig) *DiscordOAuthClient {
	return &DiscordOAuthClient{
		cfg:        cfg,
		pending:    make(map[string]bool),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Enabled は Discord ログインが有効かを返す。
func (c *DiscordOAuthClient) Enabled() bool {
	return c.cfg != nil && c.cfg.Enabled && c.cfg.ClientID != "" && c.cfg.GuildID != ""
}

// AuthCodeURL は Discord の認可 URL を生成する (state は CSRF 対策)。
// scope: identify (ユーザー情報) + guilds (所属サーバー判定)
func (c *DiscordOAuthClient) AuthCodeURL() string {
	stateBytes := make([]byte, 32)
	rand.Read(stateBytes)
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	c.stateMu.Lock()
	c.pending[state] = true
	c.stateMu.Unlock()

	params := url.Values{
		"client_id":     {c.cfg.ClientID},
		"redirect_uri":  {c.cfg.RedirectURI},
		"response_type": {"code"},
		"scope":         {"identify guilds"},
		"state":         {state},
	}
	return discordAuthorizeURL + "?" + params.Encode()
}

// VerifyState は state を検証し、使い捨てにする。
func (c *DiscordOAuthClient) VerifyState(state string) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if !c.pending[state] {
		return false
	}
	delete(c.pending, state)
	return true
}

// Exchange は認可コードをアクセストークンと交換する。
func (c *DiscordOAuthClient) Exchange(code string) (string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
		"redirect_uri":  {c.cfg.RedirectURI},
	}
	resp, err := c.httpClient.PostForm(discordTokenURL, form)
	if err != nil {
		return "", fmt.Errorf("discord token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("discord token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", err
	}
	return tokenResp.AccessToken, nil
}

// FetchUser はアクセストークンでユーザー情報を取得する。
func (c *DiscordOAuthClient) FetchUser(accessToken string) (*DiscordUser, error) {
	req, _ := http.NewRequest("GET", discordMeURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discord me request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discord me returned %d", resp.StatusCode)
	}
	var user DiscordUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}
	return &user, nil
}

// FetchGuilds はアクセストークンで所属ギルド一覧を取得する。
func (c *DiscordOAuthClient) FetchGuilds(accessToken string) ([]DiscordGuild, error) {
	req, _ := http.NewRequest("GET", discordGuildsURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discord guilds request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discord guilds returned %d", resp.StatusCode)
	}
	var guilds []DiscordGuild
	if err := json.NewDecoder(resp.Body).Decode(&guilds); err != nil {
		return nil, err
	}
	return guilds, nil
}

// IsGuildMember は対象の Guild ID に所属しているかを判定する (部員認証)。
func (c *DiscordOAuthClient) IsGuildMember(accessToken string) (bool, error) {
	guilds, err := c.FetchGuilds(accessToken)
	if err != nil {
		return false, err
	}
	for _, g := range guilds {
		if g.ID == c.cfg.GuildID {
			return true, nil
		}
	}
	return false, nil
}

// DisplayName は UI 表示用の名前を返す。
func (u *DiscordUser) DisplayName() string {
	if u.GlobalName != "" {
		return u.GlobalName
	}
	return u.Username
}

// normalizeDiscordUsername は Discord の username をコンテナ名用に正規化する。
// LXD コンテナ名は英小文字・数字・ハイフンのみ許可される。
func normalizeDiscordUsername(username string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(username) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "member"
	}
	return out
}
