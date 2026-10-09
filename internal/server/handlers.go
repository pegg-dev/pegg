package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/agents"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/utils/query"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{
		Status:  "ok",
		Version: config.AppVersion,
	})
}

func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	s.llmMgr.WaitUntilReady()

	type modelEntry struct {
		Provider string `json:"provider"`
		Model    any    `json:"model"`
	}

	var entries []modelEntry
	for _, p := range s.cfg.Providers {
		models, err := s.llmMgr.Models(p.Provider)
		if err != nil {
			continue
		}
		for _, m := range models {
			entries = append(entries, modelEntry{
				Provider: p.Provider,
				Model:    m,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"models": entries,
	})
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	var req RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	orch, err := agents.New("orchestrator")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create agent")
		return
	}
	orch.Bus = s.bus

	prov, mdl, err := s.resolveModel(req.Provider, req.Model)
	if err != nil {
		writeError(w, http.StatusBadRequest, "model selection failed: "+err.Error())
		return
	}
	orch.SetModelProvider(mdl, prov)

	if len(req.Files) > 0 {
		atts, err := loadAttachments(req.Files)
		if err != nil {
			writeError(w, http.StatusBadRequest, "failed to load attachments: "+err.Error())
			return
		}
		orch.Attachments = atts
	}

	ctx, stop := signal.NotifyContext(r.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var history []llm.Message
	if req.SessionID != "" {
		msgs, err := s.sessions.Messages(req.SessionID)
		if err != nil {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		if orch.SystemPrompt != "" {
			history = append(history, llm.SystemMessage(orch.SystemPrompt))
		}
		history = append(history, session.MessagesToLLM(msgs)...)
		s.bus.Publish(session.TopicSessionResume, session.SessionResume{
			AgentID:   orch.ID,
			SessionID: req.SessionID,
		})
	}

	if req.Stream {
		s.handleRunStream(w, ctx, orch, req.Message, history)
	} else {
		s.handleRunSync(w, ctx, orch, req.Message, history)
	}
}

func (s *Server) handleRunStream(w http.ResponseWriter, ctx context.Context, orch *agent.Agent, message string, history []llm.Message) {
	sse, err := NewSSEWriter(w)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "sse not supported")
		return
	}
	defer sse.Close()

	eventCh := make(chan agent.StreamEvent, 64)
	unsubscribed := make(chan struct{})

	go func() {
		defer close(unsubscribed)
		for ev := range eventCh {
			sseEvent := SSEEvent{Type: string(ev.Type)}
			switch ev.Type {
			case agent.StreamToken:
				sseEvent.Content = ev.Content
				sseEvent.Reasoning = ev.Reasoning
			case agent.StreamToolCall:
				if ev.ToolCall != nil {
					sseEvent.ToolName = ev.ToolCall.Function.Name
					sseEvent.ToolArgs = ev.ToolCall.Function.Arguments
				}
			case agent.StreamToolResult:
				if ev.ToolCall != nil {
					sseEvent.ToolName = ev.ToolCall.Function.Name
				}
				sseEvent.ToolOutput = ev.ToolOutput
			case agent.StreamCompaction:
				sseEvent.Strategy = ev.Strategy
				sseEvent.Messages = ev.Messages
				sseEvent.Tokens = ev.Tokens
			case agent.StreamDone:
				sseEvent.Done = true
				if ev.Usage != nil {
					sseEvent.Usage = ev.Usage
				}
			}
			if err := sse.WriteEvent("agent", sseEvent); err != nil {
				return
			}
		}
	}()

	var result *agent.RunResult
	if len(history) > 0 {
		result, err = orch.ResumeStream(ctx, message, history, func(ev agent.StreamEvent) error {
			select {
			case eventCh <- ev:
			default:
			}
			return nil
		})
	} else {
		result, err = orch.RunStream(ctx, message, func(ev agent.StreamEvent) error {
			select {
			case eventCh <- ev:
			default:
			}
			return nil
		})
	}

	close(eventCh)
	<-unsubscribed

	if err != nil {
		sse.WriteEvent("error", SSEEvent{Error: err.Error()})
		return
	}

	finalEvent := SSEEvent{Type: "done", Done: true}
	if result != nil {
		finalEvent.Usage = result.Usage
	}
	sse.WriteEvent("done", finalEvent)
}

func (s *Server) handleRunSync(w http.ResponseWriter, ctx context.Context, orch *agent.Agent, message string, history []llm.Message) {
	var result *agent.RunResult
	var err error

	if len(history) > 0 {
		result, err = orch.ResumeStream(ctx, message, history, func(ev agent.StreamEvent) error { return nil })
	} else {
		result, err = orch.RunStream(ctx, message, func(ev agent.StreamEvent) error { return nil })
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"output":        result.Output,
		"usage":         result.Usage,
		"iterations":    result.Iterations,
		"finish_reason": result.FinishReason,
	})
}

func (s *Server) resolveModel(provider, model string) (llm.Provider, llm.Model, error) {
	if s.llmMgr == nil {
		return nil, llm.Model{}, errors.New("llm manager unavailable")
	}
	s.llmMgr.WaitUntilReady()

	mode := llm.SelectModePreferred
	if model != "" {
		mode = llm.SelectModeExact
	}
	res := s.llmMgr.Select(llm.SelectRequest{
		Mode:     mode,
		Provider: provider,
		Model:    model,
	})
	if res.Err != nil {
		return nil, llm.Model{}, fmt.Errorf("model selection failed: %w", res.Err)
	}
	prov, err := s.llmMgr.Provider(res.Provider)
	if err != nil {
		return nil, llm.Model{}, fmt.Errorf("model selection failed: %w", err)
	}
	return prov, res.Model, nil
}

func loadAttachments(paths []string) ([]llm.Attachment, error) {
	var out []llm.Attachment
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("read attachment %q: %w", p, err)
		}
		mediaType := "application/octet-stream"
		attType := llm.AttachmentTypeFile
		if strings.HasPrefix(mediaType, "image/") {
			attType = llm.AttachmentTypeImage
		}
		att := llm.NewAttachmentFromBase64(attType, mediaType, llm.EncodeFileToBase64(data))
		att.FileName = filepath.Base(p)
		out = append(out, att)
	}
	return out, nil
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	page := 1
	size := 20
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if sz := r.URL.Query().Get("size"); sz != "" {
		if v, err := strconv.Atoi(sz); err == nil && v > 0 && v <= 100 {
			size = v
		}
	}

	q := query.Query{
		Page: query.Page{Number: page, Size: size},
		Sort: []query.Sort{{Column: "updated_at", Dir: query.Desc}},
		Filters: []query.Filter{
			{Column: "compaction_parent_id", Operator: query.OpEqual, Value: ""},
		},
	}

	sessions, total, err := s.sessions.List(q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}

	resp := ListSessionsResponse{
		Sessions: make([]SessionResponse, len(sessions)),
		Total:    total,
	}
	for i, sess := range sessions {
		resp.Sessions[i] = toSessionResponse(&sess)
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "session id is required")
		return
	}

	sess, err := s.sessions.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	writeJSON(w, http.StatusOK, toSessionResponse(sess))
}

func (s *Server) handleGetSessionMessages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "session id is required")
		return
	}

	msgs, err := s.sessions.Messages(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	resp := make([]MessageResponse, len(msgs))
	for i, m := range msgs {
		resp[i] = toMessageResponse(&m)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"messages": resp,
	})
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "session id is required")
		return
	}

	if err := s.sessions.Delete(id); err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}
