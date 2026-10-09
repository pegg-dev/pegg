package bootstrap

import (
	"testing"

	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/session"
)

type discardHandler struct{}

func (discardHandler) Write(logger.Record) error { return nil }
func (discardHandler) Close() error              { return nil }

func newSessionManager(t *testing.T) *session.Manager {
	t.Helper()
	store, err := session.NewJSONStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return session.NewManager(store, event.New(), logger.New(logger.LevelDebug, discardHandler{}))
}

func TestSessionModelPrefersProjectDir(t *testing.T) {
	mgr := newSessionManager(t)
	if _, err := mgr.Create(session.CreateOptions{Title: "a", Provider: "pa", Model: "ma", ProjectDir: "/proj/a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Create(session.CreateOptions{Title: "b", Provider: "pb", Model: "mb", ProjectDir: "/proj/b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Create(session.CreateOptions{Title: "c", ProjectDir: "/proj/a"}); err != nil {
		t.Fatal(err)
	}

	prov, model, ok := sessionModel(mgr, "/proj/a")
	if !ok || prov != "pa" || model != "ma" {
		t.Fatalf("sessionModel(/proj/a) = (%q,%q,%v), want (pa,ma,true)", prov, model, ok)
	}
}

func TestSessionModelGlobalFallback(t *testing.T) {
	mgr := newSessionManager(t)
	if _, err := mgr.Create(session.CreateOptions{Title: "a", Provider: "pa", Model: "ma", ProjectDir: "/proj/a"}); err != nil {
		t.Fatal(err)
	}

	if _, _, ok := sessionModel(mgr, "/proj/unknown"); ok {
		t.Fatal("expected no model for an unknown project dir")
	}
	prov, model, ok := sessionModel(mgr, "")
	if !ok || prov != "pa" || model != "ma" {
		t.Fatalf("global sessionModel = (%q,%q,%v), want (pa,ma,true)", prov, model, ok)
	}
}
