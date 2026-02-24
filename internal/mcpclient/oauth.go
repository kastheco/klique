package mcpclient

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// OAuthToken holds cached OAuth credentials.
type OAuthToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// IsExpired returns true if the token has expired.
func (t *OAuthToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// TokenPath returns the default path for cached ClickUp OAuth tokens.
func TokenPath() string {
	configDir, _ := os.UserConfigDir()
	return filepath.Join(configDir, "kasmos", "clickup_oauth.json")
}

// SaveToken writes a token to disk with restrictive permissions.
func SaveToken(path string, tok *OAuthToken) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// LoadToken reads a token from disk.
func LoadToken(path string) (*OAuthToken, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tok OAuthToken
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

// GeneratePKCE returns a code verifier and its S256 challenge per RFC 7636.
func GeneratePKCE() (verifier, challenge string) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	verifier = base64.RawURLEncoding.EncodeToString(buf)
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return
}

// OAuthConfig holds the OAuth application settings for the PKCE flow.
type OAuthConfig struct {
	AuthURL  string // Authorization endpoint (e.g. "https://app.clickup.com/api")
	TokenURL string // Token exchange endpoint
	ClientID string
}

// OAuthFlow performs the browser-based OAuth 2.1 PKCE flow and returns a token.
// The openBrowser callback is injectable for testing; pass nil for default behavior.
func OAuthFlow(ctx context.Context, cfg OAuthConfig, openBrowser func(string) error) (*OAuthToken, error) {
	verifier, challenge := GeneratePKCE()

	// Start local callback server on an ephemeral port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code in callback: %s", r.URL.RawQuery)
			fmt.Fprint(w, "Error: no authorization code received")
			return
		}
		codeCh <- code
		fmt.Fprint(w, "Authorization successful! You can close this tab.")
	})

	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(listener) }()
	defer func() { _ = srv.Shutdown(ctx) }()

	// Build authorization URL with PKCE parameters.
	params := url.Values{
		"client_id":             {cfg.ClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	authURL := cfg.AuthURL + "?" + params.Encode()

	if openBrowser == nil {
		openBrowser = defaultOpenBrowser
	}
	if err := openBrowser(authURL); err != nil {
		return nil, fmt.Errorf("open browser: %w", err)
	}

	// Wait for the OAuth callback or timeout.
	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return exchangeCode(cfg, code, verifier, redirectURI)
}

// exchangeCode trades an authorization code for an access token.
func exchangeCode(cfg OAuthConfig, code, verifier, redirectURI string) (*OAuthToken, error) {
	data := url.Values{
		"client_id":     {cfg.ClientID},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}
	resp, err := http.PostForm(cfg.TokenURL, data)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	if result.AccessToken == "" {
		return nil, fmt.Errorf("empty access token in response")
	}
	return &OAuthToken{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(result.ExpiresIn) * time.Second),
	}, nil
}

// defaultOpenBrowser opens the system browser.
func defaultOpenBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", rawURL)
	case "darwin":
		cmd = exec.Command("open", rawURL)
	default:
		return fmt.Errorf("unsupported OS for browser open: %s", runtime.GOOS)
	}
	return cmd.Start()
}
