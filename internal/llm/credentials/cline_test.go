package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeClineFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestClineAccessTokenValid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")
	exp := time.Now().Add(time.Hour).UnixMilli()
	writeClineFile(t, path, `{"version":1,"providers":{"cline":{"tokenSource":"oauth","settings":{"provider":"cline","auth":{"accessToken":"workos:tok","refreshToken":"rt","expiresAt":`+itoa(exp)+`,"accountId":"usr-1"}}}}}`)

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer srv.Close()

	c := &Cline{provider: "cline", path: path, refreshURL: srv.URL}
	token, err := c.AccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "workos:tok" {
		t.Fatalf("token = %q", token)
	}
	if called {
		t.Fatal("refresh should not be called")
	}
	if st := c.Status(); !st.LoggedIn || st.Account != "usr-1" {
		t.Fatalf("status = %+v", st)
	}
}

func TestClineRefreshExpired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")
	expired := time.Now().Add(-time.Hour).UnixMilli()
	writeClineFile(t, path, `{"version":1,"providers":{"cline":{"tokenSource":"oauth","settings":{"provider":"cline","auth":{"accessToken":"workos:old","refreshToken":"rt-old","expiresAt":`+itoa(expired)+`}}},"other":{"settings":{}}}}`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["refreshToken"] != "rt-old" || body["grantType"] != "refresh_token" {
			t.Errorf("refresh body = %v", body)
		}
		newExp := time.Now().Add(2 * time.Hour).UnixMilli()
		_, _ = w.Write([]byte(`{"success":true,"data":{"accessToken":"workos:new","refreshToken":"rt-new","expiresAt":` + itoa(newExp) + `}}`))
	}))
	defer srv.Close()

	c := &Cline{provider: "cline", path: path, refreshURL: srv.URL}
	token, err := c.AccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "workos:new" {
		t.Fatalf("token = %q, want refreshed", token)
	}
	data, _ := os.ReadFile(path)
	var root map[string]any
	_ = json.Unmarshal(data, &root)
	providers, _ := root["providers"].(map[string]any)
	if _, ok := providers["other"]; !ok {
		t.Fatalf("unrelated provider dropped: %v", providers)
	}
	entry, _ := providers["cline"].(map[string]any)
	settings, _ := entry["settings"].(map[string]any)
	auth, _ := settings["auth"].(map[string]any)
	if auth["accessToken"] != "workos:new" || auth["refreshToken"] != "rt-new" {
		t.Fatalf("persisted auth = %v", auth)
	}
}

func TestClineNotLoggedIn(t *testing.T) {
	c := &Cline{provider: "cline", path: filepath.Join(t.TempDir(), "missing.json"), refreshURL: "http://unused"}
	if _, err := c.AccessToken(context.Background()); !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("err = %v, want ErrNotLoggedIn", err)
	}
}

func TestClinePassFallsBackToClineAuth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")
	exp := time.Now().Add(time.Hour).UnixMilli()
	writeClineFile(t, path, `{"providers":{"cline":{"settings":{"auth":{"accessToken":"workos:tok","refreshToken":"rt","expiresAt":`+itoa(exp)+`}}}}}`)

	c := &Cline{provider: "cline-pass", path: path, refreshURL: "http://unused"}
	token, err := c.AccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "workos:tok" {
		t.Fatalf("token = %q, want the shared cline token", token)
	}
}
