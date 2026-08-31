package main

// PortalOIDCClient はポータルが OIDC プロバイダ (同じプロセス内) と
// Authorization Code Flow + PKCE で通信するためのクライアント。

import (
	"crypto/rand"
	"crypto/sha256"
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

// PortalToken はトークン交換の結果。
type PortalToken struct {
	AccessToken       string
	IDToken           string
	PreferredUsername string
	Name              string
	Email             string
}

// PortalOIDCClient は OIDC プロバイダへのクライアント。
type PortalOIDCClient struct {
	cfg              *Config
	stateMu          sync.Mutex
	pending          map[string]bool
	pendingVerifiers map[string]string
	httpClient       *http.Client
}

func NewPortalOIDCClient(cfg *Config) (*PortalOIDCClient, error) {
	return &PortalOIDCClient{
		cfg:              cfg,
		pending:          make(map[string]bool),
		pendingVerifiers: make(map[string]string),
		httpClient:       &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// AuthCodeURL は PKCE 付きの認証 URL を生成する。
func (c *PortalOIDCClient) AuthCodeURL() string {
	state := generateRandomString(32)

	verifier := make([]byte, 32)
	rand.Read(verifier)
	codeVerifier := base64.RawURLEncoding.EncodeToString(verifier)
	sum := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(sum[:])

	c.stateMu.Lock()
	c.pending[state] = true
	c.pendingVerifiers[state] = codeVerifier
	c.stateMu.Unlock()

	params := url.Values{
		"client_id":             {c.cfg.ClientID},
		"redirect_uri":          {c.cfg.RedirectURI},
		"response_type":         {"code"},
		"scope":                 {"openid profile email"},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
		"nonce":                 {state},
	}
	return c.cfg.Issuer + "/authorize?" + params.Encode()
}

// VerifyState は state を検証する。
func (c *PortalOIDCClient) VerifyState(state string) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if !c.pending[state] {
		return false
	}
	delete(c.pending, state)
	return true
}

// takeVerifier は state に紐づく code_verifier を取り出す。
func (c *PortalOIDCClient) takeVerifier(state string) string {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	v := c.pendingVerifiers[state]
	delete(c.pendingVerifiers, state)
	return v
}

// Exchange は認可コードをトークンと交換する (PKCE verifier 付き)。
func (c *PortalOIDCClient) Exchange(code, state string) (*PortalToken, error) {
	verifier := c.takeVerifier(state)

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
		"redirect_uri":  {c.cfg.RedirectURI},
	}
	if verifier != "" {
		form.Set("code_verifier", verifier)
	}

	resp, err := c.httpClient.PostForm(c.cfg.Issuer+"/token", form)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}

	username, name, email := decodeIDTokenClaims(tokenResp.IDToken)

	return &PortalToken{
		AccessToken:       tokenResp.AccessToken,
		IDToken:           tokenResp.IDToken,
		PreferredUsername: username,
		Name:              name,
		Email:             email,
	}, nil
}

// decodeIDTokenClaims は ID Token の JWT ペイロードからユーザー情報を取り出す。
func decodeIDTokenClaims(idToken string) (username, name, email string) {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return "", "", ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", ""
	}
	var claims struct {
		PreferredUsername string `json:"preferred_username"`
		Name              string `json:"name"`
		Email             string `json:"email"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", "", ""
	}
	return claims.PreferredUsername, claims.Name, claims.Email
}
