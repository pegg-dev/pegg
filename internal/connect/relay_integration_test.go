package connect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
)

type testDiscardHandler struct{}

func (testDiscardHandler) Write(logger.Record) error { return nil }
func (testDiscardHandler) Close() error              { return nil }

func testLogger() *logger.Logger { return logger.New(logger.LevelDebug, testDiscardHandler{}) }

func TestConnectorHandshakeAndStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	frames := make(chan *Frame, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer c.CloseNow()
		ctx := context.Background()

		hello, err := testReadFrame(ctx, c)
		if err != nil {
			t.Errorf("read hello: %v", err)
			return
		}
		var hp HelloPayload
		if err := jsonUnmarshal(hello.Params, &hp); err != nil {
			t.Errorf("decode hello: %v", err)
			return
		}
		if err := VerifyHello(hp.PublicKey, hp.DeviceID, hp.Nonce, hp.TS, hp.Signature); err != nil {
			t.Errorf("verify hello: %v", err)
			return
		}

		wb, _ := marshal(WelcomePayload{
			ProtocolVersion: ProtocolVersion,
			GrantedScopes:   []string{ScopeRead, ScopeAdmin},
			HeartbeatSecs:   20,
		})
		if err := testWriteFrame(ctx, c, &Frame{Type: "welcome", Result: wb}); err != nil {
			return
		}
		_ = testWriteFrame(ctx, c, &Frame{Type: "request", ID: "1", Method: "status.get"})

		for {
			f, err := testReadFrame(ctx, c)
			if err != nil {
				return
			}
			frames <- f
			if f.Type == "response" && f.ID == "1" {
				return
			}
		}
	}))
	defer srv.Close()

	cfg := config.DefaultConfig().Connect
	cfg.RelayURL = "ws" + strings.TrimPrefix(srv.URL, "http")
	cfg.MaxConcurrency = 2

	conn, err := New(Deps{Config: config.DefaultConfig(), Bus: event.New(), Log: testLogger(), Version: "test"}, cfg)
	if err != nil {
		t.Fatalf("new connector: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = conn.Run(ctx) }()

	deadline := time.After(10 * time.Second)
	for {
		select {
		case f := <-frames:
			if f.Type != "response" || f.ID != "1" {
				continue
			}
			if f.Error != nil {
				t.Fatalf("status.get error: %v", f.Error)
			}
			var res map[string]any
			if err := jsonUnmarshal(f.Result, &res); err != nil {
				t.Fatalf("decode result: %v", err)
			}
			if res["device_id"] == "" {
				t.Fatalf("missing device_id: %v", res)
			}
			if res["protocol"] == nil {
				t.Fatalf("missing protocol: %v", res)
			}
			conn.Stop()
			return
		case <-deadline:
			t.Fatal("timed out waiting for status.get response")
		}
	}
}

func TestConnectorRejectsUnknownMethod(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	resp := make(chan *Frame, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer c.CloseNow()
		ctx := context.Background()
		hello, err := testReadFrame(ctx, c)
		if err != nil {
			return
		}
		var hp HelloPayload
		_ = jsonUnmarshal(hello.Params, &hp)
		wb, _ := marshal(WelcomePayload{ProtocolVersion: ProtocolVersion, GrantedScopes: []string{ScopeAdmin}})
		_ = testWriteFrame(ctx, c, &Frame{Type: "welcome", Result: wb})
		_ = testWriteFrame(ctx, c, &Frame{Type: "request", ID: "9", Method: "does.not.exist"})
		for {
			f, err := testReadFrame(ctx, c)
			if err != nil {
				return
			}
			if f.Type == "response" && f.ID == "9" {
				resp <- f
				return
			}
		}
	}))
	defer srv.Close()

	cfg := config.DefaultConfig().Connect
	cfg.RelayURL = "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, err := New(Deps{Config: config.DefaultConfig(), Bus: event.New(), Log: testLogger(), Version: "test"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = conn.Run(ctx) }()

	select {
	case f := <-resp:
		if f.Error == nil || f.Error.Code != ErrCodeNotFound {
			t.Fatalf("want not_found error, got %+v", f)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}
	conn.Stop()
}

func testReadFrame(ctx context.Context, c *websocket.Conn) (*Frame, error) {
	_, data, err := c.Read(ctx)
	if err != nil {
		return nil, err
	}
	var f Frame
	if err := jsonUnmarshal(data, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

func testWriteFrame(ctx context.Context, c *websocket.Conn, f *Frame) error {
	return writeFrame(ctx, c, f)
}
