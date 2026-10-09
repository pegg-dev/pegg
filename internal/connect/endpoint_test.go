package connect_test

import (
	"testing"

	"github.com/peggco/pegg/internal/connect"
)

func TestEndpointAppliesToken(t *testing.T) {
	cases := []struct {
		name  string
		relay string
		token string
		want  string
	}{
		{
			name:  "adds the token",
			relay: "wss://connect.example.com/agent",
			token: "tok_123",
			want:  "wss://connect.example.com/agent?token=tok_123",
		},
		{
			name:  "replaces an existing token",
			relay: "wss://connect.example.com/agent?token=old",
			token: "new",
			want:  "wss://connect.example.com/agent?token=new",
		},
		{
			name:  "keeps other query parameters",
			relay: "ws://localhost:3000/api/connect/agent?region=eu",
			token: "tok 1",
			want:  "ws://localhost:3000/api/connect/agent?region=eu&token=tok+1",
		},
		{
			name:  "empty token strips it",
			relay: "wss://connect.example.com/agent?token=old&region=eu",
			token: "",
			want:  "wss://connect.example.com/agent?region=eu",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := connect.Endpoint(tc.relay, tc.token)
			if err != nil {
				t.Fatalf("Endpoint(%q, %q) error: %v", tc.relay, tc.token, err)
			}
			if got != tc.want {
				t.Fatalf("Endpoint = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEndpointRejectsInvalidURLs(t *testing.T) {
	for _, relay := range []string{"", "   ", "https://connect.example.com", "not a url"} {
		if _, err := connect.Endpoint(relay, "tok"); err == nil {
			t.Fatalf("Endpoint(%q) accepted an invalid relay URL", relay)
		}
	}
}

func TestTokenFromURLAndStrip(t *testing.T) {
	relay := "wss://connect.example.com/agent?token=abc&region=eu"
	if got := connect.TokenFromURL(relay); got != "abc" {
		t.Fatalf("TokenFromURL = %q, want abc", got)
	}
	if got := connect.TokenFromURL("wss://connect.example.com/agent"); got != "" {
		t.Fatalf("TokenFromURL without token = %q, want empty", got)
	}
	if got := connect.StripToken(relay); got != "wss://connect.example.com/agent?region=eu" {
		t.Fatalf("StripToken = %q", got)
	}
	if got := connect.StripToken("nonsense"); got != "nonsense" {
		t.Fatalf("StripToken(invalid) = %q, want unchanged", got)
	}
}

func TestMaskToken(t *testing.T) {
	if got := connect.MaskToken(""); got != "(none)" {
		t.Fatalf("MaskToken(\"\") = %q", got)
	}
	if got := connect.MaskToken("short"); got != "•••••" {
		t.Fatalf("MaskToken(short) = %q", got)
	}
	if got := connect.MaskToken("abcdefghijklmnop"); got != "abcd••••••mnop" {
		t.Fatalf("MaskToken(long) = %q", got)
	}
}
