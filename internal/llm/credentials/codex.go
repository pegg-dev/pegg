package credentials

import (
	"bytes"
	"context"
	"encoding/base64"
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

const (
	codexTokenURL = "https://auth.openai.com/oauth/token"
	codexClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
)

type Codex struct {
	path     string
	tokenURL string
	mu       sync.Mutex
}

func NewCodex() *Codex {
	return &Codex{path: codexAuthPath(), tokenURL: codexTokenURL}
}

func codexAuthPath() string {
	if dir := strings.TrimSpace(os.Getenv("CODEX_HOME")); dir != "" {
		return filepath.Join(dir, "auth.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "auth.json"
	}
	return filepath.Join(home, ".codex", "auth.json")
}

func (c *Codex) AccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var token string
	err := lockFile(c.path+".lock", func() error {
		root, tokens, err := c.load()
		if err != nil {
			return err
		}
		access := stringField(tokens, "access_token")
		refresh := stringField(tokens, "refresh_token")
		if access == "" && refresh == "" {
			return ErrNotLoggedIn
		}
		exp := jwtExpiry(access)
		if refresh != "" && (exp.IsZero() || time.Now().Add(refreshSkew).After(exp)) {
			newTokens, err := refreshCodex(ctx, c.tokenURL, refresh)
			if err != nil {
				return err
			}
			access = newTokens.AccessToken
			applyCodexTokens(tokens, newTokens)
			root["tokens"] = tokens
			root["last_refresh"] = time.Now().UTC().Format(time.RFC3339Nano)
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

func (c *Codex) Refresh(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return lockFile(c.path+".lock", func() error {
		root, tokens, err := c.load()
		if err != nil {
			return err
		}
		refresh := stringField(tokens, "refresh_token")
		if refresh == "" {
			return ErrNotLoggedIn
		}
		newTokens, err := refreshCodex(ctx, c.tokenURL, refresh)
		if err != nil {
			return err
		}
		applyCodexTokens(tokens, newTokens)
		root["tokens"] = tokens
		root["last_refresh"] = time.Now().UTC().Format(time.RFC3339Nano)
		return c.save(root)
	})
}

func (c *Codex) AccountID() string {
	_, tokens, err := c.load()
	if err != nil {
		return ""
	}
	if id := stringField(tokens, "account_id"); id != "" {
		return id
	}
	return extractAccountID(stringField(tokens, "id_token"), stringField(tokens, "access_token"))
}

func (c *Codex) Status() Status {
	_, tokens, err := c.load()
	if err != nil {
		return Status{Provider: "codex"}
	}
	access := stringField(tokens, "access_token")
	return Status{
		Provider:  "codex",
		Account:   extractEmail(stringField(tokens, "id_token")),
		Plan:      "chatgpt",
		ExpiresAt: jwtExpiry(access),
		LoggedIn:  access != "" || stringField(tokens, "refresh_token") != "",
	}
}

func (c *Codex) load() (map[string]any, map[string]any, error) {
	data, err := os.ReadFile(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, ErrNotLoggedIn
		}
		return nil, nil, fmt.Errorf("credentials: read codex auth: %w", err)
	}
	root, err := decodeObject(data)
	if err != nil {
		return nil, nil, fmt.Errorf("credentials: parse codex auth: %w", err)
	}
	tokens := objectField(root, "tokens")
	if tokens == nil {
		return nil, nil, ErrNotLoggedIn
	}
	return root, tokens, nil
}

func (c *Codex) save(root map[string]any) error {
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("credentials: marshal codex auth: %w", err)
	}
	return writeJSONAtomic(c.path, data)
}

type codexTokens struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	ExpiresIn    int64
}

func refreshCodex(ctx context.Context, tokenURL, refreshToken string) (codexTokens, error) {
	body, _ := json.Marshal(map[string]any{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     codexClientID,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewReader(body))
	if err != nil {
		return codexTokens{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return codexTokens{}, fmt.Errorf("credentials: refresh codex token: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return codexTokens{}, fmt.Errorf("credentials: refresh codex token: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var parsed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return codexTokens{}, fmt.Errorf("credentials: parse codex refresh response: %w", err)
	}
	if parsed.AccessToken == "" {
		return codexTokens{}, fmt.Errorf("credentials: codex refresh response missing access_token")
	}
	return codexTokens{
		AccessToken:  parsed.AccessToken,
		RefreshToken: parsed.RefreshToken,
		IDToken:      parsed.IDToken,
		ExpiresIn:    parsed.ExpiresIn,
	}, nil
}

func applyCodexTokens(tokens map[string]any, t codexTokens) {
	tokens["access_token"] = t.AccessToken
	if t.RefreshToken != "" {
		tokens["refresh_token"] = t.RefreshToken
	}
	if t.IDToken != "" {
		tokens["id_token"] = t.IDToken
	}
	if id := extractAccountID(t.IDToken, t.AccessToken); id != "" {
		tokens["account_id"] = id
	}
}

func jwtClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	payload := parts[1]
	if pad := len(payload) % 4; pad != 0 {
		payload += strings.Repeat("=", 4-pad)
	}
	raw, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		if raw, err = base64.RawURLEncoding.DecodeString(parts[1]); err != nil {
			return nil
		}
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil
	}
	return claims
}

func jwtExpiry(token string) time.Time {
	claims := jwtClaims(token)
	if claims == nil {
		return time.Time{}
	}
	switch v := claims["exp"].(type) {
	case float64:
		return time.Unix(int64(v), 0)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return time.Unix(n, 0)
		}
	}
	return time.Time{}
}

func extractAccountID(idToken, accessToken string) string {
	for _, token := range []string{idToken, accessToken} {
		claims := jwtClaims(token)
		if claims == nil {
			continue
		}
		if id, ok := claims["chatgpt_account_id"].(string); ok && id != "" {
			return id
		}
		if auth, ok := claims["https://api.openai.com/auth"].(map[string]any); ok {
			if id, ok := auth["chatgpt_account_id"].(string); ok && id != "" {
				return id
			}
		}
		if orgs, ok := claims["organizations"].([]any); ok && len(orgs) > 0 {
			if first, ok := orgs[0].(map[string]any); ok {
				if id, ok := first["id"].(string); ok && id != "" {
					return id
				}
			}
		}
	}
	return ""
}

func extractEmail(idToken string) string {
	claims := jwtClaims(idToken)
	if claims == nil {
		return ""
	}
	if email, ok := claims["email"].(string); ok {
		return email
	}
	return ""
}
