package lsp

import (
	"context"
	"fmt"
	json "github.com/goccy/go-json"
	"sync"
	"time"
)

type Client struct {
	name      string
	transport Transport
	timeout   time.Duration
	onDiag    func(uri string, version int, diags []Diagnostic)

	mu        sync.Mutex
	nextID    int
	pending   map[int]chan *Response
	closeCh   chan struct{}
	closeOnce sync.Once
	initOnce  sync.Once
	info      InitializeResult
	initErr   error
}

type ClientOptions struct {
	Name    string
	Timeout time.Duration
	OnDiag  func(uri string, version int, diags []Diagnostic)
}

func NewClient(transport Transport, opts ClientOptions) *Client {
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	c := &Client{
		name:      opts.Name,
		transport: transport,
		timeout:   opts.Timeout,
		onDiag:    opts.OnDiag,
		pending:   make(map[int]chan *Response),
		closeCh:   make(chan struct{}),
	}
	go c.readLoop()
	return c
}

func (c *Client) Initialize(ctx context.Context) (InitializeResult, error) {
	c.initOnce.Do(func() {
		var result InitializeResult
		params := map[string]any{
			"processId":    -1,
			"clientInfo":   map[string]any{"name": "pegg", "version": "0.1.0"},
			"capabilities": map[string]any{},
		}
		err := c.call(ctx, MethodInitialize, params, &result)
		if err != nil {
			c.initErr = err
			return
		}
		notif, err := buildNotification(MethodInitialized, nil)
		if err != nil {
			c.initErr = err
			return
		}
		if err := c.transport.Write(notif); err != nil {
			c.initErr = err
			return
		}
		c.info = result
	})
	if c.initErr != nil {
		return InitializeResult{}, c.initErr
	}
	return c.info, nil
}

func (c *Client) Ping(ctx context.Context) error {
	return c.call(ctx, MethodShutdown, nil, nil)
}

func (c *Client) DidOpen(ctx context.Context, item TextDocumentItem) error {
	return c.notify(MethodDidOpen, DidOpenParams{TextDocument: item})
}

func (c *Client) DidChange(ctx context.Context, uri string, version int, text string) error {
	params := DidChangeParams{
		TextDocument: VersionedTextDocumentIdentifier{URI: uri, Version: version},
		ContentChanges: []TextDocumentContentChangeEvent{
			{Text: text},
		},
	}
	return c.notify(MethodDidChange, params)
}

func (c *Client) DidSave(ctx context.Context, uri string, text string) error {
	params := DidSaveParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Text:         text,
	}
	return c.notify(MethodDidSave, params)
}

func (c *Client) DidClose(ctx context.Context, uri string) error {
	params := DidCloseParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	}
	return c.notify(MethodDidClose, params)
}

func (c *Client) notify(method string, params any) error {
	select {
	case <-c.closeCh:
		return ErrClosed
	default:
	}
	payload, err := buildNotification(method, params)
	if err != nil {
		return err
	}
	return c.transport.Write(payload)
}

func (c *Client) Shutdown(ctx context.Context) error {
	_ = c.call(ctx, MethodShutdown, nil, nil)
	notif, err := buildNotification(MethodExit, nil)
	if err != nil {
		return err
	}
	return c.transport.Write(notif)
}

func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		close(c.closeCh)
		_ = c.transport.Close()
	})
	return nil
}

func (c *Client) call(ctx context.Context, method string, params any, result any) error {
	select {
	case <-c.closeCh:
		return ErrClosed
	default:
	}

	c.mu.Lock()
	if err := ctx.Err(); err != nil {
		c.mu.Unlock()
		return err
	}
	c.nextID++
	id := c.nextID
	respCh := make(chan *Response, 1)
	c.pending[id] = respCh
	c.mu.Unlock()

	payload, err := buildRequest(id, method, params)
	if err != nil {
		c.dropPending(id)
		return err
	}

	if err := c.transport.Write(payload); err != nil {
		c.dropPending(id)
		return err
	}

	select {
	case resp := <-respCh:
		if resp.Error != nil {
			return resp.Error
		}
		if result != nil && len(resp.Result) > 0 {
			if err := json.Unmarshal(resp.Result, result); err != nil {
				return fmt.Errorf("%w: unmarshal %s result: %v", ErrProtocol, method, err)
			}
		}
		return nil
	case <-time.After(c.timeout):
		c.dropPending(id)
		return fmt.Errorf("%w: %s (id %d)", ErrTimeout, method, id)
	case <-ctx.Done():
		c.dropPending(id)
		return ctx.Err()
	}
}

func (c *Client) readLoop() {
	for {
		select {
		case <-c.closeCh:
			return
		default:
		}

		frame, err := c.transport.ReadFrame()
		if err != nil {
			c.failAll(err)
			return
		}
		if len(frame) == 0 {
			continue
		}

		if isNotification(frame) {
			c.handleNotification(frame)
			continue
		}

		resp, err := parseResponse(frame)
		if err != nil {
			continue
		}

		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		if ok {
			delete(c.pending, resp.ID)
		}
		c.mu.Unlock()
		if ok {
			ch <- resp
		}
	}
}

func (c *Client) handleNotification(frame []byte) {
	var n Notification
	if err := json.Unmarshal(frame, &n); err != nil {
		return
	}
	if n.Method != MethodPublishDiagnostics {
		return
	}
	var params PublishDiagnosticsParams
	if err := json.Unmarshal(n.Params, &params); err != nil {
		return
	}
	if c.onDiag != nil {
		c.onDiag(params.URI, params.Version, params.Diagnostics)
	}
}

func (c *Client) dropPending(id int) {
	c.mu.Lock()
	if ch, ok := c.pending[id]; ok {
		delete(c.pending, id)
		ch <- &Response{Error: &RPCError{Code: -1, Message: "client cancelled request"}}
	}
	c.mu.Unlock()
}

func (c *Client) failAll(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		delete(c.pending, id)
		ch <- &Response{Error: &RPCError{Code: -2, Message: err.Error()}}
	}
}

func (c *Client) ServerInfo() InitializeResult {
	return c.info
}
