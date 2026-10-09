package cli

import (
	"errors"
	"testing"

	"github.com/peggco/pegg/internal/core/config"
)

func TestResolveConnectTokenPriority(t *testing.T) {
	saved := "tok_saved"
	prompt := func() (string, error) { return "tok_prompted", nil }

	cases := []struct {
		name        string
		arg         string
		flag        string
		saved       string
		interactive bool
		want        string
		wantSource  tokenSource
	}{
		{
			name: "argument wins",
			arg:  "tok_arg", flag: "tok_flag", saved: saved, interactive: false,
			want: "tok_arg", wantSource: tokenFromArgument,
		},
		{
			name: "flag beats saved",
			flag: "tok_flag", saved: saved, interactive: false,
			want: "tok_flag", wantSource: tokenFromFlag,
		},
		{
			name:  "saved token runs without asking",
			saved: saved, interactive: true,
			want: saved, wantSource: tokenFromConfig,
		},
		{
			name:        "prompts when nothing is provided",
			interactive: true,
			want:        "tok_prompted", wantSource: tokenFromPrompt,
		},
		{
			name: "trims whitespace",
			arg:  "  tok_arg  ", interactive: false,
			want: "tok_arg", wantSource: tokenFromArgument,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, source, err := resolveConnectToken(tc.arg, tc.flag, tc.saved, tc.interactive, prompt)
			if err != nil {
				t.Fatalf("resolveConnectToken error: %v", err)
			}
			if got != tc.want || source != tc.wantSource {
				t.Fatalf("got (%q, %v), want (%q, %v)", got, source, tc.want, tc.wantSource)
			}
		})
	}
}

func TestResolveConnectTokenFailsWithoutInput(t *testing.T) {
	prompt := func() (string, error) { return "", errors.New("should not prompt") }

	if _, _, err := resolveConnectToken("", "", "", false, prompt); err == nil {
		t.Fatal("expected an error when no token is available and stdin is not a terminal")
	}

	blank := func() (string, error) { return "   ", nil }
	if _, _, err := resolveConnectToken("", "", "", true, blank); err == nil {
		t.Fatal("expected an error for an empty prompted token")
	}

	failing := func() (string, error) { return "", errors.New("ctrl+c") }
	if _, _, err := resolveConnectToken("", "", "", true, failing); err == nil {
		t.Fatal("expected the prompt error to propagate")
	}
}

func TestConnectConfigRelayAndToken(t *testing.T) {
	c := &CLI{}
	def := c.connectConfig("", false, false, 0, nil, "", 0)
	if def.RelayURL != config.DefaultConnectRelayURL {
		t.Fatalf("RelayURL = %q, want the built-in relay", def.RelayURL)
	}
	if def.Token != "" {
		t.Fatalf("Token = %q, want empty", def.Token)
	}

	if got := c.connectConfig("tok_123", false, false, 0, nil, "", 0).Token; got != "tok_123" {
		t.Fatalf("Token = %q, want tok_123", got)
	}

	saved := &CLI{cfg: &config.Config{Connect: &config.ConnectConfig{
		RelayURL: "wss://relay.example.com/agent?token=embedded",
	}}}
	cc := saved.connectConfig("", false, false, 0, nil, "", 0)
	if cc.RelayURL != "wss://relay.example.com/agent" || cc.Token != "embedded" {
		t.Fatalf("unexpected config: relay=%q token=%q", cc.RelayURL, cc.Token)
	}

	if got := saved.connectConfig("explicit", false, false, 0, nil, "", 0); got.Token != "explicit" {
		t.Fatalf("Token = %q, want explicit", got.Token)
	}
}

func TestConnectRelayEnvOverride(t *testing.T) {
	t.Setenv(envConnectRelay, "ws://127.0.0.1:4000/agent")
	c := &CLI{}

	cc := c.connectConfig("", false, false, 0, nil, "", 0)
	if cc.RelayURL != "ws://127.0.0.1:4000/agent" {
		t.Fatalf("RelayURL = %q, want the env override", cc.RelayURL)
	}

	saved := &CLI{cfg: &config.Config{Connect: &config.ConnectConfig{RelayURL: "wss://relay.example.com/agent"}}}
	if got := saved.connectConfig("", false, false, 0, nil, "", 0).RelayURL; got != "wss://relay.example.com/agent" {
		t.Fatalf("RelayURL = %q, want the configured relay", got)
	}
}
