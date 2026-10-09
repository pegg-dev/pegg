package mcp

import (
	"context"
	"fmt"
	json "github.com/goccy/go-json"
	"sync"
	"time"
)

type Client struct {
	name      string
	transport Duplex
	timeout   time.Duration

	mu        sync.Mutex
	nextID    int
	pending   map[int]chan *Response
	closeCh   chan struct{}
	closeOnce sync.Once
	initOnce  sync.Once
	info      InitializeResult
}

type Options struct {
	Name    string
	Timeout time.Duration
}

func NewClient(name string, transport Duplex, opts Options) *Client {
	if opts.Timeout <= 0 {
		opts.Timeout = 60 * time.Second
	}
	c := &Client{
		name:      name,
		transport: transport,
		timeout:   opts.Timeout,
		pending:   make(map[int]chan *Response),
		closeCh:   make(chan struct{}),
	}
	go c.readLoop()
	return c
}

func (c *Client) Initialize(ctx context.Context) (InitializeResult, error) {
	var err error
	c.initOnce.Do(func() {
		var result InitializeResult
		err = c.call(ctx, MethodInitialize, map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "pegg",
				"version": "0.1.0",
			},
		}, &result)
		if err != nil {
			return
		}
		if result.ProtocolVersion == "" {
			err = fmt.Errorf("%w: initialize missing protocolVersion", ErrProtocol)
			return
		}
		notif, err := buildNotification(MethodInitialized, nil)
		if err != nil {
			return
		}
		if err := c.transport.Write(notif); err != nil {
			return
		}
		c.info = result
	})
	if err != nil {
		return InitializeResult{}, err
	}
	return c.info, nil
}

func (c *Client) Ping(ctx context.Context) error {
	return c.call(ctx, MethodPing, nil, nil)
}

func (c *Client) ListTools(ctx context.Context) ([]ToolSpec, error) {
	var out []ToolSpec
	cursor := ""
	for {
		var result ListToolsResult
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		if err := c.call(ctx, MethodListTools, params, &result); err != nil {
			return nil, err
		}
		out = append(out, result.Tools...)
		if result.NextCursor == "" {
			break
		}
		cursor = result.NextCursor
	}
	return out, nil
}

func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]any) (CallToolResult, error) {
	params := map[string]any{"name": name}
	if arguments != nil {
		params["arguments"] = arguments
	}
	var result CallToolResult
	if err := c.call(ctx, MethodCallTool, params, &result); err != nil {
		return CallToolResult{}, err
	}
	return result, nil
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
