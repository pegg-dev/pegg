package http

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	json "github.com/goccy/go-json"
)

type Client struct {
	httpClient   *http.Client
	streamClient *http.Client
	headers      map[string]string
	baseURL      string
	timeout      time.Duration
	maxBodyBytes int64

	authHeader string
	authFn     func(context.Context) (string, error)
	refreshFn  func(context.Context) error
}

const defaultMaxBodyBytes int64 = 10 << 20

type Option func(*Client)

func NewClient(baseURL string, opts ...Option) *Client {
	c := &Client{
		headers:      make(map[string]string),
		baseURL:      baseURL,
		timeout:      60 * time.Second,
		maxBodyBytes: defaultMaxBodyBytes,
	}

	for _, opt := range opts {
		opt(c)
	}

	c.httpClient = &http.Client{Timeout: c.timeout}
	c.streamClient = &http.Client{Timeout: 0}

	return c
}

func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		c.timeout = timeout
	}
}

func WithMaxBody(maxBytes int64) Option {
	return func(c *Client) {
		c.maxBodyBytes = maxBytes
	}
}

func WithHeader(key, value string) Option {
	return func(c *Client) {
		c.headers[key] = value
	}
}

func WithAPIKey(apiKey string) Option {
	return func(c *Client) {
		c.headers["Authorization"] = "Bearer " + apiKey
	}
}

func WithAuthHeader(name string, fn func(context.Context) (string, error)) Option {
	return func(c *Client) {
		c.authHeader = name
		c.authFn = fn
	}
}

func WithRefresh(fn func(context.Context) error) Option {
	return func(c *Client) {
		c.refreshFn = fn
	}
}

func (c *Client) buildRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	fullURL, err := url.JoinPath(c.baseURL, path)
	if err != nil {
		return nil, fmt.Errorf("failed to build request url: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for key, value := range c.headers {
		req.Header.Set(key, value)
	}

	if c.authFn != nil && c.authHeader != "" {
		value, err := c.authFn(ctx)
		if err != nil {
			return nil, err
		}
		if value != "" {
			req.Header.Set(c.authHeader, value)
		}
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

func (c *Client) Do(ctx context.Context, method, path string, body any, result any) error {
	return c.do(ctx, method, path, body, result, c.refreshFn != nil)
}

func (c *Client) do(ctx context.Context, method, path string, body any, result any, canRetry bool) error {
	req, err := c.buildRequest(ctx, method, path, body)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBodyBytes+1))
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized && canRetry {
		if rerr := c.refreshFn(ctx); rerr == nil {
			return c.do(ctx, method, path, body, result, false)
		}
	}

	if resp.StatusCode >= 400 {
		if int64(len(respBody)) > c.maxBodyBytes {
			respBody = respBody[:c.maxBodyBytes]
		}
		return &HTTPError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}

	if int64(len(respBody)) > c.maxBodyBytes {
		return fmt.Errorf("response body exceeds %d bytes", c.maxBodyBytes)
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
}

func (e *HTTPError) Temporary() bool {
	return e.StatusCode == 429 || e.StatusCode >= 500
}

func (c *Client) DoStream(ctx context.Context, path string, body any, handler func(line []byte) error) error {
	return c.doStream(ctx, path, body, handler, c.refreshFn != nil)
}

func (c *Client) doStream(ctx context.Context, path string, body any, handler func(line []byte) error, canRetry bool) error {
	req, err := c.buildRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.streamClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, c.maxBodyBytes+1))
		if resp.StatusCode == http.StatusUnauthorized && canRetry {
			if rerr := c.refreshFn(ctx); rerr == nil {
				return c.doStream(ctx, path, body, handler, false)
			}
		}
		if int64(len(respBody)) > c.maxBodyBytes {
			respBody = respBody[:c.maxBodyBytes]
		}
		return &HTTPError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}

	decoder := NewLineDecoder(resp.Body)
	for {
		line, err := decoder.Decode()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if len(line) == 0 {
			continue
		}

		if err := handler(line); err != nil {
			return err
		}
	}

	return nil
}
