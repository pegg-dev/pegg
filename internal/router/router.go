package router

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/hook"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/decision"
	"github.com/peggco/pegg/internal/llm"
)

const maxCandidates = 40

const maxTaskChars = 1500

const SmartRouterModel = "smart-router"

type smartRouterProvider struct{}

func (smartRouterProvider) Name() string { return SmartRouterModel }
func (smartRouterProvider) Chat(context.Context, *llm.Request) (*llm.Response, error) {
	return nil, errors.New("smart router: no model resolved (is smart_router.enabled set in config?)")
}
func (smartRouterProvider) ChatStream(context.Context, *llm.Request, llm.StreamHandler) error {
	return errors.New("smart router: no model resolved (is smart_router.enabled set in config?)")
}
func (smartRouterProvider) ListModels(context.Context) ([]llm.Model, error) { return nil, nil }

var SmartRouterProvider llm.Provider = smartRouterProvider{}

type ModelOption struct {
	Provider string
	ModelID  string
	Label    string
	Detail   string
}

var ModelOptionsHook = hook.NewHook[[]ModelOption]()

type Deps struct {
	Config   *config.Config
	LLM      *llm.Manager
	Decision *decision.Manager
}

type Router struct {
	deps Deps
	log  *logger.Logger
}

func New(deps Deps, log *logger.Logger) *Router {
	r := &Router{deps: deps, log: log}
	if deps.LLM != nil {
		deps.LLM.RegisterSystemProvider(SmartRouterModel, SmartRouterProvider, []llm.Model{{
			ID:   SmartRouterModel,
			Name: "Smart Router",
		}})
	}
	ModelOptionsHook.Add(func(opts []ModelOption) []ModelOption {
		if !r.Enabled() {
			return opts
		}
		for _, o := range opts {
			if o.Provider == SmartRouterModel {
				return opts
			}
		}
		return append([]ModelOption{{
			Provider: SmartRouterModel,
			ModelID:  SmartRouterModel,
			Label:    "Smart Router",
			Detail:   "auto-select per task",
		}}, opts...)
	})
	agent.OnModelResolve(func(mr agent.ModelResolve) agent.ModelResolve {
		if !r.Enabled() {
			return mr
		}
		a := mr.Agent
		if a == nil {
			return mr
		}
		if a.ParentAgentID == "" && a.Model.ID != SmartRouterModel {
			return mr
		}
		if a.ParentAgentID != "" && !a.RouterManaged {
			return mr
		}
		prov, mdl, err := r.Select(mr.Ctx, SelectRequest{
			AgentName: a.Name,
			Task:      mr.Input,
			HasImages: hasImageAttachments(a),
		})
		if err == nil {
			a.SetModelProvider(mdl, prov)
			a.RouterManaged = true
		}
		return mr
	})
	return r
}

func hasImageAttachments(a *agent.Agent) bool {
	for _, att := range a.Attachments {
		if att.Type == llm.AttachmentTypeImage {
			return true
		}
	}
	return false
}

func (r *Router) cfg() *config.Config {
	if r == nil {
		return nil
	}
	return r.deps.Config
}

func (r *Router) Enabled() bool {
	if r == nil || r.cfg() == nil || r.cfg().SmartRouter == nil {
		return false
	}
	return r.cfg().SmartRouter.Enabled
}

type candidate struct {
	provider string
	model    llm.Model
	key      string
}

type SelectRequest struct {
	AgentName  string
	Task       string
	HasImages  bool
	Difficulty string
}

const (
	DifficultyTrivial  = "trivial"
	DifficultyModerate = "moderate"
	DifficultyComplex  = "complex"
)

var difficultyCriteria = []string{
	"Trivial: a single small, well-understood action",
	"Moderate: a few files, some investigation, a contained change",
	"Complex: multi-step work, deep investigation, or broad changes",
}

func difficultyFromScore(score float64) string {
	if score < 0.5 {
		return DifficultyTrivial
	}
	if score < 1.5 {
		return DifficultyModerate
	}
	return DifficultyComplex
}

func (r *Router) Select(ctx context.Context, req SelectRequest) (llm.Provider, llm.Model, error) {
	if r.usesDifficulty(req.AgentName) && !req.HasImages {
		req.Difficulty = r.evaluateDifficulty(ctx, req)
	}

	cands, err := r.candidates(req)
	if err != nil {
		return r.preferred()
	}
	if len(cands) == 0 {
		return r.preferred()
	}

	if cand, ok, err := r.selectViaDecision(ctx, req, cands); err == nil && ok {
		return r.resolve(cand)
	}

	if cand, ok, err := r.selectViaLLM(ctx, req, cands); err == nil && ok {
		return r.resolve(cand)
	}

	return r.preferred()
}

func (r *Router) usesDifficulty(agentName string) bool {
	cfg := r.cfg()
	if cfg == nil || cfg.SmartRouter == nil {
		return false
	}
	acfg, ok := cfg.SmartRouter.Agents[agentName]
	if !ok {
		return false
	}
	return len(acfg.Difficulty) > 0
}

func (r *Router) evaluateDifficulty(ctx context.Context, req SelectRequest) string {
	prov, err := r.decisionProvider()
	if err != nil {
		return ""
	}
	model := ""
	if r.cfg().SmartRouter != nil {
		model = r.cfg().SmartRouter.Model
	}
	res, err := prov.Decide(ctx, &decision.Request{
		Model: model,
		State: buildState(req),
		Questions: map[string]decision.Question{
			"task_difficulty": decision.ScoreQuestion(
				"How difficult is this task for an AI coding agent?",
				difficultyCriteria,
			),
		},
	})
	if err != nil {
		return ""
	}
	ans, ok := res.Answers["task_difficulty"]
	if !ok {
		return ""
	}
	score, ok := ans.ScoreValue()
	if !ok {
		return ""
	}
	return difficultyFromScore(score)
}

func (r *Router) resolve(cand candidate) (llm.Provider, llm.Model, error) {
	if r.deps.LLM == nil {
		return nil, llm.Model{}, errors.New("router: llm manager unavailable")
	}
	prov, err := r.deps.LLM.Provider(cand.provider)
	if err != nil {
		return nil, llm.Model{}, err
	}
	return prov, cand.model, nil
}

func (r *Router) preferred() (llm.Provider, llm.Model, error) {
	if r.deps.LLM == nil {
		return nil, llm.Model{}, errors.New("router: llm manager unavailable")
	}
	res := r.deps.LLM.Select(llm.SelectRequest{Mode: llm.SelectModePreferred})
	if res.Err != nil || res.Model.ID == "" {
		return nil, llm.Model{}, errors.New("router: no models available")
	}
	prov, err := r.deps.LLM.Provider(res.Provider)
	if err != nil {
		return nil, llm.Model{}, err
	}
	return prov, res.Model, nil
}

func (r *Router) candidates(req SelectRequest) ([]candidate, error) {
	if r.deps.LLM == nil || r.cfg() == nil {
		return nil, errors.New("router: llm manager or config unavailable")
	}

	all, err := r.allModels()
	if err != nil {
		return nil, err
	}

	prefs := r.preferences(req)
	ordered := make([]candidate, 0, len(all))
	seen := make(map[string]bool)

	for _, pref := range prefs {
		for _, c := range all {
			if seen[c.key] {
				continue
			}
			if c.model.ID == pref || c.model.Name == pref {
				seen[c.key] = true
				ordered = append(ordered, c)
			}
		}
	}
	for _, c := range all {
		if seen[c.key] {
			continue
		}
		seen[c.key] = true
		ordered = append(ordered, c)
	}

	if req.HasImages {
		var vision []candidate
		for _, c := range ordered {
			if supportsImage(c.model) {
				vision = append(vision, c)
			}
		}
		if len(vision) > 0 {
			ordered = vision
		}
	}

	if len(ordered) > maxCandidates {
		ordered = ordered[:maxCandidates]
	}
	return ordered, nil
}

func (r *Router) preferences(req SelectRequest) []string {
	acfg, ok := r.cfg().SmartRouter.Agents[req.AgentName]
	if !ok {
		return nil
	}
	switch {
	case req.HasImages && len(acfg.Images) > 0:
		return acfg.Images
	case req.Difficulty != "" && len(acfg.Difficulty[req.Difficulty]) > 0:
		return acfg.Difficulty[req.Difficulty]
	default:
		return acfg.Default
	}
}

func (r *Router) allModels() ([]candidate, error) {
	var out []candidate
	for _, p := range r.cfg().Providers {
		if p.APIKey == "" {
			continue
		}
		models, err := r.deps.LLM.Models(p.Provider)
		if err != nil {
			continue
		}
		for _, m := range models {
			out = append(out, candidate{
				provider: p.Provider,
				model:    m,
				key:      p.Provider + "/" + m.ID,
			})
		}
	}
	if len(out) == 0 {
		return nil, errors.New("router: no models available")
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].provider != out[j].provider {
			return out[i].provider < out[j].provider
		}
		return out[i].model.ID < out[j].model.ID
	})
	return out, nil
}

func supportsImage(m llm.Model) bool {
	if m.Config == nil || m.Config.Modalities == nil {
		return false
	}
	for _, mod := range m.Config.Modalities.Input {
		if mod == "image" {
			return true
		}
	}
	return false
}

func (r *Router) selectViaDecision(ctx context.Context, req SelectRequest, cands []candidate) (candidate, bool, error) {
	prov, err := r.decisionProvider()
	if err != nil {
		return candidate{}, false, err
	}

	model := ""
	if r.cfg().SmartRouter != nil {
		model = r.cfg().SmartRouter.Model
	}

	criteria := make(map[string]string, len(cands))
	for _, c := range cands {
		criteria[c.key] = describeModel(c)
	}

	res, err := prov.Decide(ctx, &decision.Request{
		Model: model,
		State: buildState(req),
		Questions: map[string]decision.Question{
			"best_model": decision.ChoiceQuestion(
				"Select the single best suitable model for this agent and task from the options below.",
				criteria,
			),
		},
	})
	if err != nil {
		return candidate{}, false, err
	}

	answer, ok := res.Answers["best_model"]
	if !ok {
		return candidate{}, false, errors.New("router: decision returned no answer")
	}
	choice := answer.Choice
	if choice == "" {
		return candidate{}, false, errors.New("router: decision returned an invalid choice")
	}

	for _, c := range cands {
		if c.key == choice {
			return c, true, nil
		}
	}
	return candidate{}, false, fmt.Errorf("router: decision chose unknown model %q", choice)
}

func (r *Router) decisionProvider() (decision.Provider, error) {
	if r.deps.Decision == nil {
		return nil, errors.New("router: decision manager unavailable")
	}
	if r.cfg() != nil && r.cfg().SmartRouter != nil && r.cfg().SmartRouter.Provider != "" {
		return r.deps.Decision.Provider(r.cfg().SmartRouter.Provider)
	}
	return r.deps.Decision.Preferred()
}

func (r *Router) selectViaLLM(ctx context.Context, req SelectRequest, cands []candidate) (candidate, bool, error) {
	if r.deps.LLM == nil {
		return candidate{}, false, errors.New("router: llm manager unavailable")
	}
	res := r.deps.LLM.Select(llm.SelectRequest{Mode: llm.SelectModePreferred})
	if res.Err != nil || res.Model.ID == "" {
		return candidate{}, false, res.Err
	}
	prov, err := r.deps.LLM.Provider(res.Provider)
	if err != nil {
		return candidate{}, false, err
	}

	var b strings.Builder
	b.WriteString("You are a model router. A task is described below; pick the single best suitable model for it.\n\n")
	b.WriteString(buildState(req))
	b.WriteString("\nAvailable models:\n")
	for _, c := range cands {
		fmt.Fprintf(&b, "- %s: %s\n", c.key, describeModel(c))
	}
	b.WriteString("\nRespond with the exact key of the chosen model.")

	messages := []llm.Message{
		llm.SystemMessage(b.String()),
		llm.UserMessage(req.Task),
	}

	llmReq := llm.NewRequest(res.Model.ID, messages).WithStrictStructuredOutput("model_pick", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"model": map[string]any{"type": "string"},
		},
		"required":             []string{"model"},
		"additionalProperties": false,
	})

	out, err := prov.Chat(ctx, llmReq)
	if err != nil {
		return candidate{}, false, err
	}
	if len(out.Choices) == 0 || out.Choices[0].Message == nil {
		return candidate{}, false, errors.New("router: llm selection returned no response")
	}
	text := llm.MessageText(*out.Choices[0].Message)

	var picked struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &picked); err != nil {
		return candidate{}, false, fmt.Errorf("router: llm selection returned invalid output: %w", err)
	}
	if picked.Model == "" {
		return candidate{}, false, errors.New("router: llm selection returned an empty model")
	}
	for _, c := range cands {
		if c.key == picked.Model {
			return c, true, nil
		}
	}
	return candidate{}, false, fmt.Errorf("router: llm selected unknown model %q", picked.Model)
}

func buildState(req SelectRequest) string {
	task := strings.TrimSpace(req.Task)
	if len(task) > maxTaskChars {
		task = task[:maxTaskChars] + "…"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Agent: %s\n", req.AgentName)
	if task != "" {
		fmt.Fprintf(&b, "Task: %s\n", task)
	}
	if req.Difficulty != "" {
		fmt.Fprintf(&b, "Difficulty: %s\n", req.Difficulty)
	}
	if req.HasImages {
		b.WriteString("Images attached: yes — the model must support image input.\n")
	}
	return b.String()
}

func describeModel(c candidate) string {
	m := c.model
	var parts []string
	parts = append(parts, "provider="+c.provider)
	if m.Config != nil {
		if m.Config.MaxInputTokens > 0 {
			parts = append(parts, fmt.Sprintf("context=%dk", m.Config.MaxInputTokens/1000))
		}
		if m.Config.MaxOutputTokens > 0 {
			parts = append(parts, fmt.Sprintf("max_output=%dk", m.Config.MaxOutputTokens/1000))
		}
		if m.Config.SupportsReasoning {
			parts = append(parts, "reasoning=yes")
		}
		if m.Config.ToolCall {
			parts = append(parts, "tool_call=yes")
		}
		if supportsImage(m) {
			parts = append(parts, "vision=yes")
		}
		if m.Config.Family != "" {
			parts = append(parts, "family="+m.Config.Family)
		}
		if m.Config.OpenWeights {
			parts = append(parts, "open_weights=yes")
		}
		if m.Config.Description != "" {
			parts = append(parts, "about="+truncate(m.Config.Description, 60))
		}
	}
	desc := strings.Join(parts, "; ")
	if desc == "" {
		desc = m.ID
	}
	return truncate(desc, 160)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
