package tui

import (
	"context"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/gdamore/tcell/v2/terminfo"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/agents"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/update"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/router"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/skill"
	"github.com/peggco/pegg/internal/tui/components"
	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/page/home"
	"github.com/peggco/pegg/internal/tui/page/settings"
	"github.com/peggco/pegg/internal/tui/styles"
)

const (
	blinkInterval   = 450 * time.Millisecond
	renderInterval  = 16 * time.Millisecond
	doubleEscWindow = 2 * time.Second
	fullRepaintFreq = 1 * time.Second
)

type App struct {
	bus     event.Bus
	deps    settings.Deps
	agent   *agent.Agent
	screen  tcell.Screen
	root    components.Component
	overlay components.Component

	overlayMu sync.RWMutex
	chatMu    sync.Mutex

	home  *home.Page
	chat  *components.Chat
	model selectedModel

	main            *agentTranscript
	subs            map[string]*agentTranscript
	subItemByID     map[string]*components.ChatItem
	viewID          string
	running         bool
	usage           llm.Usage
	history         []llm.Message
	session         *activeSession
	loadedFloor     int
	reasoningEffort string

	baseMentionItems []components.ListItem

	ctx    context.Context
	cancel context.CancelFunc
	blink  bool
	quit   bool

	lastEsc     time.Time
	agentCancel context.CancelFunc
	escTimer    *time.Timer

	escHint  bool
	errorMsg string

	mouseX int
	mouseY int

	pasteActive bool
	pasteBuffer strings.Builder

	redrawPending atomic.Bool
}

type selectedModel struct {
	provider string
	model    llm.Model
}

type activeSession struct {
	info settings.SessionInfo

	viewID string
}

func (a *App) modelDisplay() string {
	if a.model.provider == router.SmartRouterModel {
		return "Smart Router"
	}
	if a.model.provider == "" {
		return ""
	}
	name := a.model.model.ID
	if a.model.model.Name != "" {
		name = a.model.model.Name
	}
	return a.model.provider + "/" + name
}

func (a *App) selectPreferred() {
	if a.deps.LLM == nil {
		return
	}
	go func() {
		res := a.deps.LLM.Select(llm.SelectRequest{Mode: llm.SelectModePreferred})
		if res.Err != nil {
			return
		}
		a.chatMu.Lock()
		a.model = selectedModel{provider: res.Provider, model: res.Model}
		a.refreshHomeLocked()
		a.chatMu.Unlock()
		a.requestRedraw()
	}()
}

func (a *App) setOverlay(c components.Component) {
	a.overlayMu.Lock()
	a.overlay = c
	a.overlayMu.Unlock()
}

func (a *App) getOverlay() components.Component {
	a.overlayMu.RLock()
	defer a.overlayMu.RUnlock()
	return a.overlay
}

func (a *App) requestRedraw() {
	a.redrawPending.Store(true)
}

func Run(bus event.Bus, deps settings.Deps) error {
	setBus(bus)

	s, err := newScreen()
	if err != nil {
		return fmt.Errorf("tui: create screen: %w", err)
	}
	if err := s.Init(); err != nil {
		return fmt.Errorf("tui: init screen: %w", err)
	}
	defer s.Fini()

	ctx, cancel := context.WithCancel(context.Background())
	a := &App{
		bus:         bus,
		deps:        deps,
		agent:       deps.Agent,
		screen:      s,
		subs:        make(map[string]*agentTranscript),
		subItemByID: make(map[string]*components.ChatItem),
		ctx:         ctx,
		cancel:      cancel,
	}
	return a.start()
}

func newScreen() (tcell.Screen, error) {
	if ti, err := tcell.LookupTerminfo(os.Getenv("TERM")); err == nil {
		if prep := prepareTerminfo(ti); prep != nil {
			if s, err := tcell.NewTerminfoScreenFromTtyTerminfo(newKeypadTty(os.Getenv("PEGG_KEYLOG")), prep); err == nil {
				return s, nil
			}
		}
	}
	return tcell.NewScreen()
}

func prepareTerminfo(ti *terminfo.Terminfo) *terminfo.Terminfo {
	if ti == nil {
		return nil
	}
	clone := *ti
	clone.EnterKeypad = ""
	clone.ExitKeypad = ""
	return &clone
}

func (a *App) start() error {
	if a.bus != nil {
		if err := a.subscribeChat(a.bus); err != nil {
			return err
		}
		defer a.unsubscribeChat(a.bus)
	}
	if a.cancel == nil {
		a.ctx, a.cancel = context.WithCancel(context.Background())
	}
	defer a.cancel()

	a.build()
	dispatchReady()
	defer dispatchQuit()
	return a.loop()
}

func (a *App) build() {
	styles.RegisterDefaults()

	if a.deps.Config != nil && a.deps.Config.Theme != "" {
		styles.Set(a.deps.Config.Theme)
	}

	a.selectPreferred()

	go a.checkForUpdates()

	if a.agent != nil {
		a.viewID = a.agent.ID
	}

	page := home.New()
	a.home = page
	a.chat = page.Chat()
	page.Input().OnSubmit = a.submitMessage
	page.Input().Focus()
	page.SetModel(a.modelDisplay())
	page.AttachmentBar().OnChange = func() {
		a.chatMu.Lock()
		defer a.chatMu.Unlock()
		a.refreshMentionItemsLocked()
	}

	a.chat.SetOnActivate(a.activateItem)
	a.chat.SetOnBack(a.backFromSubagent)
	a.chat.SetOnLoadMore(a.loadMore)
	a.chat.SetOnSubagentHistory(a.openSubagentHistory)

	var skillItems []components.ListItem
	for _, sk := range skill.List() {
		skillItems = append(skillItems, components.ListItem{Label: sk.Name, Detail: sk.Description})
	}
	page.SetSkills(skillItems)

	var mentionItems []components.ListItem
	for _, name := range agents.List() {
		a, err := agents.New(name)
		desc := ""
		if err == nil && a != nil {
			desc = a.Description
		}
		mentionItems = append(mentionItems, components.ListItem{Label: name, Detail: desc, Data: "agent"})
	}
	a.baseMentionItems = mentionItems
	page.SetMentionItems(mentionItems)

	if fs := a.deps.VFS; fs != nil {
		go func() {
			filePaths, _ := fs.Glob("**", "")
			sort.Strings(filePaths)
			a.chatMu.Lock()
			defer a.chatMu.Unlock()
			for _, p := range filePaths {
				detail := "file"
				data := "file"
				if fi, err := os.Stat(filepath.Join(fs.Root(), filepath.FromSlash(p))); err == nil && fi.IsDir() {
					detail = "folder"
					data = "folder"
				}
				a.baseMentionItems = append(a.baseMentionItems, components.ListItem{Label: p, Detail: detail, Data: data})
			}
			a.refreshMentionItemsLocked()
			a.requestRedraw()
		}()
	}

	comps := componentHook.Apply([]components.Component{page})
	if len(comps) > 0 {
		a.root = comps[0]
		if hp, ok := comps[0].(*home.Page); ok {
			a.home = hp
			a.chat = hp.Chat()
		}
	}
	a.refreshHomeLocked()

	th := styles.Current()
	a.screen.SetStyle(th.Base())
	a.screen.EnableMouse()
	a.screen.EnablePaste()
	a.screen.EnableFocus()
	a.draw()
}

func (a *App) checkForUpdates() {
	current := config.AppVersion
	ctx := context.Background()
	rel, found, err := update.DetectLatest(ctx)
	if err != nil || !found {
		return
	}

	if !update.IsNewerThan(current, rel.Version) {
		return
	}

	dismissed := update.GetDismissedVersion(a.deps.Cache)
	if dismissed == rel.Version {
		return
	}

	modal := NewUpdateModal(rel.Version,
		func() error {
			return update.UpdateToLatest(context.Background())
		},
		func() {
			update.SetDismissedVersion(a.deps.Cache, rel.Version)
			a.setOverlay(nil)
		},
	)
	modal.SetRedraw(func() { a.requestRedraw() })
	modal.SetQuit(func() {
		a.screen.Fini()
		os.Exit(0)
	})

	a.setOverlay(modal)
	a.requestRedraw()
}

func (a *App) loop() error {
	ticker := time.NewTicker(renderInterval)
	defer ticker.Stop()

	stopTick := make(chan struct{})
	defer close(stopTick)
	go func() {
		for {
			select {
			case <-stopTick:
				return
			case <-ticker.C:
				ev := &tcell.EventTime{}
				ev.SetEventNow()
				_ = a.screen.PostEvent(ev)
			}
		}
	}()

	lastBlink := time.Now()
	lastFullRepaint := time.Now()

	for {
		ev := a.screen.PollEvent()
		if ev == nil {
			return nil
		}
		switch e := ev.(type) {
		case *tcell.EventResize:
			a.screen.Sync()
			a.draw()
		case *tcell.EventPaste:
			if a.getOverlay() != nil {
				break
			}
			if e.Start() {
				a.pasteActive = true
				a.pasteBuffer.Reset()
			} else {
				a.pasteActive = false
				if a.pasteBuffer.Len() > 0 {
					a.handleFilePaste(a.pasteBuffer.String())
				}
				a.pasteBuffer.Reset()
			}
		case *tcell.EventKey:
			if a.pasteActive {
				if r := e.Rune(); r != 0 {
					a.pasteBuffer.WriteRune(r)
				}
				continue
			}
			if a.handleKey(e) {
				a.draw()
			}
		case *tcell.EventTime:
			now := time.Now()
			if now.Sub(lastBlink) >= blinkInterval {
				a.blink = !a.blink
				if t, ok := a.root.(components.Ticker); ok {
					t.OnTick(a.blink)
				}
				lastBlink = now
				a.redrawPending.Store(true)
			}
			if now.Sub(lastFullRepaint) >= fullRepaintFreq {
				a.screen.Clear()
				a.redrawPending.Store(true)
				lastFullRepaint = now
			}
			if a.redrawPending.Swap(false) {
				a.draw()
			}
		case *tcell.EventMouse:
			x, y := e.Position()
			a.mouseX, a.mouseY = x, y
			btns := e.Buttons()
			if ov := a.getOverlay(); ov != nil {
				if m, ok := ov.(components.MouseComponent); ok && m.HandleMouse(x, y, btns) {
					a.draw()
				}
				break
			}
			if a.home == nil {
				break
			}
			switch {
			case btns&tcell.WheelUp != 0:
				if a.home.HandleScroll(-3) {
					a.draw()
				}
			case btns&tcell.WheelDown != 0:
				if a.home.HandleScroll(3) {
					a.draw()
				}
			default:
				w, h := a.screen.Size()
				if a.home.HandleMouse(x, y, w, h, btns) {
					a.draw()
				}
			}
		case *tcell.EventFocus:
			if a.bus != nil {
				if e.Focused {
					a.bus.Publish(event.TopicAppFocused)
				} else {
					a.bus.Publish(event.TopicAppBlurred)
				}
			}
		}
		if a.quit {
			return nil
		}
	}
}

func (a *App) handleKey(ev *tcell.EventKey) bool {
	ke := dispatchKey(ev)
	if ke.Consumed {
		return true
	}
	kev := tcell.NewEventKey(ke.Key, ke.Rune, ke.Mod)

	if kev.Key() == tcell.KeyEsc {
		if a.escHandledLocally() || !a.running {
			a.chatMu.Lock()
			a.clearEscHint()
			a.chatMu.Unlock()
		} else if !a.lastEsc.IsZero() && time.Since(a.lastEsc) < doubleEscWindow {
			if a.agentCancel != nil {
				a.agentCancel()
			}
			a.chatMu.Lock()
			a.clearEscHint()
			a.chatMu.Unlock()
			return true
		} else {
			a.lastEsc = time.Now()
			a.escHint = true
			a.escTimer = time.AfterFunc(doubleEscWindow, func() {
				a.chatMu.Lock()
				a.clearEscHint()
				a.chatMu.Unlock()
			})
		}
	}

	if ov := a.getOverlay(); ov != nil {
		if resolveGlobal(kev) == ActionQuit {
			a.quit = true
			return false
		}
		if ov.HandleKey(kev) {
			return true
		}
		return true
	}

	if resolveGlobal(kev) == ActionCopy {
		a.copySelection()
		return true
	}

	if a.root != nil && a.root.HandleKey(kev) {
		return true
	}
	switch resolveGlobal(kev) {
	case ActionQuit:
		a.quit = true
	case ActionThemeNext:
		styles.Next()
		a.chat.Invalidate()
		if a.deps.Config != nil {
			a.deps.Config.Theme = styles.Name()
			go config.SaveTheme(styles.Name())
		}
		return true
	case ActionSettings:
		a.openSettings()
		return true
	}
	return false
}

func (a *App) copySelection() {
	a.chatMu.Lock()
	var text string
	if a.home != nil && a.home.Input().HasSelection() {
		text = a.home.Input().SelectedText()
	}
	if text == "" && a.chat != nil && a.chat.HasSelection() {
		text = a.chat.SelectedText()
	}
	a.chatMu.Unlock()
	if text == "" {
		return
	}
	a.screen.SetClipboard([]byte(text))
}

func (a *App) openSettings() {
	if a.getOverlay() != nil {
		return
	}
	s := settings.New(a.deps)
	s.SetSelectedModel(a.model.provider, a.model.model)
	s.SetOnModelChange(func(provider string, model llm.Model) {
		a.chatMu.Lock()
		a.model = selectedModel{provider: provider, model: model}
		a.reasoningEffort = ""
		a.refreshHomeLocked()
		a.chatMu.Unlock()
	})
	s.SetReasoningEffort(a.reasoningEffort)
	s.SetOnReasoningEffortChange(func(effort string) {
		a.chatMu.Lock()
		a.reasoningEffort = effort
		a.refreshHomeLocked()
		a.chatMu.Unlock()
	})
	if a.session != nil {
		info := a.session.info
		s.SetActiveSession(&info)
	}
	s.SetOnSessionChange(func(info settings.SessionInfo) {
		a.activateSession(info)
	})
	s.SetOnSessionClear(func() {
		a.chatMu.Lock()
		a.session = nil
		a.history = nil
		a.usage = llm.Usage{}
		a.chat.Clear()
		a.main = nil
		if a.agent != nil {
			a.agent.DetachReminder()
		}
		a.refreshHomeLocked()
		a.chatMu.Unlock()
		if a.agent != nil && a.bus != nil {
			a.bus.Publish(session.TopicSessionResume, session.SessionResume{AgentID: a.agent.ID})
		}
	})
	s.SetOnClose(func() { a.setOverlay(nil) })
	s.SetOnThemeChange(func() {
		a.chat.Invalidate()
		a.requestRedraw()
		if a.deps.Config != nil {
			a.deps.Config.Theme = styles.Name()
			go config.SaveTheme(styles.Name())
		}
	})
	s.SetOnRequestUpdate(func() {
		a.setOverlay(nil)
		rel, found, err := update.DetectLatest(context.Background())
		if err != nil || !found {
			return
		}
		modal := NewUpdateModal(rel.Version,
			func() error {
				return update.UpdateToLatest(context.Background())
			},
			func() {
				a.setOverlay(nil)
			},
		)
		modal.SetRedraw(func() { a.requestRedraw() })
		modal.SetQuit(func() {
			a.screen.Fini()
			os.Exit(0)
		})
		a.setOverlay(modal)
		a.requestRedraw()
	})
	s.SetRequestRedraw(func() { a.requestRedraw() })
	a.setOverlay(s)
}

func (a *App) escHandledLocally() bool {
	if a.getOverlay() != nil {
		return true
	}
	if a.home != nil && a.home.AskOpen() {
		return true
	}
	return a.chat != nil && a.chat.HasBack()
}

func (a *App) clearEscHint() {
	a.lastEsc = time.Time{}
	if a.escTimer != nil {
		a.escTimer.Stop()
		a.escTimer = nil
	}
	a.escHint = false
	a.requestRedraw()
}

func (a *App) refreshHomeLocked() {
	if a.home == nil {
		return
	}
	a.home.SetModel(a.modelDisplay())
	a.home.SetReasoningEffort(a.reasoningEffort)
	a.home.SetRunning(a.running)
	a.home.SetUsage(a.usage)
	if a.model.model.Config != nil {
		a.home.SetMaxInputTokens(a.model.model.MaxInputTokens())
	}
}

func (a *App) refreshMentionItemsLocked() {
	if a.home == nil {
		return
	}
	var agents, files []components.ListItem
	for _, it := range a.baseMentionItems {
		if it.Data == "agent" {
			agents = append(agents, it)
		} else {
			files = append(files, it)
		}
	}
	items := make([]components.ListItem, 0, len(a.baseMentionItems)+len(a.home.AttachmentBar().Attachments()))
	items = append(items, agents...)
	for _, att := range a.home.AttachmentBar().Attachments() {
		name := att.FileName
		if name == "" {
			name = "attachment"
		}
		name = strings.ReplaceAll(name, " ", "_")
		items = append(items, components.ListItem{Label: name, Detail: string(att.Type), Data: "attachment"})
	}
	items = append(items, files...)
	a.home.SetMentionItems(items)
}

func (a *App) activateSession(info settings.SessionInfo) {
	a.chatMu.Lock()
	changed := a.session == nil || a.session.info.ID != info.ID
	a.session = &activeSession{info: info, viewID: info.ID}
	if changed && a.agent != nil {
		a.agent.DetachReminder()
	}
	a.reasoningEffort = info.ReasoningEffort
	if a.deps.LLM != nil && info.Provider != "" && info.Model != "" {
		if res := a.deps.LLM.Select(llm.SelectRequest{
			Mode:     llm.SelectModeExact,
			Provider: info.Provider,
			Model:    info.Model,
		}); res.Err == nil {
			a.model = selectedModel{provider: res.Provider, model: res.Model}
		}
	}
	a.loadSessionIntoChatLocked()
	a.refreshHomeLocked()
	a.chatMu.Unlock()
	a.requestRedraw()
}

func (a *App) loadSessionIntoChatLocked() {
	if a.session == nil {
		return
	}
	if a.main == nil {
		id := ""
		if a.agent != nil {
			id = a.agent.ID
		}
		a.main = newTranscript(id, "orchestrator")
	}
	a.usage = a.session.info.Usage
	msgs := a.session.info.Messages
	if len(msgs) > initialChatMessages {
		msgs = msgs[len(msgs)-initialChatMessages:]
	}
	items := messagesToItems(msgs)
	a.main.items = items
	a.loadedFloor = seqFloor(msgs)
	hasMore := len(a.session.info.Messages) > len(msgs)
	if a.session.info.CompactionParentID != "" {
		hasMore = true
	}
	a.chat.SetHasMore(hasMore)
	mainID := ""
	if a.agent != nil {
		mainID = a.agent.ID
	}
	if a.viewID == "" || a.viewID == mainID {
		a.showTranscript(a.main)
		a.chat.SetBack(false)
	}
	a.refreshHomeLocked()
	a.seedHistoryFromSessionLocked()
}

func (a *App) seedHistoryFromSessionLocked() {
	msgs := a.session.info.Messages
	h := make([]llm.Message, 0, len(msgs)+1)
	if a.agent != nil && a.agent.SystemPrompt != "" {
		h = append(h, llm.SystemMessage(a.agent.SystemPrompt))
	}
	h = append(h, session.MessagesToLLM(msgs)...)
	a.history = h
}

const initialChatMessages = 50

func seqFloor(msgs []session.Message) int {
	floor := 0
	for _, m := range msgs {
		if floor == 0 || m.Seq < floor {
			floor = m.Seq
		}
	}
	return floor
}

func messageText(m session.Message) string {
	switch c := m.Content.(type) {
	case string:
		return c
	case []any:
		for _, item := range c {
			if mm, ok := item.(map[string]any); ok {
				if t, ok := mm["type"].(string); ok && t == "text" {
					if text, ok := mm["text"].(string); ok {
						return text
					}
				}
			}
		}
	}
	return ""
}

func (a *App) draw() {
	a.chatMu.Lock()
	defer a.chatMu.Unlock()
	a.drawLocked()
}

func (a *App) drawLocked() {
	w, h := a.screen.Size()
	if w < 1 || h < 1 {
		return
	}
	bounds := layout.Region{Left: 0, Top: 0, Width: w, Height: h}
	if a.root != nil {
		a.root.Draw(a.screen, bounds, true)
	}
	if ov := a.getOverlay(); ov != nil {
		ov.Draw(a.screen, bounds, true)
	}
	th := styles.Current()
	style := th.Base().Foreground(th.Error)
	hintY := h - 2
	switch {
	case a.errorMsg != "" && a.getOverlay() == nil && a.home != nil:
		a.home.DrawErrorBanner(a.screen, bounds, a.errorMsg, style)
	case a.errorMsg != "" && a.getOverlay() == nil:
		components.DrawText(a.screen, 2, hintY, a.errorMsg, style)
	case a.escHint && a.getOverlay() == nil:
		components.DrawText(a.screen, 2, hintY, "Press Esc to interrupt", style)
	}
	a.screen.Show()
}

func (a *App) handleFilePaste(text string) {
	if a.home != nil && a.home.AskOpen() {
		a.chatMu.Lock()
		a.home.AskPicker().InsertPaste(text)
		a.chatMu.Unlock()
		a.requestRedraw()
		return
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	lines := strings.Split(text, "\n")
	if len(lines) == 1 {
		line := lines[0]
		if _, err := os.Stat(line); err == nil {
			a.tryAttachFile(line)
			return
		}
		if len(line) > components.LongTextThreshold {
			a.chatMu.Lock()
			if a.home != nil {
				a.home.Input().InsertLongText(line)
			}
			a.chatMu.Unlock()
			a.requestRedraw()
			return
		}
		a.pasteShortText(text)
		return
	}

	anyAttached := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, err := os.Stat(line); err == nil {
			a.tryAttachFile(line)
			anyAttached = true
		}
	}
	if !anyAttached {
		if len(text) > components.LongTextThreshold {
			a.chatMu.Lock()
			if a.home != nil {
				a.home.Input().InsertLongText(text)
			}
			a.chatMu.Unlock()
			a.requestRedraw()
		} else {
			a.pasteShortText(text)
		}
	}
}

func (a *App) pasteShortText(text string) {
	a.chatMu.Lock()
	defer a.chatMu.Unlock()
	if a.home == nil {
		return
	}
	if a.home.AskOpen() {
		a.home.AskPicker().InsertPaste(text)
		a.requestRedraw()
		return
	}
	input := a.home.Input()
	for _, r := range text {
		input.InsertRune(r)
	}
	a.requestRedraw()
}

func (a *App) tryAttachFile(path string) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return
	}

	if !a.validateFileForModel(path) {
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	mediaType := mime.TypeByExtension(filepath.Ext(path))
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}

	var attType llm.AttachmentType
	switch {
	case strings.HasPrefix(mediaType, "image/"):
		attType = llm.AttachmentTypeImage
	case strings.HasPrefix(mediaType, "audio/"):
		attType = llm.AttachmentTypeAudio
	default:
		attType = llm.AttachmentTypeFile
	}

	att := llm.NewFileAttachmentFromBase64(mediaType, llm.EncodeFileToBase64(data), filepath.Base(path))
	att.Type = attType

	a.chatMu.Lock()
	if a.home != nil {
		a.home.AttachmentBar().Add(att)
		a.refreshMentionItemsLocked()
		a.refreshHomeLocked()
	}
	a.chatMu.Unlock()
	a.requestRedraw()
}

func (a *App) validateFileForModel(path string) bool {
	if a.model.model.Config == nil {
		return true
	}

	mediaType := mime.TypeByExtension(filepath.Ext(path))
	config := a.model.model.Config

	switch {
	case strings.HasPrefix(mediaType, "image/"):
		if config.Modalities == nil || !strings.Contains(strings.Join(config.Modalities.Input, ","), "image") {
			a.showError("Model does not support image attachments")
			return false
		}
	case strings.HasPrefix(mediaType, "audio/"):
		if config.Modalities == nil || !strings.Contains(strings.Join(config.Modalities.Input, ","), "audio") {
			a.showError("Model does not support audio attachments")
			return false
		}
	}
	return true
}

func (a *App) showError(msg string) {
	a.chatMu.Lock()
	defer a.chatMu.Unlock()
	if a.main != nil {
		a.appendItem(a.main, &components.ChatItem{Kind: components.ItemError, Text: msg})
		a.requestRedraw()
	}
}
