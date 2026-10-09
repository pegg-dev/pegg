package connect

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const TokenQueryParam = "token"

func Endpoint(relayURL, token string) (string, error) {
	trimmed := strings.TrimSpace(relayURL)
	if trimmed == "" {
		return "", errors.New("connect: relay URL is empty")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("connect: invalid relay URL %q: %w", relayURL, err)
	}
	if parsed.Scheme != "ws" && parsed.Scheme != "wss" {
		return "", fmt.Errorf("connect: unsupported relay URL scheme in %q (want ws:// or wss://)", relayURL)
	}
	query := parsed.Query()
	if token == "" {
		query.Del(TokenQueryParam)
	} else {
		query.Set(TokenQueryParam, token)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func TokenFromURL(relayURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(relayURL))
	if err != nil {
		return ""
	}
	return parsed.Query().Get(TokenQueryParam)
}

func StripToken(relayURL string) string {
	stripped, err := Endpoint(relayURL, "")
	if err != nil {
		return relayURL
	}
	return stripped
}

func MaskToken(token string) string {
	switch {
	case token == "":
		return "(none)"
	case len(token) <= 8:
		return strings.Repeat("•", len(token))
	default:
		return token[:4] + strings.Repeat("•", 6) + token[len(token)-4:]
	}
}
