package acp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/peggco/pegg/internal/session"
)

func TestInitialize(t *testing.T) {
	srv := newTestServer(t)
	srv.transport = newBufferTransport()
	ctx := context.Background()

	params := InitializeParams{
		ProtocolVersion: 1,
		ClientInfo: &ClientInfo{
			Name:    "test-client",
			Version: "1.0.0",
		},
	}
	resp := sendRequest(t, srv, ctx, "initialize", params)
	if !strings.Contains(resp, `"protocolVersion":1`) {
		t.Errorf("expected protocolVersion 1 in response, got %s", resp)
	}
	if !strings.Contains(resp, `"loadSession":true`) {
		t.Errorf("expected loadSession capability, got %s", resp)
	}
	if !strings.Contains(resp, "pegg") {
		t.Errorf("expected pegg in agentInfo, got %s", resp)
	}
}

func TestInitializeInvalidVersion(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()
	params := InitializeParams{ProtocolVersion: 0}
	resp, err := srv.handleInitialize(ctx, toJSON(t, params))
	if err == nil {
		t.Fatal("expected error for invalid version")
	}
	if resp != nil {
		t.Errorf("expected nil result, got %v", resp)
	}
	rpcErr, ok := err.(*RPCError)
	if !ok {
		t.Fatalf("expected *RPCError, got %T", err)
	}
	if rpcErr.Code != ErrCodeInvalidParams {
		t.Errorf("expected invalid params code, got %d", rpcErr.Code)
	}
}

func TestInitializeMalformedParams(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()
	_, err := srv.handleInitialize(ctx, json.RawMessage(`{"protocolVersion":`))
	if err == nil {
		t.Fatal("expected error for malformed params")
	}
}

func TestSessionNew(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	result, err := srv.handleSessionNew(ctx, toJSON(t, SessionNewParams{Cwd: "/tmp"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sr := result.(SessionNewResult)
	if sr.SessionId == "" {
		t.Fatal("expected non-empty session id")
	}
	if _, ok := srv.getSession(string(sr.SessionId)); !ok {
		t.Error("expected session to be active after session/new")
	}
}

func TestSessionNewEmptyCwd(t *testing.T) {
	srv := newTestServer(t)
	srv.transport = newBufferTransport()
	ctx := context.Background()

	params := SessionNewParams{Cwd: ""}
	resp := sendRequest(t, srv, ctx, "session/new", params)
	if !strings.Contains(resp, `"code":-32602`) {
		t.Errorf("expected invalid params error, got %s", resp)
	}
}

func TestSessionNewCreatesPeggSession(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	result, err := srv.handleSessionNew(ctx, toJSON(t, SessionNewParams{Cwd: "/tmp"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sr := result.(SessionNewResult)
	sessID := string(sr.SessionId)

	acpSess, ok := srv.getSession(sessID)
	if !ok {
		t.Fatal("expected active session")
	}
	if acpSess.PeggSessionID == "" {
		t.Error("expected a Pegg session to be persisted")
	}
	if _, err := srv.sessions.Get(acpSess.PeggSessionID); err != nil {
		t.Errorf("expected pegg session to exist, got %v", err)
	}
}

func TestSessionCloseActive(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	result, err := srv.handleSessionNew(ctx, toJSON(t, SessionNewParams{Cwd: "/tmp"}))
	if err != nil {
		t.Fatalf("session/new failed: %v", err)
	}
	sessID := string(result.(SessionNewResult).SessionId)

	if _, err := srv.handleSessionClose(ctx, toJSON(t, SessionCloseParams{SessionId: SessionId(sessID)})); err != nil {
		t.Fatalf("session/close failed: %v", err)
	}
	if _, ok := srv.getSession(sessID); ok {
		t.Error("expected session removed after close")
	}
}

func TestSessionCloseMissing(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	if _, err := srv.handleSessionClose(ctx, toJSON(t, SessionCloseParams{SessionId: "does-not-exist"})); err != nil {
		t.Fatalf("expected no error for missing session close, got %v", err)
	}
}

func TestSessionList(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	result, err := srv.handleSessionList(ctx, toJSON(t, SessionListParams{}))
	if err != nil {
		t.Fatalf("session/list failed: %v", err)
	}
	lr := result.(SessionListResult)
	if lr.Sessions == nil {
		t.Fatal("expected non-nil sessions")
	}
}

func TestSessionDeleteMissing(t *testing.T) {
	srv := newTestServer(t)
	srv.transport = newBufferTransport()
	ctx := context.Background()

	resp := sendRequest(t, srv, ctx, "session/delete", SessionDeleteParams{SessionId: "does-not-exist"})
	if !strings.Contains(resp, `"code"`) {
		t.Errorf("expected error for missing session, got %s", resp)
	}
}

func TestMethodNotFound(t *testing.T) {
	srv := newTestServer(t)
	srv.transport = newBufferTransport()
	ctx := context.Background()

	raw, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "does/not/exist",
	})
	srv.handleFrame(ctx, Message{Body: raw})
	bt := srv.transport.(*bufferTransport)
	resp := bt.sends[len(bt.sends)-1]
	if !strings.Contains(resp, `"code":-32601`) {
		t.Errorf("expected method not found error, got %s", resp)
	}
}

func TestParseError(t *testing.T) {
	srv := newTestServer(t)
	srv.transport = newBufferTransport()
	ctx := context.Background()

	srv.handleFrame(ctx, Message{Body: json.RawMessage(`{"jsonrpc": "2.0", "id": 1, "method"`)})
	bt := srv.transport.(*bufferTransport)
	resp := bt.sends[len(bt.sends)-1]
	if !strings.Contains(resp, `"code":-32700`) {
		t.Errorf("expected parse error code, got %s", resp)
	}
}

func TestInvalidJSONRPCVersion(t *testing.T) {
	srv := newTestServer(t)
	srv.transport = newBufferTransport()
	ctx := context.Background()

	raw, _ := json.Marshal(map[string]any{
		"jsonrpc": "1.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	})
	srv.handleFrame(ctx, Message{Body: raw})
	bt := srv.transport.(*bufferTransport)
	resp := bt.sends[len(bt.sends)-1]
	if !strings.Contains(resp, `"code":-32600`) {
		t.Errorf("expected invalid request error, got %s", resp)
	}
}

func TestNotificationNoResponse(t *testing.T) {
	srv := newTestServer(t)
	srv.transport = newBufferTransport()
	ctx := context.Background()

	sendNotification(t, srv, ctx, "session/cancel", SessionCancelParams{SessionId: "any"})
	bt := srv.transport.(*bufferTransport)
	if len(bt.sends) != 0 {
		t.Errorf("expected no response for notification, got %d sends: %v", len(bt.sends), bt.sends)
	}
}

func TestClientResponseDelivery(t *testing.T) {
	srv := newTestServer(t)
	srv.transport = newBufferTransport()
	ctx := context.Background()

	raw, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      "some-request-id",
		"result":  map[string]any{"content": "hello"},
	})
	srv.handleFrame(ctx, Message{Body: raw})
	bt := srv.transport.(*bufferTransport)
	if len(bt.sends) != 0 {
		t.Errorf("expected no sends for client response, got %v", bt.sends)
	}
}

func TestExtractPromptText(t *testing.T) {
	blocks := []ContentBlock{
		{Type: "text", Text: "hello world"},
	}
	if got := extractPromptText(blocks); got != "hello world" {
		t.Errorf("expected 'hello world', got %q", got)
	}
}

func TestExtractPromptTextEmpty(t *testing.T) {
	if got := extractPromptText(nil); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestExtractPromptTextNoText(t *testing.T) {
	blocks := []ContentBlock{
		{Type: "image", Data: "base64", MimeType: "image/png"},
	}
	if got := extractPromptText(blocks); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestExtractContent(t *testing.T) {
	msg := session.Message{Content: "hello", Role: "user"}
	block := extractContent(msg)
	if block == nil {
		t.Fatal("expected content block")
	}
	if block.Text != "hello" {
		t.Errorf("expected 'hello', got %q", block.Text)
	}
	if block.Type != "text" {
		t.Errorf("expected type text, got %q", block.Type)
	}
}

func TestExtractContentNonString(t *testing.T) {
	msg := session.Message{Content: 12345}
	if block := extractContent(msg); block != nil {
		t.Errorf("expected nil block, got %v", block)
	}
}

func TestSessionPromptMissingSession(t *testing.T) {
	srv := newTestServer(t)
	srv.transport = newBufferTransport()
	ctx := context.Background()

	params := SessionPromptParams{
		SessionId: "missing",
		Prompt:    []ContentBlock{{Type: "text", Text: "hi"}},
	}
	resp := sendRequest(t, srv, ctx, "session/prompt", params)
	if !strings.Contains(resp, `"code"`) {
		t.Errorf("expected error for missing session, got %s", resp)
	}
}

func TestSessionPromptMalformed(t *testing.T) {
	srv := newTestServer(t)
	srv.transport = newBufferTransport()
	ctx := context.Background()

	raw, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "session/prompt",
		"params":  "not-an-object",
	})
	srv.handleFrame(ctx, Message{Body: raw})
	bt := srv.transport.(*bufferTransport)
	resp := bt.sends[len(bt.sends)-1]
	if !strings.Contains(resp, `"code"`) {
		t.Errorf("expected error for malformed params, got %s", resp)
	}
}

func TestContentBlockSerialization(t *testing.T) {
	block := ContentBlock{Type: "text", Text: "hello"}
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded ContentBlock
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Type != "text" || decoded.Text != "hello" {
		t.Errorf("round-trip failed: %+v", decoded)
	}
}

func TestSessionUpdateSerialization(t *testing.T) {
	update := SessionUpdate{
		SessionUpdate: "tool_call",
		ToolCallID:    "call_1",
		Title:         "read file",
		Kind:          string(ToolKindRead),
		Status:        string(ToolCallPending),
	}
	data, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded SessionUpdate
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.SessionUpdate != "tool_call" {
		t.Errorf("expected tool_call, got %q", decoded.SessionUpdate)
	}
	if decoded.ToolCallID != "call_1" {
		t.Errorf("expected call_1, got %q", decoded.ToolCallID)
	}
}

func TestUsageUpdateSerialization(t *testing.T) {
	update := SessionUpdate{
		SessionUpdate: "usage_update",
		Used:          50000,
		Size:          200000,
		Cost:          &Cost{Amount: 0.05, Currency: "USD"},
	}
	data, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded SessionUpdate
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Used != 50000 {
		t.Errorf("expected 50000, got %d", decoded.Used)
	}
	if decoded.Cost == nil || decoded.Cost.Amount != 0.05 {
		t.Errorf("expected cost 0.05, got %+v", decoded.Cost)
	}
}

func TestPlanEntrySerialization(t *testing.T) {
	entry := PlanEntry{Content: "do something", Priority: "high", Status: "pending"}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded PlanEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Content != "do something" {
		t.Errorf("expected content, got %q", decoded.Content)
	}
}

func TestRPCErrorSerialization(t *testing.T) {
	err := &RPCError{Code: ErrCodeMethodNotFound, Message: "not found"}
	data, jerr := json.Marshal(err)
	if jerr != nil {
		t.Fatalf("marshal: %v", jerr)
	}
	if !strings.Contains(string(data), `"code":-32601`) {
		t.Errorf("expected code -32601, got %s", string(data))
	}
}

func TestResponseSerialization(t *testing.T) {
	result := json.RawMessage(`{"sessionId":"sess_1"}`)
	resp := Response{JSONRPC: "2.0", ID: 1, Result: result}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"jsonrpc":"2.0"`) {
		t.Errorf("expected jsonrpc field, got %s", data)
	}
}

func TestNotificationSerialization(t *testing.T) {
	n := struct {
		JSONRPC string          `json:"jsonrpc"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}{
		JSONRPC: "2.0",
		Method:  "session/update",
		Params:  json.RawMessage(`{"sessionId":"s1"}`),
	}
	data, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), "session/update") {
		t.Errorf("expected session/update, got %s", data)
	}
}

func TestIsNotification(t *testing.T) {
	notification := json.RawMessage(`{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"s1"}}`)
	request := json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"session/prompt","params":{}}`)

	if !isNotification(notification) {
		t.Error("expected notification without id to be detected")
	}
	if isNotification(request) {
		t.Error("expected request with id not to be a notification")
	}
}

func TestClientCallTimeout(t *testing.T) {
	srv := newTestServer(t)
	srv.transport = newBufferTransport()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	srv.client.timeout = 30 * time.Millisecond
	_, err := srv.client.ReadTextFile(ctx, "sess", "/tmp/file.txt", 0, 0)
	if err == nil {
		t.Error("expected error for timeout")
	}
}

func TestClientRequestIsSent(t *testing.T) {
	srv := newTestServer(t)
	bt := newBufferTransport()
	srv.transport = bt
	ctx := context.Background()

	go func() {
		time.Sleep(20 * time.Millisecond)
		var parsed struct {
			ID string `json:"id"`
		}
		if len(bt.sends) == 0 {
			return
		}
		json.Unmarshal([]byte(bt.sends[len(bt.sends)-1]), &parsed)
		resp, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      parsed.ID,
			"result":  map[string]any{"content": "file contents"},
		})
		srv.handleFrame(ctx, Message{Body: resp})
	}()

	out, err := srv.client.ReadTextFile(ctx, "sess", "/tmp/f.txt", 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "file contents" {
		t.Errorf("expected 'file contents', got %q", out)
	}
}

func TestJSONIdToString(t *testing.T) {
	if got := jsonIDToString("abc"); got != "abc" {
		t.Errorf("expected abc, got %q", got)
	}
	if got := jsonIDToString(42); got != "42" {
		t.Errorf("expected 42, got %q", got)
	}
	if got := jsonIDToString(3.5); got != "4" {
		t.Errorf("expected 4, got %q", got)
	}
	if got := jsonIDToString(nil); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestServeWithStdioStartEOF(t *testing.T) {
	tr := newBufferTransport()
	srv := newTestServer(t)
	close(tr.msg)
	ctx := context.Background()
	if err := srv.Serve(ctx, tr); err != nil {
		t.Fatalf("expected nil for closed transport, got %v", err)
	}
}

func TestServeContextCancel(t *testing.T) {
	tr := newBufferTransport()
	srv := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- srv.Serve(ctx, tr)
	}()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Error("expected context cancellation to return")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after context cancel")
	}
}
