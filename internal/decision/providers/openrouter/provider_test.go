package openrouter

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/decision"
)

func TestDecideRequestShape(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != DecisionsPath {
			t.Errorf("path = %q", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer sk-test" {
			t.Errorf("auth = %q", auth)
		}
		if r.Header.Get("HTTP-Referer") == "" || r.Header.Get("X-Title") == "" {
			t.Error("expected referer/title headers")
		}
		gotBody = decodeBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"gen-dec-1","model":"typesafe/jev-1.13-20260917","provider":"TypeSafe","answers":{},"usage":{"input_tokens":10,"output_tokens":5}}`))
	}))
	defer srv.Close()

	prov, err := NewFromConfig(config.LLMConfig{
		Provider: ProviderName,
		APIKey:   "sk-test",
		BaseURL:  srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = prov.Decide(context.Background(), &decision.Request{
		State: "state text",
		Questions: map[string]decision.Question{
			"safe_to_run": decision.BoolQuestion("Is it safe?", map[string]string{
				"true": "yes", "false": "no",
			}),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if gotBody["model"] != DefaultModel {
		t.Errorf("model = %v, want default %q", gotBody["model"], DefaultModel)
	}
	if gotBody["state"] != "state text" {
		t.Errorf("state = %v", gotBody["state"])
	}
	qs, ok := gotBody["questions"].(map[string]any)
	if !ok {
		t.Fatalf("questions missing: %v", gotBody)
	}
	q, ok := qs["safe_to_run"].(map[string]any)
	if !ok {
		t.Fatalf("safe_to_run missing: %v", qs)
	}
	if q["type"] != "noul" {
		t.Errorf("question type = %v", q["type"])
	}
}

func TestDecideParsesAnswers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "gen-dec-2",
			"model": "typesafe/jev-1.13-20260917",
			"provider": "TypeSafe",
			"answers": {
				"safe_to_run": {"type": "noul", "noul": 0.98, "confidence": 0.97},
				"team": {"type": "choice", "choice": "billing", "probabilities": {"billing": 1.0}, "confidence": 1.0},
				"urgency": {"type": "score", "score": 1.99, "legend": {"0": "Calm", "1": "Urgent"}, "probabilities": {"0": 0.01, "1": 0.99}, "confidence": 0.99}
			},
			"usage": {"input_tokens": 384, "output_tokens": 22, "cost": 0.000016128}
		}`))
	}))
	defer srv.Close()

	prov, err := NewFromConfig(config.LLMConfig{Provider: ProviderName, APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := prov.Decide(context.Background(), &decision.Request{State: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Model != "typesafe/jev-1.13-20260917" {
		t.Errorf("model = %q", resp.Model)
	}
	if v, ok := resp.Answers["safe_to_run"].NoulValue(); !ok || v != 0.98 {
		t.Errorf("noul = %v", v)
	}
	if resp.Answers["team"].Choice != "billing" {
		t.Errorf("choice = %q", resp.Answers["team"].Choice)
	}
	if v, ok := resp.Answers["urgency"].ScoreValue(); !ok || v != 1.99 {
		t.Errorf("score = %v", v)
	}
	if resp.Usage.InputTokens != 384 || resp.Usage.Cost == 0 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

func TestDecideMapsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer srv.Close()

	prov, err := NewFromConfig(config.LLMConfig{Provider: ProviderName, APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = prov.Decide(context.Background(), &decision.Request{State: "s"})
	var perr *decision.ProviderError
	if !errors.As(err, &perr) {
		t.Fatalf("expected ProviderError, got %v", err)
	}
	if perr.StatusCode != 429 || !strings.Contains(perr.Message, "rate limited") {
		t.Errorf("perr = %+v", perr)
	}
	if !perr.Temporary() {
		t.Error("expected temporary error")
	}
}

func TestDecideExplicitModel(t *testing.T) {
	var model string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := decodeBody(t, r)
		model, _ = body["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"1","answers":{}}`))
	}))
	defer srv.Close()

	prov, err := NewFromConfig(config.LLMConfig{Provider: ProviderName, APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prov.Decide(context.Background(), &decision.Request{Model: "~typesafe/jev-latest", State: "s"}); err != nil {
		t.Fatal(err)
	}
	if model != "~typesafe/jev-latest" {
		t.Errorf("model = %q", model)
	}
}

func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	return body
}
