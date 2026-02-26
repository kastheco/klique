package mcpclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kastheco/kasmos/internal/mcpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOAuthToken_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")

	tok := &mcpclient.OAuthToken{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		ExpiresAt:    time.Now().Add(time.Hour).Truncate(time.Second),
	}
	require.NoError(t, mcpclient.SaveToken(path, tok))

	loaded, err := mcpclient.LoadToken(path)
	require.NoError(t, err)
	assert.Equal(t, "access-123", loaded.AccessToken)
	assert.Equal(t, "refresh-456", loaded.RefreshToken)
	assert.WithinDuration(t, tok.ExpiresAt, loaded.ExpiresAt, time.Second)
}

func TestOAuthToken_SaveCreatesDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "dir", "token.json")

	tok := &mcpclient.OAuthToken{AccessToken: "tok"}
	require.NoError(t, mcpclient.SaveToken(path, tok))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestOAuthToken_IsExpired(t *testing.T) {
	tests := []struct {
		name    string
		offset  time.Duration
		expired bool
	}{
		{"expired", -time.Minute, true},
		{"valid", time.Hour, false},
		{"just expired", -time.Second, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok := &mcpclient.OAuthToken{ExpiresAt: time.Now().Add(tt.offset)}
			assert.Equal(t, tt.expired, tok.IsExpired())
		})
	}
}

func TestOAuthToken_LoadMissing(t *testing.T) {
	_, err := mcpclient.LoadToken("/nonexistent/path/token.json")
	assert.Error(t, err)
}

func TestPKCEChallenge(t *testing.T) {
	verifier, challenge := mcpclient.GeneratePKCE()
	assert.NotEmpty(t, verifier)
	assert.NotEmpty(t, challenge)
	assert.NotEqual(t, verifier, challenge)
	// Verifier should be 43-128 chars per RFC 7636
	assert.GreaterOrEqual(t, len(verifier), 43)
	assert.LessOrEqual(t, len(verifier), 128)
}

func TestPKCEChallenge_Unique(t *testing.T) {
	v1, c1 := mcpclient.GeneratePKCE()
	v2, c2 := mcpclient.GeneratePKCE()
	assert.NotEqual(t, v1, v2, "verifiers should be unique")
	assert.NotEqual(t, c1, c2, "challenges should be unique")
}

func TestOAuthFlow_ExchangesCode(t *testing.T) {
	// Mock token endpoint
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		require.NoError(t, r.ParseForm())
		assert.NotEmpty(t, r.FormValue("code"))
		assert.NotEmpty(t, r.FormValue("code_verifier"))
		assert.Equal(t, "authorization_code", r.FormValue("grant_type"))

		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "test-access-token",
			"refresh_token": "test-refresh-token",
			"expires_in":    3600,
		})
	}))
	defer tokenSrv.Close()

	cfg := mcpclient.OAuthConfig{
		AuthURL:     "http://localhost/auth",
		TokenURL:    tokenSrv.URL,
		ClientID:    "test-client",
		RedirectURI: "",
	}

	// openBrowser simulates the OAuth callback by parsing the redirect_uri
	// from the auth URL and hitting it with a code parameter.
	openBrowser := func(authURL string) error {
		u, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		redirectURI := u.Query().Get("redirect_uri")
		resp, err := http.Get(redirectURI + "?code=test-auth-code")
		if err != nil {
			return err
		}
		resp.Body.Close()
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tok, err := mcpclient.OAuthFlow(ctx, cfg, openBrowser)
	require.NoError(t, err)
	assert.Equal(t, "test-access-token", tok.AccessToken)
	assert.Equal(t, "test-refresh-token", tok.RefreshToken)
	assert.False(t, tok.IsExpired())
}

func TestOAuthFlow_Timeout(t *testing.T) {
	cfg := mcpclient.OAuthConfig{
		AuthURL:     "http://localhost/auth",
		TokenURL:    "http://localhost/token",
		ClientID:    "test-client",
		RedirectURI: "",
	}

	// openBrowser does nothing — simulates user never authorizing
	openBrowser := func(authURL string) error { return nil }

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := mcpclient.OAuthFlow(ctx, cfg, openBrowser)
	assert.Error(t, err)
}
