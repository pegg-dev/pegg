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
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	claudeTokenURL = "https://console.anthropic.com/v1/oauth/token"
	claudeClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"

	refreshSkew = 60 * time.Second
)

type Claude struct {
	path     string
	tokenURL string
	mu       sync.Mutex
}

func NewClaude() *Claude {
	return &Claude{path: claudeCredentialsPath(), tokenURL: claudeTokenURL}
}

func claudeCredentialsPath() string {
	if dir := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); dir != "" {
		return filepath.Join(dir, ".credentials.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".credentials.json"
	}
	return filepath.Join(home, ".claude", ".credentials.json")
}

func (c *Claude) AccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var token string
	err := lockFile(c.path+".lock", func() error {
		root, oauth, err := c.load()
		if err != nil {
			return err
		}
		access := rawString(oauth, "accessToken")
		refresh := rawString(oauth, "refreshToken")
		if access == "" && refresh == "" {
			return ErrNotLoggedIn
		}

		exp, unitMs := parseExpiry(oauth)
		if refresh != "" && (exp.IsZero() || time.Now().Add(refreshSkew).After(exp)) {
			tokens, err := refreshClaude(ctx, c.tokenURL, refresh)
			if err != nil {
				return err
			}
			access = tokens.AccessToken
			oauth["accessToken"] = jsonString(tokens.AccessToken)
			if tokens.RefreshToken != "" {
				oauth["refreshToken"] = jsonString(tokens.RefreshToken)
			}
			oauth["expiresAt"] = jsonInt(expiryValue(tokens.ExpiresIn, unitMs))
			root["claudeAiOauth"] = marshalRaw(oauth)
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

func (c *Claude) Refresh(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return lockFile(c.path+".lock", func() error {
		root, oauth, err := c.load()
		if err != nil {
			return err
		}
		refresh := rawString(oauth, "refreshToken")
		if refresh == "" {
			return ErrNotLoggedIn
		}
		_, unitMs := parseExpiry(oauth)
		tokens, err := refreshClaude(ctx, c.tokenURL, refresh)
		if err != nil {
			return err
		}
		oauth["accessToken"] = jsonString(tokens.AccessToken)
		if tokens.RefreshToken != "" {
			oauth["refreshToken"] = jsonString(tokens.RefreshToken)
		}
		oauth["expiresAt"] = jsonInt(expiryValue(tokens.ExpiresIn, unitMs))
		root["claudeAiOauth"] = marshalRaw(oauth)
		return c.save(root)
	})
}

func (c *Claude) Status() Status {
	_, oauth, err := c.load()
	if err != nil {
		return Status{Provider: "claude"}
	}
	exp, _ := parseExpiry(oauth)
	return Status{
		Provider:  "claude",
		Account:   rawString(oauth, "email"),
		Plan:      rawString(oauth, "subscriptionType"),
		ExpiresAt: exp,
		LoggedIn:  rawString(oauth, "accessToken") != "" || rawString(oauth, "refreshToken") != "",
	}
}

func (c *Claude) load() (map[string]json.RawMessage, map[string]json.RawMessage, error) {
	data, err := os.ReadFile(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, ErrNotLoggedIn
		}
		return nil, nil, fmt.Errorf("credentials: read claude credentials: %w", err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, nil, fmt.Errorf("credentials: parse claude credentials: %w", err)
	}
	oauth := map[string]json.RawMessage{}
	if raw, ok := root["claudeAiOauth"]; ok {
		if err := json.Unmarshal(raw, &oauth); err != nil {
			return nil, nil, fmt.Errorf("credentials: parse claude oauth block: %w", err)
		}
	}
	return root, oauth, nil
}

func (c *Claude) save(root map[string]json.RawMessage) error {
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("credentials: marshal claude credentials: %w", err)
	}
	return writeJSONAtomic(c.path, data)
}

type claudeTokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
}

func refreshClaude(ctx context.Context, tokenURL, refreshToken string) (claudeTokens, error) {
	body, _ := json.Marshal(map[string]any{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     claudeClientID,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewReader(body))
	if err != nil {
		return claudeTokens{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return claudeTokens{}, fmt.Errorf("credentials: refresh claude token: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return claudeTokens{}, fmt.Errorf("credentials: refresh claude token: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var parsed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return claudeTokens{}, fmt.Errorf("credentials: parse refresh response: %w", err)
	}
	if parsed.AccessToken == "" {
		return claudeTokens{}, fmt.Errorf("credentials: refresh response missing access_token")
	}
	if parsed.ExpiresIn <= 0 {
		parsed.ExpiresIn = 3600
	}
	return claudeTokens{
		AccessToken:  parsed.AccessToken,
		RefreshToken: parsed.RefreshToken,
		ExpiresIn:    parsed.ExpiresIn,
	}, nil
}

func rawString(m map[string]json.RawMessage, key string) string {
	raw, ok := m[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

func rawInt(m map[string]json.RawMessage, key string) (int64, bool) {
	raw, ok := m[key]
	if !ok {
		return 0, false
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		if v, err := n.Int64(); err == nil {
			return v, true
		}
		if f, err := n.Float64(); err == nil {
			return int64(f), true
		}
	}
	return 0, false
}

func parseExpiry(oauth map[string]json.RawMessage) (time.Time, bool) {
	n, ok := rawInt(oauth, "expiresAt")
	if !ok || n == 0 {
		return time.Time{}, true
	}
	if n >= 1e11 {
		return time.UnixMilli(n), true
	}
	return time.Unix(n, 0), false
}

func expiryValue(expiresInSec int64, unitMs bool) int64 {
	t := time.Now().Add(time.Duration(expiresInSec) * time.Second)
	if unitMs {
		return t.UnixMilli()
	}
	return t.Unix()
}

func jsonString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

func jsonInt(n int64) json.RawMessage {
	return json.RawMessage(strconv.FormatInt(n, 10))
}

func marshalRaw(m map[string]json.RawMessage) json.RawMessage {
	b, _ := json.Marshal(m)
	return b
}
