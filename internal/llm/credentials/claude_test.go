package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func writeClaudeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeAccessTokenValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".credentials.json")
	exp := time.Now().Add(time.Hour).UnixMilli()
	writeClaudeFile(t, path, `{"claudeAiOauth":{"accessToken":"tok-valid","refreshToken":"rt","expiresAt":`+itoa(exp)+`,"subscriptionType":"max","unknown":"keep"}}`)

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true }))
	defer srv.Close()

	c := &Claude{path: path, tokenURL: srv.URL}
	token, err := c.AccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "tok-valid" {
		t.Fatalf("token = %q, want tok-valid", token)
	}
	if called {
		t.Fatal("refresh endpoint should not be called for a valid token")
	}
}

func TestClaudeRefresh(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".credentials.json")
	expired := time.Now().Add(-time.Hour).UnixMilli()
	writeClaudeFile(t, path, `{"claudeAiOauth":{"accessToken":"tok-old","refreshToken":"rt-old","expiresAt":`+itoa(expired)+`,"subscriptionType":"max","rateLimitTier":"x","unknown":{"a":1}}}`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content-type = %q", got)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["grant_type"] != "refresh_token" || body["refresh_token"] != "rt-old" {
			t.Errorf("refresh body = %v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-new","refresh_token":"rt-new","expires_in":3600}`))
	}))
	defer srv.Close()

	c := &Claude{path: path, tokenURL: srv.URL}
	token, err := c.AccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "tok-new" {
		t.Fatalf("token = %q, want tok-new", token)
	}

	data, _ := os.ReadFile(path)
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"unknown"`) || !strings.Contains(string(data), `"rateLimitTier"`) {
		t.Fatalf("unknown fields were dropped: %s", data)
	}
	var oauth map[string]json.RawMessage
	_ = json.Unmarshal(root["claudeAiOauth"], &oauth)
	if got := rawString(oauth, "accessToken"); got != "tok-new" {
		t.Fatalf("persisted accessToken = %q, want tok-new", got)
	}
	if got := rawString(oauth, "refreshToken"); got != "rt-new" {
		t.Fatalf("persisted refreshToken = %q, want rt-new", got)
	}
	n, ok := rawInt(oauth, "expiresAt")
	if !ok || n < 1e11 {
		t.Fatalf("persisted expiresAt = %d (ok=%v), want millisecond value", n, ok)
	}
}

func TestClaudeRefreshPreservesSecondsUnit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".credentials.json")
	expiredSec := time.Now().Add(-time.Hour).Unix()
	writeClaudeFile(t, path, `{"claudeAiOauth":{"accessToken":"tok-old","refreshToken":"rt","expiresAt":`+itoa(expiredSec)+`}}`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"tok-new","refresh_token":"rt2","expires_in":3600}`))
	}))
	defer srv.Close()

	c := &Claude{path: path, tokenURL: srv.URL}
	if _, err := c.AccessToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var root map[string]json.RawMessage
	_ = json.Unmarshal(data, &root)
	var oauth map[string]json.RawMessage
	_ = json.Unmarshal(root["claudeAiOauth"], &oauth)
	n, _ := rawInt(oauth, "expiresAt")
	if n >= 1e11 {
		t.Fatalf("expiresAt = %d, want a seconds value (< 1e11)", n)
	}
}

func TestClaudeNotLoggedIn(t *testing.T) {
	c := &Claude{path: filepath.Join(t.TempDir(), "missing.json"), tokenURL: "http://unused"}
	if _, err := c.AccessToken(context.Background()); !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("err = %v, want ErrNotLoggedIn", err)
	}
}

func TestClaudeStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".credentials.json")
	exp := time.Now().Add(time.Hour).UnixMilli()
	writeClaudeFile(t, path, `{"claudeAiOauth":{"accessToken":"tok","refreshToken":"rt","expiresAt":`+itoa(exp)+`,"subscriptionType":"pro","email":"u@example.com"}}`)

	st := (&Claude{path: path}).Status()
	if !st.LoggedIn || st.Plan != "pro" || st.Account != "u@example.com" {
		t.Fatalf("status = %+v", st)
	}
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
