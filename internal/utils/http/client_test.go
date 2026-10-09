package http

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	json "github.com/goccy/go-json"
)

func TestNewClient(t *testing.T) {
	client := NewClient("http://example.com")
	if client.baseURL != "http://example.com" {
		t.Errorf("baseURL = %q, want %q", client.baseURL, "http://example.com")
	}
	if client.timeout != 60*time.Second {
		t.Errorf("timeout = %v, want %v", client.timeout, 60*time.Second)
	}
	if client.headers == nil {
		t.Error("headers should be initialized")
	}
	if client.streamClient == nil {
		t.Error("streamClient should be initialized")
	}
	if client.streamClient.Timeout != 0 {
		t.Errorf("streamClient.Timeout = %v, want 0 (no total deadline for streams)", client.streamClient.Timeout)
	}
}

func TestDoStream_NoTotalTimeout(t *testing.T) {
	const thinkDelay = 200 * time.Millisecond
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		time.Sleep(thinkDelay)
		fmt.Fprintf(w, "data: first\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
		w.(http.Flusher).Flush()
	}))
	defer server.Close()

	client := NewClient(server.URL, WithTimeout(50*time.Millisecond))
	var got []string
	err := client.DoStream(context.Background(), "/stream", map[string]string{"q": "x"}, func(line []byte) error {
		got = append(got, string(line))
		return nil
	})
	if err != nil {
		t.Fatalf("DoStream() error = %v, want stream to survive past client timeout", err)
	}
	if len(got) == 0 || !strings.Contains(got[0], "first") {
		t.Errorf("streamed lines = %v, want first line", got)
	}
}

func TestDoStream_ContextCancelStopsStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		for {
			if _, err := fmt.Fprintf(w, "data: x\n\n"); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := client.DoStream(ctx, "/stream", nil, func(line []byte) error { return nil })
	if err == nil {
		t.Fatal("DoStream() should return error when ctx is canceled")
	}
}

func TestNewClient_WithTimeout(t *testing.T) {
	client := NewClient("http://example.com", WithTimeout(30*time.Second))
	if client.timeout != 30*time.Second {
		t.Errorf("timeout = %v, want %v", client.timeout, 30*time.Second)
	}
}

func TestNewClient_WithHeader(t *testing.T) {
	client := NewClient("http://example.com", WithHeader("X-Custom", "value"))
	if client.headers["X-Custom"] != "value" {
		t.Errorf("headers[X-Custom] = %q, want %q", client.headers["X-Custom"], "value")
	}
}

func TestNewClient_WithAPIKey(t *testing.T) {
	client := NewClient("http://example.com", WithAPIKey("secret"))
	if client.headers["Authorization"] != "Bearer secret" {
		t.Errorf("headers[Authorization] = %q, want %q", client.headers["Authorization"], "Bearer secret")
	}
}

func TestNewClient_MultipleOptions(t *testing.T) {
	client := NewClient(
		"http://example.com",
		WithTimeout(5*time.Second),
		WithAPIKey("key123"),
		WithHeader("X-App", "test"),
	)

	if client.timeout != 5*time.Second {
		t.Errorf("timeout = %v, want %v", client.timeout, 5*time.Second)
	}
	if client.headers["Authorization"] != "Bearer key123" {
		t.Errorf("headers[Authorization] = %q, want %q", client.headers["Authorization"], "Bearer key123")
	}
	if client.headers["X-App"] != "test" {
		t.Errorf("headers[X-App] = %q, want %q", client.headers["X-App"], "test")
	}
}

func TestBuildRequest_WithBody(t *testing.T) {
	client := NewClient("http://example.com")
	body := map[string]string{"key": "value"}

	req, err := client.buildRequest(context.Background(), http.MethodPost, "/api", body)
	if err != nil {
		t.Fatalf("buildRequest() error = %v", err)
	}

	if req.Method != http.MethodPost {
		t.Errorf("Method = %q, want %q", req.Method, http.MethodPost)
	}
	if req.URL.String() != "http://example.com/api" {
		t.Errorf("URL = %q, want %q", req.URL.String(), "http://example.com/api")
	}
	if req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want %q", req.Header.Get("Content-Type"), "application/json")
	}

	var reqBody map[string]string
	json.NewDecoder(req.Body).Decode(&reqBody)
	if reqBody["key"] != "value" {
		t.Errorf("body key = %q, want %q", reqBody["key"], "value")
	}
}

func TestBuildRequest_WithoutBody(t *testing.T) {
	client := NewClient("http://example.com")

	req, err := client.buildRequest(context.Background(), http.MethodGet, "/api", nil)
	if err != nil {
		t.Fatalf("buildRequest() error = %v", err)
	}

	if req.Method != http.MethodGet {
		t.Errorf("Method = %q, want %q", req.Method, http.MethodGet)
	}
	if req.Header.Get("Content-Type") != "" {
		t.Errorf("Content-Type should be empty for GET request")
	}
}

func TestBuildRequest_Headers(t *testing.T) {
	client := NewClient(
		"http://example.com",
		WithHeader("X-Custom", "test"),
		WithAPIKey("apikey"),
	)

	req, err := client.buildRequest(context.Background(), http.MethodGet, "/api", nil)
	if err != nil {
		t.Fatalf("buildRequest() error = %v", err)
	}

	if req.Header.Get("X-Custom") != "test" {
		t.Errorf("X-Custom = %q, want %q", req.Header.Get("X-Custom"), "test")
	}
	if req.Header.Get("Authorization") != "Bearer apikey" {
		t.Errorf("Authorization = %q, want %q", req.Header.Get("Authorization"), "Bearer apikey")
	}
}

func TestHTTPError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantMsg    string
		temporary  bool
	}{
		{
			name:       "400 Bad Request",
			statusCode: 400,
			body:       "invalid input",
			wantMsg:    "HTTP 400: invalid input",
			temporary:  false,
		},
		{
			name:       "404 Not Found",
			statusCode: 404,
			body:       "not found",
			wantMsg:    "HTTP 404: not found",
			temporary:  false,
		},
		{
			name:       "429 Too Many Requests",
			statusCode: 429,
			body:       "rate limited",
			wantMsg:    "HTTP 429: rate limited",
			temporary:  true,
		},
		{
			name:       "500 Internal Server Error",
			statusCode: 500,
			body:       "server error",
			wantMsg:    "HTTP 500: server error",
			temporary:  true,
		},
		{
			name:       "503 Service Unavailable",
			statusCode: 503,
			body:       "unavailable",
			wantMsg:    "HTTP 503: unavailable",
			temporary:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &HTTPError{
				StatusCode: tt.statusCode,
				Body:       tt.body,
			}

			if err.Error() != tt.wantMsg {
				t.Errorf("Error() = %q, want %q", err.Error(), tt.wantMsg)
			}
			if err.Temporary() != tt.temporary {
				t.Errorf("Temporary() = %v, want %v", err.Temporary(), tt.temporary)
			}
		})
	}
}

func TestDo_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	var result map[string]string
	err := client.Do(context.Background(), http.MethodGet, "/", nil, &result)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("result[status] = %q, want %q", result["status"], "ok")
	}
}

func TestDo_WithBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Method = %q, want %q", r.Method, http.MethodPost)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want %q", r.Header.Get("Content-Type"), "application/json")
		}

		var reqBody map[string]string
		json.NewDecoder(r.Body).Decode(&reqBody)
		if reqBody["input"] != "test" {
			t.Errorf("body input = %q, want %q", reqBody["input"], "test")
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"echo": reqBody["input"]})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	var result map[string]string
	err := client.Do(context.Background(), http.MethodPost, "/", map[string]string{"input": "test"}, &result)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result["echo"] != "test" {
		t.Errorf("result[echo] = %q, want %q", result["echo"], "test")
	}
}

func TestDo_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	var result map[string]string
	err := client.Do(context.Background(), http.MethodGet, "/", nil, &result)

	if err == nil {
		t.Fatal("Do() should return error")
	}

	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("error should be *HTTPError, got %T", err)
	}
	if httpErr.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, want %d", httpErr.StatusCode, http.StatusBadRequest)
	}
	if httpErr.Body != "bad request" {
		t.Errorf("Body = %q, want %q", httpErr.Body, "bad request")
	}
}

func TestDo_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	var result map[string]string
	err := client.Do(context.Background(), http.MethodGet, "/", nil, &result)

	if err == nil {
		t.Fatal("Do() should return error")
	}

	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("error should be *HTTPError, got %T", err)
	}
	if httpErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want %d", httpErr.StatusCode, http.StatusInternalServerError)
	}
	if !httpErr.Temporary() {
		t.Error("500 error should be temporary")
	}
}

func TestDo_NilResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.Do(context.Background(), http.MethodGet, "/", nil, nil)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
}

func TestDo_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	var result map[string]string
	err := client.Do(ctx, http.MethodGet, "/", nil, &result)
	if err == nil {
		t.Error("Do() should return error when context is cancelled")
	}
}

func TestDo_BodyTooLarge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("0123456789"))
	}))
	defer server.Close()

	client := NewClient(server.URL, WithMaxBody(4))
	var result map[string]string
	err := client.Do(context.Background(), http.MethodGet, "/", nil, &result)
	if err == nil {
		t.Fatal("Do() should return error for oversized body")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error = %q, want size limit error", err)
	}
}

func TestDoStream_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Method = %q, want %q", r.Method, http.MethodPost)
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("Accept = %q, want %q", r.Header.Get("Accept"), "text/event-stream")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: line1\ndata: line2\ndata: [DONE]\n"))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	var lines []string
	err := client.DoStream(context.Background(), "/", nil, func(line []byte) error {
		lines = append(lines, string(line))
		return nil
	})

	if err != nil {
		t.Fatalf("DoStream() error = %v", err)
	}
	if len(lines) < 1 {
		t.Fatalf("expected at least 1 line, got %d", len(lines))
	}
}

func TestDoStream_WithBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]string
		json.NewDecoder(r.Body).Decode(&reqBody)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: received\n\n"))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	var lines []string
	err := client.DoStream(context.Background(), "/", map[string]string{"input": "test"}, func(line []byte) error {
		lines = append(lines, string(line))
		return nil
	})

	if err != nil {
		t.Fatalf("DoStream() error = %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
}

func TestDoStream_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("unauthorized"))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.DoStream(context.Background(), "/", nil, func(line []byte) error {
		return nil
	})

	if err == nil {
		t.Fatal("DoStream() should return error")
	}

	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("error should be *HTTPError, got %T", err)
	}
	if httpErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want %d", httpErr.StatusCode, http.StatusUnauthorized)
	}
}

func TestDoStream_EmptyLines(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: line1\n\n\ndata: line2\n"))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	var lines []string
	err := client.DoStream(context.Background(), "/", nil, func(line []byte) error {
		lines = append(lines, string(line))
		return nil
	})

	if err != nil {
		t.Fatalf("DoStream() error = %v", err)
	}
	if len(lines) < 1 {
		t.Fatalf("expected at least 1 line, got %d: %v", len(lines), lines)
	}
}

func TestDoStream_HandlerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: line1\n\n"))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	handlerErr := http.ErrAbortHandler
	err := client.DoStream(context.Background(), "/", nil, func(line []byte) error {
		return handlerErr
	})

	if err != handlerErr {
		t.Errorf("DoStream() error = %v, want %v", err, handlerErr)
	}
}

func TestDo_AuthHeaderAndRefreshRetry(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			if got := r.Header.Get("Authorization"); got != "Bearer old" {
				t.Errorf("first auth = %q, want Bearer old", got)
			}
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"expired"}`))
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer new" {
			t.Errorf("second auth = %q, want Bearer new", got)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	token := "old"
	refreshed := false
	client := NewClient(srv.URL,
		WithAuthHeader("Authorization", func(context.Context) (string, error) { return "Bearer " + token, nil }),
		WithRefresh(func(context.Context) error { token = "new"; refreshed = true; return nil }),
	)
	var out map[string]bool
	if err := client.Do(context.Background(), "GET", "/x", nil, &out); err != nil {
		t.Fatal(err)
	}
	if !refreshed || calls != 2 || !out["ok"] {
		t.Fatalf("refreshed=%v calls=%d out=%v", refreshed, calls, out)
	}
}
