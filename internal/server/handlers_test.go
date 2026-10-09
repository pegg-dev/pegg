package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/session"
)

func TestHandleHealth(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()

	srv.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected status ok, got %s", resp.Status)
	}
	if resp.Version == "" {
		t.Error("expected version to be set")
	}
}

func TestHandleListSessions(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/sessions", nil)
	w := httptest.NewRecorder()
	srv.handleListSessions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp ListSessionsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(resp.Sessions))
	}
}

func TestHandleListSessionsWithQueryParams(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/sessions?page=2&size=10", nil)
	w := httptest.NewRecorder()
	srv.handleListSessions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleListSessionsInvalidParams(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/sessions?page=abc&size=xyz", nil)
	w := httptest.NewRecorder()
	srv.handleListSessions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with invalid params (uses defaults), got %d", w.Code)
	}
}

func TestHandleListSessionsSizeOverflow(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/sessions?size=999", nil)
	w := httptest.NewRecorder()
	srv.handleListSessions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with oversized param (caps at 100), got %d", w.Code)
	}
}

func TestHandleListSessionsWithSessions(t *testing.T) {
	srv := newTestServer(t)

	srv.sessions.Create(session.CreateOptions{Title: "first"})
	srv.sessions.Create(session.CreateOptions{Title: "second"})
	srv.sessions.Create(session.CreateOptions{Title: "third"})

	req := httptest.NewRequest("GET", "/api/sessions", nil)
	w := httptest.NewRecorder()
	srv.handleListSessions(w, req)

	var resp ListSessionsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Total != 3 {
		t.Errorf("expected total 3, got %d", resp.Total)
	}
}

func TestHandleGetSession(t *testing.T) {
	srv := newTestServer(t)

	sess, _ := srv.sessions.Create(session.CreateOptions{Title: "test session"})
	id := sess.ID

	req := httptest.NewRequest("GET", "/api/sessions/"+id, nil)
	w := httptest.NewRecorder()
	req.SetPathValue("id", id)
	srv.handleGetSession(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp SessionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ID != id {
		t.Errorf("expected ID %s, got %s", id, resp.ID)
	}
	if resp.Title != "test session" {
		t.Errorf("expected Title 'test session', got %s", resp.Title)
	}
}

func TestHandleGetSessionNotFound(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/sessions/nonexistent", nil)
	w := httptest.NewRecorder()
	req.SetPathValue("id", "nonexistent")
	srv.handleGetSession(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleGetSessionEmptyID(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/sessions/", nil)
	w := httptest.NewRecorder()
	req.SetPathValue("id", "")
	srv.handleGetSession(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleGetSessionMessages(t *testing.T) {
	srv := newTestServer(t)

	sess, _ := srv.sessions.Create(session.CreateOptions{Title: "test"})
	id := sess.ID

	req := httptest.NewRequest("GET", "/api/sessions/"+id+"/messages", nil)
	w := httptest.NewRecorder()
	req.SetPathValue("id", id)
	srv.handleGetSessionMessages(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Messages []MessageResponse `json:"messages"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(resp.Messages))
	}
}

func TestHandleGetSessionMessagesNotFound(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/sessions/nonexistent/messages", nil)
	w := httptest.NewRecorder()
	req.SetPathValue("id", "nonexistent")
	srv.handleGetSessionMessages(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleGetSessionMessagesEmptyID(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/sessions//messages", nil)
	w := httptest.NewRecorder()
	req.SetPathValue("id", "")
	srv.handleGetSessionMessages(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleDeleteSession(t *testing.T) {
	srv := newTestServer(t)

	sess, _ := srv.sessions.Create(session.CreateOptions{Title: "to delete"})
	id := sess.ID

	req := httptest.NewRequest("DELETE", "/api/sessions/"+id, nil)
	w := httptest.NewRecorder()
	req.SetPathValue("id", id)
	srv.handleDeleteSession(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "deleted" {
		t.Errorf("expected status deleted, got %s", resp["status"])
	}

	_, err := srv.sessions.Get(id)
	if err == nil {
		t.Error("expected session to be deleted")
	}
}

func TestHandleDeleteSessionNotFound(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("DELETE", "/api/sessions/nonexistent", nil)
	w := httptest.NewRecorder()
	req.SetPathValue("id", "nonexistent")
	srv.handleDeleteSession(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleDeleteSessionEmptyID(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("DELETE", "/api/sessions/", nil)
	w := httptest.NewRecorder()
	req.SetPathValue("id", "")
	srv.handleDeleteSession(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleRunEmptyBody(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("POST", "/api/run", strings.NewReader("{}"))
	w := httptest.NewRecorder()
	srv.handleRun(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleRunInvalidJSON(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("POST", "/api/run", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	srv.handleRun(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleRunMissingMessage(t *testing.T) {
	srv := newTestServer(t)

	body, _ := json.Marshal(RunRequest{})
	req := httptest.NewRequest("POST", "/api/run", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleRun(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if !strings.Contains(resp.Error, "message is required") {
		t.Errorf("expected error about message required, got %s", resp.Error)
	}
}

func TestHandleRunInvalidFile(t *testing.T) {
	srv := newTestServer(t)

	body, _ := json.Marshal(RunRequest{
		Message: "test",
		Files:   []string{"/nonexistent/file.txt"},
	})
	req := httptest.NewRequest("POST", "/api/run", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleRun(w, req)

	if w.Code != http.StatusBadRequest && w.Code != http.StatusInternalServerError {
		t.Errorf("expected 400 or 500, got %d", w.Code)
	}
}

func TestHandleRunSessionNotFound(t *testing.T) {
	srv := newTestServer(t)

	body, _ := json.Marshal(RunRequest{
		Message:   "test",
		SessionID: "nonexistent",
	})
	req := httptest.NewRequest("POST", "/api/run", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleRun(w, req)

	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("expected 404 or 500, got %d", w.Code)
	}
}

func TestLoadAttachments(t *testing.T) {
	tmpFile := t.TempDir() + "/test.txt"
	if err := os.WriteFile(tmpFile, []byte("hello world"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	atts, err := loadAttachments([]string{tmpFile})
	if err != nil {
		t.Fatalf("failed to load attachments: %v", err)
	}

	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}
	if atts[0].FileName != "test.txt" {
		t.Errorf("expected filename test.txt, got %s", atts[0].FileName)
	}
}

func TestLoadAttachmentsNotFound(t *testing.T) {
	_, err := loadAttachments([]string{"/nonexistent/file.txt"})
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadAttachmentsEmpty(t *testing.T) {
	atts, err := loadAttachments([]string{})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(atts) != 0 {
		t.Errorf("expected 0 attachments, got %d", len(atts))
	}
}

func TestLoadAttachmentsNil(t *testing.T) {
	atts, err := loadAttachments(nil)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(atts) != 0 {
		t.Errorf("expected 0 attachments, got %d", len(atts))
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %s", resp["status"])
	}
}

func TestWriteJSONWithNilData(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, nil)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "bad input")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error != "bad input" {
		t.Errorf("expected error 'bad input', got %s", resp.Error)
	}
}

func TestWriteErrorEmptyMessage(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusInternalServerError, "")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}

	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error != "" {
		t.Errorf("expected empty error, got %s", resp.Error)
	}
}
