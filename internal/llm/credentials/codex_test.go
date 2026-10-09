package credentials

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fakeJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func writeCodexFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCodexAccessTokenValid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	access := fakeJWT(t, map[string]any{"exp": time.Now().Add(time.Hour).Unix(), "email": "u@example.com"})
	writeCodexFile(t, path, `{"auth_mode":"chatgpt","tokens":{"access_token":"`+access+`","refresh_token":"rt","account_id":"acct"}}`)

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer srv.Close()

	c := &Codex{path: path, tokenURL: srv.URL}
	token, err := c.AccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != access {
		t.Fatal("token mismatch")
	}
	if called {
		t.Fatal("refresh should not be called for a valid token")
	}
	if c.AccountID() != "acct" {
		t.Fatalf("account id = %q, want acct", c.AccountID())
	}
}

func TestCodexRefreshExpired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	expired := fakeJWT(t, map[string]any{"exp": time.Now().Add(-time.Hour).Unix()})
	writeCodexFile(t, path, `{"auth_mode":"chatgpt","last_refresh":"2026-01-01T00:00:00Z","tokens":{"access_token":"`+expired+`","refresh_token":"rt-old","id_token":"","account_id":"acct"}}`)

	newAccess := fakeJWT(t, map[string]any{"exp": time.Now().Add(time.Hour).Unix()})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["grant_type"] != "refresh_token" || body["refresh_token"] != "rt-old" || body["client_id"] != codexClientID {
			t.Errorf("refresh body = %v", body)
		}
		_, _ = w.Write([]byte(`{"access_token":"` + newAccess + `","refresh_token":"rt-new","id_token":"","expires_in":3600}`))
	}))
	defer srv.Close()

	c := &Codex{path: path, tokenURL: srv.URL}
	token, err := c.AccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != newAccess {
		t.Fatalf("token = %q, want refreshed", token)
	}
	data, _ := os.ReadFile(path)
	var root map[string]any
	_ = json.Unmarshal(data, &root)
	tokens, _ := root["tokens"].(map[string]any)
	if tokens["access_token"] != newAccess || tokens["refresh_token"] != "rt-new" {
		t.Fatalf("persisted tokens = %v", tokens)
	}
	if root["last_refresh"] == "" || root["last_refresh"] == "2026-01-01T00:00:00Z" {
		t.Fatalf("last_refresh not updated: %v", root["last_refresh"])
	}
}

func TestCodexNotLoggedIn(t *testing.T) {
	c := &Codex{path: filepath.Join(t.TempDir(), "missing.json"), tokenURL: "http://unused"}
	if _, err := c.AccessToken(context.Background()); !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("err = %v, want ErrNotLoggedIn", err)
	}
}
