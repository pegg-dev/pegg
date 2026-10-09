package claude

import (
	"context"
	"testing"

	"github.com/peggco/pegg/internal/llm"
)

func TestBuildBodyPrependsSystemPrefix(t *testing.T) {
	s := NewService("claude", ServiceConfig{BaseURL: "http://example.com", SystemPrefix: "IDENTITY"})
	body := s.buildBody(&llm.Request{
		Messages: []llm.Message{
			llm.SystemMessage("agent prompt"),
			llm.UserMessage("hi"),
		},
	}, false)

	cr, ok := body.(*claudeRequest)
	if !ok {
		t.Fatalf("body type = %T", body)
	}
	if cr.System != "IDENTITY\nagent prompt" {
		t.Fatalf("system = %q, want %q", cr.System, "IDENTITY\nagent prompt")
	}
}

func TestListModelsStatic(t *testing.T) {
	s := NewService("claude", ServiceConfig{
		BaseURL: "http://example.com",
		Models:  []llm.Model{{ID: "m1", Name: "M1"}},
	})
	models, err := s.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "m1" {
		t.Fatalf("models = %+v", models)
	}
}
