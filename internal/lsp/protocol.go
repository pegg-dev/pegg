package lsp

import (
	"fmt"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/lsp/diagnostic"
)

type Diagnostic = diagnostic.Diagnostic

const (
	JSONRPC = "2.0"

	MethodInitialize         = "initialize"
	MethodInitialized        = "notifications/initialized"
	MethodShutdown           = "shutdown"
	MethodExit               = "exit"
	MethodDidOpen            = "textDocument/didOpen"
	MethodDidChange          = "textDocument/didChange"
	MethodDidSave            = "textDocument/didSave"
	MethodDidClose           = "textDocument/didClose"
	MethodPublishDiagnostics = "textDocument/publishDiagnostics"
)

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("lsp: rpc error %d: %s", e.Code, e.Message)
}

type InitializeResult struct {
	Capabilities struct {
		PositionEncoding string `json:"positionEncoding,omitempty"`
	} `json:"capabilities,omitempty"`
	ServerInfo struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo,omitempty"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId,omitempty"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type DidOpenParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type TextDocumentContentChangeEvent struct {
	Text string `json:"text"`
}

type DidChangeParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges,omitempty"`
}

type DidSaveParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Text         string                 `json:"text,omitempty"`
}

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type DidCloseParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Version     int          `json:"version,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

func buildRequest(id int, method string, params any) ([]byte, error) {
	req := Request{JSONRPC: JSONRPC, ID: id, Method: method}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("lsp: marshal params: %w", err)
		}
		req.Params = raw
	}
	return json.Marshal(req)
}

func buildNotification(method string, params any) ([]byte, error) {
	n := Notification{JSONRPC: JSONRPC, Method: method}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("lsp: marshal params: %w", err)
		}
		n.Params = raw
	}
	return json.Marshal(n)
}

func parseResponse(data []byte) (*Response, error) {
	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProtocol, err)
	}
	return &resp, nil
}

func isNotification(data []byte) bool {
	var n struct {
		ID any `json:"id"`
	}
	if err := json.Unmarshal(data, &n); err != nil {
		return true
	}
	return n.ID == nil
}
