package main

// OIDC プロバイダ実装 (OUCC oidc-bridge の設計を踏襲)
// - PKCE (S256) 対応
// - go-jose/v4 による RS256 JWT 署名
// - インメモリストア (state → AuthRequest, code → Session) + 定期クリーンアップ
// - 開発モードでは mock ユーザー選択ページを表示

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// ============================================================================
// 設定
// ============================================================================

// ClientConfig は OIDC クライアント (このポータル自身) の設定。
type ClientConfig struct {
	ID           string   `yaml:"id"`
	SecretEnv    string   `yaml:"secret_env"`
	RedirectURIs []string `yaml:"redirect_uris"`
}

// OIDCConfig はプロバイダ設定。
type OIDCConfig struct {
	Issuer  string         `yaml:"issuer"`
	Clients []ClientConfig `yaml:"clients"`
	// MockMode が true の場合、Discord の代わりに開発用ユーザー選択ページを使う
	MockMode bool `yaml:"mock_mode"`
}

// ============================================================================
// インメモリストア
// ============================================================================

// AuthRequest は authorize リクエストの保存 (state で紐付け)。
type AuthRequest struct {
	ClientID            string    `json:"client_id"`
	RedirectURI         string    `json:"redirect_uri"`
	State               string    `json:"state"`
	Nonce               string    `json:"nonce"`
	CodeChallenge       string    `json:"code_challenge"`
	CodeChallengeMethod string    `json:"code_challenge_method"`
	CreatedAt           time.Time `json:"created_at"`
}

// Session は認証済みユーザー (code で紐付け)。
type Session struct {
	Sub               string   `json:"sub"`
	PreferredUsername string   `json:"preferred_username"`
	Name              string   `json:"name"`
	Email             string   `json:"email"`
	Groups            []string `json:"groups"`
	Nonce             string   `json:"nonce"`
	CreatedAt         time.Time `json:"created_at"`
}

// Store は authRequests と codes を保持するスレッドセーフなストア。
type Store struct {
	mu           sync.Mutex
	authRequests map[string]*AuthRequest // state → request
	codes        map[string]*Session     // code → session
}

func NewStore() *Store {
	return &Store{
		authRequests: make(map[string]*AuthRequest),
		codes:        make(map[string]*Session),
	}
}

func (s *Store) SaveAuthRequest(state string, req *AuthRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	req.CreatedAt = time.Now()
	s.authRequests[state] = req
}

func (s *Store) PeekAuthRequest(state string) *AuthRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authRequests[state]
}

func (s *Store) GetAuthRequest(state string) *AuthRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	req, ok := s.authRequests[state]
	if !ok {
		return nil
	}
	delete(s.authRequests, state)
	return req
}

func (s *Store) SaveCode(code string, session *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session.CreatedAt = time.Now()
	s.codes[code] = session
}

func (s *Store) GetSession(code string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.codes[code]
	if !ok {
		return nil
	}
	delete(s.codes, code)
	return session
}

// Cleanup は期限切れデータを削除する (5分ごとに実行)。
func (s *Store) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, v := range s.authRequests {
		if now.Sub(v.CreatedAt) > 10*time.Minute {
			delete(s.authRequests, k)
		}
	}
	for k, v := range s.codes {
		if now.Sub(v.CreatedAt) > 10*time.Minute {
			delete(s.codes, k)
		}
	}
}

// ============================================================================
// JWT 署名・検証 (go-jose/v4)
// ============================================================================

// JWTManager は RS256 による ID Token / Access Token の署名と検証を担当する。
type JWTManager struct {
	key    *rsa.PrivateKey
	signer jose.Signer
	keyID  string
	issuer string
}

// loadOrGenerateKey は PEM の RSA 秘密鍵を読み込む。未設定なら生成する。
func loadOrGenerateKey(keyPEM string) (*rsa.PrivateKey, error) {
	if keyPEM != "" {
		block, _ := pem.Decode([]byte(keyPEM))
		if block == nil {
			return nil, fmt.Errorf("failed to decode PEM signing key")
		}
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("failed to parse private key: %w", err)
			}
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("signing key is not an RSA private key")
		}
		return rsaKey, nil
	}
	return rsa.GenerateKey(rand.Reader, 2048)
}

func NewJWTManager(issuer string) (*JWTManager, error) {
	key, err := loadOrGenerateKey(os.Getenv("OIDC_SIGNING_KEY"))
	if err != nil {
		return nil, fmt.Errorf("failed to load signing key: %w", err)
	}

	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: key},
		&jose.SignerOptions{
			ExtraHeaders: map[jose.HeaderKey]interface{}{"kid": "osgsuken-signing-key"},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}

	return &JWTManager{
		key:    key,
		signer: signer,
		keyID:  "osgsuken-signing-key",
		issuer: issuer,
	}, nil
}

func (j *JWTManager) JWKS() ([]byte, error) {
	jwks := jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{{
			Key:       &j.key.PublicKey,
			KeyID:     j.keyID,
			Algorithm: "RS256",
			Use:       "sig",
		}},
	}
	return json.Marshal(jwks)
}

// IDTokenClaims は ID Token のクレーム。
type IDTokenClaims struct {
	jwt.Claims
	Nonce             string   `json:"nonce,omitempty"`
	PreferredUsername string   `json:"preferred_username,omitempty"`
	Name              string   `json:"name,omitempty"`
	Email             string   `json:"email,omitempty"`
	EmailVerified     bool     `json:"email_verified"`
	Groups            []string `json:"groups,omitempty"`
}

// AccessTokenClaims は Access Token (opaque JWT) のクレーム。
type AccessTokenClaims struct {
	jwt.Claims
	PreferredUsername string   `json:"preferred_username,omitempty"`
	Name              string   `json:"name,omitempty"`
	Email             string   `json:"email,omitempty"`
	Groups            []string `json:"groups,omitempty"`
}

func (j *JWTManager) SignIDToken(session *Session, clientID, nonce string) (string, error) {
	now := time.Now()
	claims := IDTokenClaims{
		Claims: jwt.Claims{
			Issuer:   j.issuer,
			Subject:  session.Sub,
			Audience: jwt.Audience{clientID},
			IssuedAt: jwt.NewNumericDate(now),
			Expiry:   jwt.NewNumericDate(now.Add(1 * time.Hour)),
		},
		Nonce:             nonce,
		PreferredUsername: session.PreferredUsername,
		Name:              session.Name,
		Email:             session.Email,
		EmailVerified:     true,
		Groups:            session.Groups,
	}
	return jwt.Signed(j.signer).Claims(claims).Serialize()
}

func (j *JWTManager) SignAccessToken(session *Session, clientID string) (string, error) {
	now := time.Now()
	claims := AccessTokenClaims{
		Claims: jwt.Claims{
			Issuer:   j.issuer,
			Subject:  session.Sub,
			Audience: jwt.Audience{clientID},
			IssuedAt: jwt.NewNumericDate(now),
			Expiry:   jwt.NewNumericDate(now.Add(1 * time.Hour)),
		},
		PreferredUsername: session.PreferredUsername,
		Name:              session.Name,
		Email:             session.Email,
		Groups:            session.Groups,
	}
	return jwt.Signed(j.signer).Claims(claims).Serialize()
}

func (j *JWTManager) ParseAccessToken(tokenStr string) (*AccessTokenClaims, error) {
	token, err := jwt.ParseSigned(tokenStr, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		return nil, err
	}
	claims := &AccessTokenClaims{}
	if err := token.Claims(&j.key.PublicKey, claims); err != nil {
		return nil, err
	}
	return claims, nil
}

// ============================================================================
// Mock ユーザー (開発用)
// ============================================================================

type MockUser struct {
	Username string
	Name     string
	Email    string
}

var mockUsers = []MockUser{
	{Username: "tanaka", Name: "田中 太郎", Email: "tanaka@school.ed.jp"},
	{Username: "suzuki", Name: "鈴木 花子", Email: "suzuki@school.ed.jp"},
	{Username: "sato", Name: "佐藤 健一", Email: "sato@school.ed.jp"},
	{Username: "advisor", Name: "情報科 顧問 (管理者)", Email: "advisor@school.ed.jp"},
}

// ============================================================================
// ユーティリティ
// ============================================================================

func generateRandomString(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)[:length]
}

func getClientConfig(cfg *OIDCConfig, clientID string) *ClientConfig {
	for i := range cfg.Clients {
		if cfg.Clients[i].ID == clientID {
			return &cfg.Clients[i]
		}
	}
	return nil
}

func getClientSecret(c *ClientConfig) string {
	return os.Getenv(c.SecretEnv)
}

func validateRedirectURI(allowed []string, redirectURI string) bool {
	for _, uri := range allowed {
		if uri == redirectURI {
			return true
		}
	}
	return false
}

// writeJSON は JSON レスポンスを書く。
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ============================================================================
// OIDC Discovery
// ============================================================================

type DiscoveryResponse struct {
	Issuer                           string   `json:"issuer"`
	AuthorizationEndpoint            string   `json:"authorization_endpoint"`
	TokenEndpoint                    string   `json:"token_endpoint"`
	UserInfoEndpoint                 string   `json:"userinfo_endpoint"`
	JWKSUri                          string   `json:"jwks_uri"`
	ScopesSupported                  []string `json:"scopes_supported"`
	ResponseTypesSupported           []string `json:"response_types_supported"`
	GrantTypesSupported              []string `json:"grant_types_supported"`
	SubjectTypesSupported            []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported []string `json:"id_token_signing_alg_values_supported"`
	ClaimsSupported                  []string `json:"claims_supported"`
	CodeChallengeMethodsSupported    []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
}

func discoveryJSON(issuer string) DiscoveryResponse {
	return DiscoveryResponse{
		Issuer:                            issuer,
		AuthorizationEndpoint:             issuer + "/authorize",
		TokenEndpoint:                     issuer + "/token",
		UserInfoEndpoint:                  issuer + "/userinfo",
		JWKSUri:                           issuer + "/jwks",
		ScopesSupported:                   []string{"openid", "profile", "email", "groups"},
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code"},
		SubjectTypesSupported:             []string{"public"},
		IDTokenSigningAlgValuesSupported:  []string{"RS256"},
		ClaimsSupported:                   []string{"sub", "iss", "aud", "iat", "exp", "preferred_username", "name", "email", "email_verified", "groups"},
		TokenEndpointAuthMethodsSupported: []string{"client_secret_basic", "client_secret_post"},
		CodeChallengeMethodsSupported:     []string{"S256"},
	}
}
