package credentials

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const clineRefreshURL = "https://api.cline.bot/api/v1/auth/refresh"

type Cline struct {
	provider   string
	path       string
	refreshURL string
	mu         sync.Mutex
}

func NewCline(provider string) *Cline {
	return &Cline{provider: provider, path: clineProvidersPath(), refreshURL: clineRefreshURL}
}

func clineProvidersPath() string {
	if dir := strings.TrimSpace(os.Getenv("CLINE_DATA_DIR")); dir != "" {
		return filepath.Join(dir, "settings", "providers.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("settings", "providers.json")
	}
	return filepath.Join(home, ".cline", "data", "settings", "providers.json")
}

func (c *Cline) AccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var token string
	err := lockFile(c.path+".lock", func() error {
		root, auth, err := c.load()
		if err != nil {
			return err
		}
		access := stringField(auth, "accessToken")
		refresh := stringField(auth, "refreshToken")
		if access == "" && refresh == "" {
			return ErrNotLoggedIn
		}
		if refresh != "" && c.needsRefresh(auth) {
			tokens, err := refreshCline(ctx, c.refreshURL, refresh)
			if err != nil {
				return err
			}
			access = tokens.AccessToken
			auth["accessToken"] = tokens.AccessToken
			if tokens.RefreshToken != "" {
				auth["refreshToken"] = tokens.RefreshToken
			}
			auth["expiresAt"] = json.Number(fmt.Sprintf("%d", tokens.ExpiresAt))
			if err := c.save(root); err != nil {
				return err
			}
		}
		if access == "" {
			return ErrNotLoggedIn
		}
		token = access
		return nil
	})
	return token, err
}

func (c *Cline) Refresh(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return lockFile(c.path+".lock", func() error {
		root, auth, err := c.load()
		if err != nil {
			return err
		}
		refresh := stringField(auth, "refreshToken")
		if refresh == "" {
			return ErrNotLoggedIn
		}
		tokens, err := refreshCline(ctx, c.refreshURL, refresh)
		if err != nil {
			return err
		}
		auth["accessToken"] = tokens.AccessToken
		if tokens.RefreshToken != "" {
			auth["refreshToken"] = tokens.RefreshToken
		}
		auth["expiresAt"] = json.Number(fmt.Sprintf("%d", tokens.ExpiresAt))
		return c.save(root)
	})
}

func (c *Cline) Status() Status {
	_, auth, err := c.load()
	if err != nil {
		return Status{Provider: c.provider}
	}
	exp := time.Time{}
	if ms, ok := intField(auth, "expiresAt"); ok && ms > 0 {
		exp = time.UnixMilli(ms)
	}
	plan := "cline"
	if c.provider == "cline-pass" {
		plan = "cline-pass"
	}
	return Status{
		Provider:  c.provider,
		Account:   stringField(auth, "accountId"),
		Plan:      plan,
		ExpiresAt: exp,
		LoggedIn:  stringField(auth, "accessToken") != "" || stringField(auth, "refreshToken") != "",
	}
}

func (c *Cline) needsRefresh(auth map[string]any) bool {
	ms, ok := intField(auth, "expiresAt")
	if !ok || ms == 0 {
		return false
	}
	return time.Now().Add(refreshSkew).After(time.UnixMilli(ms))
}

func (c *Cline) load() (map[string]any, map[string]any, error) {
	data, err := os.ReadFile(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, ErrNotLoggedIn
		}
		return nil, nil, fmt.Errorf("credentials: read cline providers: %w", err)
	}
	root, err := decodeObject(data)
	if err != nil {
		return nil, nil, fmt.Errorf("credentials: parse cline providers: %w", err)
	}
	providers := objectField(root, "providers")
	auth := clineAuthEntry(providers, c.provider)
	if auth == nil && c.provider != "cline" {
		auth = clineAuthEntry(providers, "cline")
	}
	if auth == nil {
		return nil, nil, ErrNotLoggedIn
	}
	return root, auth, nil
}

func clineAuthEntry(providers map[string]any, name string) map[string]any {
	entry := objectField(providers, name)
	settings := objectField(entry, "settings")
	return objectField(settings, "auth")
}

func (c *Cline) save(root map[string]any) error {
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("credentials: marshal cline providers: %w", err)
	}
	return writeJSONAtomic(c.path, data)
}

type clineTokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    int64
}

func refreshCline(ctx context.Context, refreshURL, refreshToken string) (clineTokens, error) {
	body, _ := json.Marshal(map[string]any{
		"grantType":    "refresh_token",
		"refreshToken": refreshToken,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, refreshURL, bytes.NewReader(body))
	if err != nil {
		return clineTokens{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return clineTokens{}, fmt.Errorf("credentials: refresh cline token: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return clineTokens{}, fmt.Errorf("credentials: refresh cline token: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var parsed struct {
		Success bool `json:"success"`
		Data    struct {
			AccessToken  string          `json:"accessToken"`
			RefreshToken string          `json:"refreshToken"`
			ExpiresAt    json.RawMessage `json:"expiresAt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return clineTokens{}, fmt.Errorf("credentials: parse cline refresh response: %w", err)
	}
	if parsed.Data.AccessToken == "" {
		return clineTokens{}, fmt.Errorf("credentials: cline refresh response missing accessToken")
	}
	exp := int64(0)
	if len(parsed.Data.ExpiresAt) > 0 {
		var n int64
		if err := json.Unmarshal(parsed.Data.ExpiresAt, &n); err == nil {
			exp = n
		} else {
			var s string
			if err := json.Unmarshal(parsed.Data.ExpiresAt, &s); err == nil {
				if t, err := time.Parse(time.RFC3339, s); err == nil {
					exp = t.UnixMilli()
				}
			}
		}
	}
	if exp == 0 {
		exp = time.Now().Add(time.Hour).UnixMilli()
	}
	return clineTokens{
		AccessToken:  parsed.Data.AccessToken,
		RefreshToken: parsed.Data.RefreshToken,
		ExpiresAt:    exp,
	}, nil
}
