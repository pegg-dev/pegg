package middlewares

import (
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/llm"
)

func TestRedact(t *testing.T) {
	r := newRedactor()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"json api_key", `{"api_key":"sk-abc123def456","name":"x"}`, `{"api_key":"[**REDACTED**]","name":"x"}`},
		{"toml quoted", `api_key = "sk-abc123def456ghi789"`, `api_key = "[**REDACTED**]"`},
		{"env unquoted", "API_KEY=sk-abc123def456ghi789jkl012", "API_KEY=[**REDACTED**]"},
		{"yaml", "password: hunter2", "password: [**REDACTED**]"},
		{"single quotes", `secret = 'my secret value'`, `secret = '[**REDACTED**]'`},
		{"openai sk-proj", "use sk-proj-1234567890abcdefghijklmnop for me", "use [**REDACTED**] for me"},
		{"anthropic sk-ant", "key sk-ant-abcdefghijklmnopqrstuvwxyz1234 here", "key [**REDACTED**] here"},
		{"generic sk", "token sk-abcdefghijklmnopqrstuvwxyz", "token [**REDACTED**]"},
		{"aws", "AKIAIOSFODNN7EXAMPLE", "[**REDACTED**]"},
		{"google", "AIzaSyA-0123456789abcdefghijklmnopqr", "[**REDACTED**]"},
		{"github ghp", "ghp_abcdefghijklmnopqrstuvwxyzABCDEFG", "[**REDACTED**]"},
		{"slack xoxb", "xoxb-123456789012-abcdefghijklmnop", "[**REDACTED**]"},
		{"stripe sk_live", "sk_live_abcdefghijklmnopqrstuvwx", "[**REDACTED**]"},
		{"telegram bot", "token 1234567890:AAHabcdefghijklmnopqrstuvwxyzABCDEF", "token [**REDACTED**]"},
		{"rsa pem", "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\n-----END RSA PRIVATE KEY-----", RedactionToken},
		{"openssh pem", "-----BEGIN OPENSSH PRIVATE KEY-----\naaa\n-----END OPENSSH PRIVATE KEY-----", RedactionToken},
		{"ec pem", "-----BEGIN EC PRIVATE KEY-----\nMHcCAQEEIImY\n-----END EC PRIVATE KEY-----", RedactionToken},
		{"pgp private key block", "-----BEGIN PGP PRIVATE KEY BLOCK-----\nVersion: BCPG C#\nlI0E\n-----END PGP PRIVATE KEY BLOCK-----", RedactionToken},
		{"jwt", "header eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U footer", "header [**REDACTED**] footer"},
		{"bearer", "Authorization: Bearer abcdefghijklmnopqrstuvwxyz0123456789", "Authorization: [**REDACTED**]"},
		{"url creds", "postgres://user:s3cr3t@db.example.com:5432/app", "postgres://[**REDACTED**]@db.example.com:5432/app"},
		{"netrc", "machine example.com login alice password hunter2", "machine example.com login alice password [**REDACTED**]"},
		{"high entropy", "dbatman_7585_8898_twitter the_random_128char_x7wQzKcVbNm9pLk2jH4fG8tR6yU3iE5oA1sD7fG9hJ0kL2xZcVbNmQwErTyUiOpAsDfGhJkLzXcVbNm", "dbatman_7585_8898_twitter [**REDACTED**]"},
		{"github pat", "github_pat_11ABCabcDEFghijklmnopqrstuvwx", "[**REDACTED**]"},
		{"gitlab", "glpat-abcdefghijklmnopqrstuv", "[**REDACTED**]"},
		{"sendgrid", "SG.abcdefghijklmnopqrstuvwxyz1234", "[**REDACTED**]"},
		{"pypi", "pypi-AgEIcHlwaS5vcmcABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcdefghij", "[**REDACTED**]"},
		{"webhook secret", "whsec_abcdefghijklmnopqrstuvwxyz", "[**REDACTED**]"},
		{"no false positive short sk-", "sk-abc", "sk-abc"},
		{"no false positive uuid", "id 123e4567-e89b-12d3-a456-426614174000 end", "id 123e4567-e89b-12d3-a456-426614174000 end"},
		{"no false positive prose", "a token of my appreciation", "a token of my appreciation"},
		{"no false pass in prose", "my password is very strong", "my password is very strong"},
		{"data uri preserved", "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==",
			"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.Redact(c.in)
			if got != c.want {
				t.Errorf("Redact(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestRedactCustomKeys(t *testing.T) {
	r := newRedactor(WithSensitiveKeys("my_special_key"))
	in := `{"my_special_key": "secretvalue", "api_key": "x"}`
	got := r.Redact(in)
	if !strings.Contains(got, "[**REDACTED**]") || strings.Contains(got, "secretvalue") {
		t.Errorf("custom key not redacted: %q", got)
	}
}

func TestRedactDisableHighEntropy(t *testing.T) {
	r := newRedactor(WithDisableHighEntropy())
	in := "abc123XYZ789def456GHI012jkl345MNO678pqr"
	if got := r.Redact(in); got != in {
		t.Errorf("expected unchanged with high entropy disabled, got %q", got)
	}
}

func TestRedactDisableKeyValue(t *testing.T) {
	r := newRedactor(WithDisableKeyValue())
	in := `password = "hunter2"`
	got := r.Redact(in)
	if strings.Contains(got, "[**REDACTED**]") {
		t.Errorf("expected no key-value redaction, but got %q", got)
	}
}

func TestRedactDisableKnownTokens(t *testing.T) {
	r := newRedactor(WithDisableKnownTokens())
	in := "AKIAIOSFODNN7EXAMPLE"
	if got := r.Redact(in); got != in {
		t.Errorf("expected no known token redaction, got %q", got)
	}
}

func TestRedactDisablePrivateKeys(t *testing.T) {
	r := newRedactor(WithDisablePrivateKeys())
	in := "-----BEGIN RSA PRIVATE KEY-----\nMIIEow\n-----END RSA PRIVATE KEY-----"
	if got := r.Redact(in); got != in {
		t.Errorf("expected no private key redaction, got %q", got)
	}
}

func TestRedactDisableURLs(t *testing.T) {
	r := newRedactor(WithDisableURLs())
	in := "postgres://user:pass@host:5432/db"
	if got := r.Redact(in); got != in {
		t.Errorf("expected no URL redaction, got %q", got)
	}
}

func TestRedactDisableNetrc(t *testing.T) {
	r := newRedactor(WithDisableNetrc())
	in := "machine example.com password secret"
	if got := r.Redact(in); got != in {
		t.Errorf("expected no netrc redaction, got %q", got)
	}
}

func TestRedactHighEntropyMinLength(t *testing.T) {
	r := newRedactor(WithHighEntropyMinLength(50), WithHighEntropyMinEntropy(1.0))
	short := "abc123XYZ789def456GHI012jkl345MNO678pqr"
	if got := r.Redact(short); got != short {
		t.Errorf("expected no redact for len=40 < minLength=50, got %q", got)
	}
}

func TestShannonEntropy(t *testing.T) {
	tests := []struct {
		s    string
		want float64
	}{
		{"", 0},
		{"aaaa", 0},
		{"abcd", 2.0},
		{"aabb", 1.0},
	}
	for _, tt := range tests {
		got := shannonEntropy(tt.s)
		if got < tt.want-0.001 || got > tt.want+0.001 {
			t.Errorf("shannonEntropy(%q) = %f, want %f", tt.s, got, tt.want)
		}
	}
}

func TestIsDataURIPayload(t *testing.T) {
	tests := []struct {
		s     string
		start int
		want  bool
	}{
		{"data:image/png;base64,iVBORw0KGgo", 22, true},
		{"data:image/png;base64,iVBORw0KGgo", 2, false},
		{"no base64 data here", 10, false},
	}
	for _, tt := range tests {
		got := isDataURIPayload(tt.s, tt.start)
		if got != tt.want {
			t.Errorf("isDataURIPayload(%q, %d) = %v, want %v", tt.s, tt.start, got, tt.want)
		}
	}
}

func TestRedactMessageStringContent(t *testing.T) {
	r := newRedactor()
	m := llm.NewMessage(llm.RoleUser, `{"api_key":"sk-1234567890abcdefghijklmno"}`)
	r.redactMessage(&m, true)
	if strings.Contains(m.Content.(string), "sk-1234567890") {
		t.Errorf("secret not masked: %v", m.Content)
	}
	if !strings.Contains(m.Content.(string), RedactionToken) {
		t.Errorf("expected redaction token in: %v", m.Content)
	}
}

func TestRedactMessageContentPartsSkipsMedia(t *testing.T) {
	r := newRedactor()
	img := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="
	m := llm.Message{Content: []any{
		map[string]any{"type": "text", "text": `api_key="sk-1234567890abcdefghijklmno"`},
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + img}},
	}}
	r.redactMessage(&m, true)
	parts := m.Content.([]any)
	text := parts[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "[**REDACTED**]") {
		t.Errorf("text part not redacted: %q", text)
	}
	url := parts[1].(map[string]any)["image_url"].(map[string]any)["url"].(string)
	if strings.Contains(url, "[**REDACTED**]") {
		t.Errorf("image url payload must not be redacted: %q", url)
	}
}

func TestRedactMessageReasoning(t *testing.T) {
	r := newRedactor()
	m := llm.NewMessage(llm.RoleAssistant, "ok")
	m.Reasoning = "the api key is sk-abcdefghijklmnopqrstuvwxyz1234"
	r.redactMessage(&m, true)
	s, ok := m.Reasoning.(string)
	if !ok {
		t.Fatalf("reasoning not a string: %T", m.Reasoning)
	}
	if strings.Contains(s, "sk-abcdefghijklmnopqrstuvwxyz1234") {
		t.Errorf("reasoning not redacted: %q", s)
	}
}

func TestRedactMessageToolCallsFlag(t *testing.T) {
	r := newRedactor()
	m := llm.Message{Content: `{"x":"sk-1234567890abcdefghijklmno"}`}
	m.ToolCalls = []llm.ToolCall{
		{Function: llm.Function{Arguments: `{"p":"sk-1234567890abcdefghijklmno"}`}},
	}
	r.redactMessage(&m, false)
	if !strings.Contains(m.ToolCalls[0].Function.Arguments, "sk-1234567890") {
		t.Errorf("tool calls must NOT be redacted when toolCalls=false")
	}
	r.redactMessage(&m, true)
	if strings.Contains(m.ToolCalls[0].Function.Arguments, "sk-1234567890") {
		t.Errorf("tool calls must be redacted when toolCalls=true")
	}
}

func TestRedactMessageToolCallsInContentFlag(t *testing.T) {
	r := newRedactor()
	args := `{"url":"postgres://admin:s3cret@db.example.com:5432/myapp"}`
	m := llm.Message{Content: []any{
		map[string]any{"type": "text", "text": "connecting"},
		map[string]any{"type": "tool_calls", "tool_calls": []any{
			map[string]any{
				"type": "function",
				"id":   "call_1",
				"function": map[string]any{
					"name":      "db_connect",
					"arguments": args,
				},
			},
		}},
	}}

	r.redactMessage(&m, false)
	parts := m.Content.([]any)
	tcPart := parts[1].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)
	fnArgs := tcPart["function"].(map[string]any)["arguments"].(string)
	if !strings.Contains(fnArgs, "s3cret") {
		t.Errorf("tool_calls in content must NOT be redacted when toolCalls=false, got: %s", fnArgs)
	}

	r.redactMessage(&m, true)
	parts = m.Content.([]any)
	tcPart = parts[1].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)
	fnArgs = tcPart["function"].(map[string]any)["arguments"].(string)
	if strings.Contains(fnArgs, "s3cret") {
		t.Errorf("tool_calls in content must be redacted when toolCalls=true")
	}
}
