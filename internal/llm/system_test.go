package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/peggco/pegg/internal/core/config"
)

type systemProvider struct {
	name string
}

func (p *systemProvider) Name() string { return p.name }
func (p *systemProvider) Chat(context.Context, *Request) (*Response, error) {
	return nil, errors.New("system provider must be resolved by a hook")
}
func (p *systemProvider) ChatStream(context.Context, *Request, StreamHandler) error {
	return errors.New("system provider must be resolved by a hook")
}
func (p *systemProvider) ListModels(context.Context) ([]Model, error) {
	return nil, nil
}

func TestManagerSystemProviderExactSelect(t *testing.T) {
	mgr, _ := newTestManager(t)
	registerMockProvider(t, "sys-real", []Model{{ID: "real-1"}})
	mgr.Sync(context.Background(), []config.LLMConfig{{Provider: "sys-real"}})

	mgr.RegisterSystemProvider("smart-router", &systemProvider{name: "smart-router"}, []Model{{
		ID: "smart-router", Name: "Smart Router",
	}})

	res := mgr.Select(SelectRequest{Mode: SelectModeExact, Model: "smart-router"})
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if res.Provider != "smart-router" || res.Model.ID != "smart-router" {
		t.Fatalf("res = %+v", res)
	}

	prov, err := mgr.Provider("smart-router")
	if err != nil {
		t.Fatal(err)
	}
	if prov.Name() != "smart-router" {
		t.Fatalf("provider = %q", prov.Name())
	}

	models, err := mgr.Models("smart-router")
	if err != nil || len(models) != 1 || models[0].ID != "smart-router" {
		t.Fatalf("models = %v, err = %v", models, err)
	}
}

func TestManagerPreferredSkipsSystemProvider(t *testing.T) {
	mgr, _ := newTestManager(t)
	registerMockProvider(t, "sys-real", []Model{{ID: "real-1"}})
	mgr.Sync(context.Background(), []config.LLMConfig{{Provider: "sys-real"}})
	mgr.RegisterSystemProvider("smart-router", &systemProvider{name: "smart-router"}, []Model{{
		ID: "smart-router",
	}})

	res := mgr.Select(SelectRequest{Mode: SelectModePreferred})
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if res.Provider == "smart-router" {
		t.Fatalf("preferred must never pick a system provider, got %+v", res)
	}
	if res.Provider != "sys-real" || res.Model.ID != "real-1" {
		t.Fatalf("res = %+v, want sys-real/real-1", res)
	}
}

func TestManagerSystemProviderExactDoesNotPolluteRealModels(t *testing.T) {
	mgr, _ := newTestManager(t)
	registerMockProvider(t, "sys-real", []Model{{ID: "real-1"}})
	mgr.Sync(context.Background(), []config.LLMConfig{{Provider: "sys-real"}})
	mgr.RegisterSystemProvider("smart-router", &systemProvider{name: "smart-router"}, []Model{{
		ID: "smart-router",
	}})

	if res := mgr.Select(SelectRequest{Mode: SelectModeExact, Model: "real-1"}); res.Err != nil || res.Provider != "sys-real" {
		t.Fatalf("real model lookup broken: %+v", res)
	}
	if res := mgr.Select(SelectRequest{Mode: SelectModeExact, Model: "nope"}); res.Err == nil {
		t.Fatalf("expected error for unknown model, got %+v", res)
	}
}
