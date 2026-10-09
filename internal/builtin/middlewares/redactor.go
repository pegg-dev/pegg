package middlewares

import (
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/peggco/pegg/internal/llm"
)

const RedactionToken = "[**REDACTED**]"

var defaultSensitiveKeys = []string{
	"api_key", "apikey", "api-key", "x_api_key", "x-api-key",
	"secret", "secret_key", "secret-key", "secrets",
	"access_key", "access-key", "accesskey", "access_token", "access-token",
	"auth_token", "auth-token", "authorization",
	"authorization_header", "auth_header",
	"bearer", "auth", "credentials", "credential",
	"password", "passwd", "pwd", "token",
	"private_key", "private-key", "privatekey",
	"client_secret", "client-secret", "consumer_secret", "consumer-secret",
	"session_key", "session-key", "session_secret", "session-secret",
	"session_id", "session-id",
	"encryption_key", "encryption-key", "encrypt_key",
	"master_key", "master-key",
	"refresh_token", "refresh-token", "id_token", "id-token",
	"access_key_id", "access-key-id",
	"secret_access_key", "secret-access-key",
	"secret_key_id", "secret-key-id",
	"login_password", "login-password", "db_password", "db-password",
	"database_password", "database-password",
	"root_password", "root-password", "admin_password", "admin-password",
	"user_password", "user-password", "proxy_password", "proxy-password",
	"redis_password", "redis-password",
	"postgres_password", "postgres-password",
	"mysql_password", "mysql-password",
	"mongo_password", "mongo-password",
	"connection_string", "connection-string", "dsn",
	"database_url", "database-url", "db_url", "db-url",
	"mongodb_uri", "postgres_uri", "mysql_uri", "redis_uri",
	"jwt_secret", "jwt-secret", "hmac_secret", "hmac-secret",
	"signing_secret", "signing-secret", "webhook_secret", "webhook-secret",
	"cookie", "set_cookie", "csrf_token", "csrf-token",
	"xsrf_token", "xsrf-token",
	"slack_token", "slack-token", "bot_token", "bot-token",
	"github_token", "github-token", "gitlab_token", "gitlab-token",
	"npm_token", "npm-token", "pypi_token", "pypi-token",
	"sendgrid_api_key", "sendgrid-api-key",
	"twilio_auth_token", "twilio-auth-token",
	"stripe_secret_key", "stripe-secret-key",
	"s3_secret", "s3-secret", "s3_secret_key", "s3-secret-key",
	"aws_secret_access_key", "aws-secret-access-key",
	"aws_access_key_id", "aws-access-key-id",
	"firebase_api_key",
	"google_api_key", "google_application_credentials",
	"azure_storage_connection_string", "azure_connection_string",
	"openai_api_key", "anthropic_api_key", "gemini_api_key",
	"_auth_token", "_auth", "_password",
}

var (
	privateKeyRe  = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY(?: BLOCK)?-----.*?-----END [A-Z0-9 ]*PRIVATE KEY(?: BLOCK)?-----`)
	jwtRe         = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`)
	bearerRe      = regexp.MustCompile(`(?i)(\bbearer\s+)([A-Za-z0-9._~+/=-]{10,})`)
	urlCredsRe    = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/\s:@]+:[^/\s:@]+@`)
	netrcGateRe   = regexp.MustCompile(`(?im)^\s*(machine\s+\S+|default)\b`)
	netrcPassRe   = regexp.MustCompile(`(?i)(\bpassword\s+)(\S+)`)
	highEntropyRe = regexp.MustCompile(`[A-Za-z0-9+/=_-]{8,}`)

	knownTokenRes = []*regexp.Regexp{
		regexp.MustCompile(`\bsk-proj-[A-Za-z0-9_-]{20,}`),
		regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}`),
		regexp.MustCompile(`\bsk-or-[A-Za-z0-9_-]{20,}`),
		regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}`),
		regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		regexp.MustCompile(`\bASIA[0-9A-Z]{16}\b`),
		regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`),
		regexp.MustCompile(`\bgh[pous]_[A-Za-z0-9]{36,}\b`),
		regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}\b`),
		regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}\b`),
		regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`),
		regexp.MustCompile(`\bsk_live_[0-9a-zA-Z]{24,}\b`),
		regexp.MustCompile(`\bsk_test_[0-9a-zA-Z]{24,}\b`),
		regexp.MustCompile(`\brk_live_[0-9a-zA-Z]{24,}\b`),
		regexp.MustCompile(`\brk_test_[0-9a-zA-Z]{24,}\b`),
		regexp.MustCompile(`\b\d{8,10}:[A-Za-z0-9_-]{35}\b`),
		regexp.MustCompile(`\bSG\.[A-Za-z0-9_-]{20,}\b`),
		regexp.MustCompile(`\bkey-[A-Za-z0-9_-]{20,}`),
		regexp.MustCompile(`\bpypi-AgEIcHlwaS5vcmc[0-9A-Za-z_-]{40,}`),
		regexp.MustCompile(`\bwhsec_[A-Za-z0-9]{20,}\b`),
	}
)

type redactor struct {
	sensitiveKeys         map[string]struct{}
	keyValueRe            *regexp.Regexp
	keyValueEnabled       bool
	knownTokensEnabled    bool
	privateKeysEnabled    bool
	urlsEnabled           bool
	netrcEnabled          bool
	highEntropyEnabled    bool
	highEntropyMinLength  int
	highEntropyMinEntropy float64
}

type RedactionOption func(*redactor)

func WithSensitiveKeys(keys ...string) RedactionOption {
	return func(r *redactor) {
		for _, k := range keys {
			if k != "" {
				r.sensitiveKeys[k] = struct{}{}
			}
		}
	}
}

func WithDisableKeyValue() RedactionOption { return func(r *redactor) { r.keyValueEnabled = false } }
func WithDisableKnownTokens() RedactionOption {
	return func(r *redactor) { r.knownTokensEnabled = false }
}
func WithDisablePrivateKeys() RedactionOption {
	return func(r *redactor) { r.privateKeysEnabled = false }
}
func WithDisableURLs() RedactionOption  { return func(r *redactor) { r.urlsEnabled = false } }
func WithDisableNetrc() RedactionOption { return func(r *redactor) { r.netrcEnabled = false } }
func WithDisableHighEntropy() RedactionOption {
	return func(r *redactor) { r.highEntropyEnabled = false }
}

func WithHighEntropyMinLength(n int) RedactionOption {
	return func(r *redactor) {
		if n > 0 {
			r.highEntropyMinLength = n
		}
	}
}

func WithHighEntropyMinEntropy(f float64) RedactionOption {
	return func(r *redactor) {
		if f >= 0 {
			r.highEntropyMinEntropy = f
		}
	}
}

func newRedactor(opts ...RedactionOption) *redactor {
	r := &redactor{
		sensitiveKeys:         make(map[string]struct{}),
		keyValueEnabled:       true,
		knownTokensEnabled:    true,
		privateKeysEnabled:    true,
		urlsEnabled:           true,
		netrcEnabled:          true,
		highEntropyEnabled:    true,
		highEntropyMinLength:  24,
		highEntropyMinEntropy: 4.2,
	}
	for _, k := range defaultSensitiveKeys {
		r.sensitiveKeys[k] = struct{}{}
	}
	for _, opt := range opts {
		opt(r)
	}
	r.keyValueRe = r.buildKeyValueRe()
	return r
}

func (r *redactor) buildKeyValueRe() *regexp.Regexp {
	keys := make([]string, 0, len(r.sensitiveKeys))
	for k := range r.sensitiveKeys {
		keys = append(keys, regexp.QuoteMeta(k))
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	return regexp.MustCompile(`(?i)(\b(?:` + strings.Join(keys, "|") + `)\b["']?\s*[:=]\s*)(?:"((?:[^"\\]|\\.)*)"|'([^']*)'|([^\n,;}\]]+))`)
}

func (r *redactor) redactKeyValues(s string) string {
	idxs := r.keyValueRe.FindAllStringSubmatchIndex(s, -1)
	if len(idxs) == 0 {
		return s
	}
	var b strings.Builder
	last := 0
	for _, m := range idxs {
		b.WriteString(s[last:m[0]])
		b.WriteString(s[m[2]:m[3]])
		switch {
		case m[4] >= 0:
			b.WriteString(`"[**REDACTED**]"`)
		case m[6] >= 0:
			b.WriteString(`'[**REDACTED**]'`)
		default:
			b.WriteString(RedactionToken)
		}
		last = m[1]
	}
	b.WriteString(s[last:])
	return b.String()
}

func (r *redactor) Redact(s string) string {
	if s == "" {
		return s
	}
	if r.privateKeysEnabled {
		s = privateKeyRe.ReplaceAllString(s, RedactionToken)
	}
	if r.keyValueEnabled {
		s = r.redactKeyValues(s)
	}
	if r.urlsEnabled {
		s = urlCredsRe.ReplaceAllString(s, `${1}[**REDACTED**]@`)
	}
	if r.netrcEnabled && netrcGateRe.MatchString(s) {
		s = netrcPassRe.ReplaceAllString(s, `${1}[**REDACTED**]`)
	}
	if r.knownTokensEnabled {
		for _, re := range knownTokenRes {
			s = re.ReplaceAllString(s, RedactionToken)
		}
		s = jwtRe.ReplaceAllString(s, RedactionToken)
		s = bearerRe.ReplaceAllString(s, `${1}[**REDACTED**]`)
	}
	if r.highEntropyEnabled {
		s = r.redactHighEntropy(s)
	}
	return s
}

func (r *redactor) redactHighEntropy(s string) string {
	var b strings.Builder
	last := 0
	for _, m := range highEntropyRe.FindAllStringIndex(s, -1) {
		start, end := m[0], m[1]
		tok := s[start:end]
		if len(tok) < r.highEntropyMinLength || shannonEntropy(tok) < r.highEntropyMinEntropy {
			continue
		}
		if isDataURIPayload(s, start) {
			continue
		}
		b.WriteString(s[last:start])
		b.WriteString(RedactionToken)
		last = end
	}
	b.WriteString(s[last:])
	return b.String()
}

func isDataURIPayload(s string, start int) bool {
	lo := start - 64
	if lo < 0 {
		lo = 0
	}
	return strings.HasSuffix(s[lo:start], "base64,")
}

func shannonEntropy(s string) float64 {
	if s == "" {
		return 0
	}
	counts := make(map[rune]int)
	for _, r := range s {
		counts[r]++
	}
	n := float64(len(s))
	var h float64
	for _, c := range counts {
		p := float64(c) / n
		h -= p * math.Log2(p)
	}
	return h
}

func (r *redactor) redactMessage(m *llm.Message, toolCalls bool) {
	switch c := m.Content.(type) {
	case string:
		m.Content = r.Redact(c)
	case []any:
		for _, item := range c {
			part, ok := item.(map[string]any)
			if !ok {
				continue
			}
			switch part["type"] {
			case "text":
				if t, ok := part["text"].(string); ok {
					part["text"] = r.Redact(t)
				}
			case "tool_calls":
				if toolCalls {
					if calls, ok := part["tool_calls"].([]any); ok {
						for _, tc := range calls {
							if cm, ok := tc.(map[string]any); ok {
								if fn, ok := cm["function"].(map[string]any); ok {
									if args, ok := fn["arguments"].(string); ok {
										fn["arguments"] = r.Redact(args)
									}
								}
							}
						}
					}
				}
			}
		}
	}
	if reas, ok := m.Reasoning.(string); ok {
		m.Reasoning = r.Redact(reas)
	}
	if toolCalls {
		for i := range m.ToolCalls {
			m.ToolCalls[i].Function.Arguments = r.Redact(m.ToolCalls[i].Function.Arguments)
		}
	}
}
