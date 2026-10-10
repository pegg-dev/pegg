package bootstrap

import (
	"context"
	"fmt"
	"os"

	"github.com/peggco/pegg/internal/builtin"
	_ "github.com/peggco/pegg/internal/builtin/agents"
	_ "github.com/peggco/pegg/internal/builtin/lsps"
	"github.com/peggco/pegg/internal/builtin/tools/subagent"
	"github.com/peggco/pegg/internal/cli"
	"github.com/peggco/pegg/internal/core/cache"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/decision"
	_ "github.com/peggco/pegg/internal/decision/providers"
	"github.com/peggco/pegg/internal/llm"
	_ "github.com/peggco/pegg/internal/llm/drivers"
	_ "github.com/peggco/pegg/internal/llm/providers"
	"github.com/peggco/pegg/internal/lsp"
	"github.com/peggco/pegg/internal/mcp"
	"github.com/peggco/pegg/internal/memory"
	"github.com/peggco/pegg/internal/notification"
	"github.com/peggco/pegg/internal/plugin"
	"github.com/peggco/pegg/internal/router"
	"github.com/peggco/pegg/internal/schedule"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/skill"
	"github.com/peggco/pegg/internal/utils/query"
	"github.com/peggco/pegg/internal/vfs"
)

func Run(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("bootstrap: load config: %w", err)
	}

	if err := config.SaveIfNotExists(cfg); err != nil {
		return fmt.Errorf("bootstrap: save default config: %w", err)
	}

	if err := config.EnsureProjectConfigDir(); err != nil {
		return fmt.Errorf("bootstrap: create project config dir: %w", err)
	}

	if os.Getenv("PEGG_SCHEDULE_RUN") == "1" && cfg.Permission != nil {
		schedule.ApplyScheduledPermissions(cfg.Permission)
	}

	log, err := logger.LoggerModule(cfg.Logger)
	if err != nil {
		return fmt.Errorf("bootstrap: init logger: %w", err)
	}
	defer log.Close()

	bus := event.New()

	notif, err := notification.NotificationModule(cfg.Notification, bus, log)
	if err != nil {
		log.Fwarn("notification: failed to initialize: %v", err)
	}
	if notif != nil {
		defer notif.Close()
	}

	cacheStore, err := cache.CacheModule(cfg.Cache)
	if err != nil {
		return fmt.Errorf("bootstrap: init cache: %w", err)
	}
	defer cacheStore.Close()

	mgr := llm.NewManager(bus, log, cacheStore)
	if err := mgr.Start(); err != nil {
		return fmt.Errorf("bootstrap: init llm manager: %w", err)
	}
	defer mgr.Shutdown()

	decMgr := decision.NewManager(bus, log)
	if err := decMgr.Start(); err != nil {
		return fmt.Errorf("bootstrap: init decision manager: %w", err)
	}
	defer decMgr.Shutdown()

	_ = router.New(router.Deps{Config: cfg, LLM: mgr, Decision: decMgr}, log)

	memMgr := memory.New(memory.Deps{
		Config:   cfg.Memory,
		LLM:      mgr,
		Decision: decMgr,
		Bus:      bus,
		Log:      log,
	})

	bus.Publish(event.TopicAppMounted, cfg)

	sess, err := session.SessionModule(cfg.Session, bus, log)
	if err != nil {
		return fmt.Errorf("bootstrap: init session manager: %w", err)
	}
	defer sess.Close()

	mgr.SetSessionResolver(func() (string, string, bool) {
		dir, err := os.Getwd()
		if err != nil {
			return "", "", false
		}
		if prov, model, ok := sessionModel(sess, dir); ok {
			return prov, model, true
		}
		return sessionModel(sess, "")
	})

	rec := session.NewRecorder(sess, log)
	if err := rec.Start(bus); err != nil {
		return fmt.Errorf("bootstrap: init session recorder: %w", err)
	}
	defer rec.Stop(bus)

	sched, err := schedule.New(schedule.Deps{
		Config:   cfg,
		Bus:      bus,
		Log:      log,
		Sessions: sess,
		LLM:      mgr,
	})
	if err != nil {
		return fmt.Errorf("bootstrap: init scheduler: %w", err)
	}

	fs, err := vfs.VFSModule(log)
	if err != nil {
		return fmt.Errorf("bootstrap: init vfs: %w", err)
	}
	if err := skill.SkillModule(); err != nil {
		return fmt.Errorf("bootstrap: init skills: %w", err)
	}
	if err := builtin.Create(fs, sess, builtin.Options{LLM: mgr, Decision: decMgr, Config: cfg, Bus: bus, Memory: memMgr, Scheduler: sched}); err != nil {
		return fmt.Errorf("bootstrap: builtin: %w", err)
	}
	log.Finfo("vfs: workspace mounted at %s", fs.Root())

	if os.Getenv("PEGG_SCHEDULER") != "0" {
		if err := sched.Start(context.Background()); err != nil {
			return fmt.Errorf("bootstrap: start scheduler: %w", err)
		}
		defer sched.Stop()
	}

	mcpMgr, err := mcp.Module(cfg.MCPServers, log)
	if err != nil {
		return fmt.Errorf("bootstrap: init mcp: %w", err)
	}
	defer mcpMgr.Close()

	lspMgr, err := lsp.Module(cfg.LanguageServers, fs, log)
	if err != nil {
		return fmt.Errorf("bootstrap: init lsp: %w", err)
	}
	defer lspMgr.Close()

	pluginMgr, err := plugin.Module(cfg, bus, log, cacheStore, mgr, sess, fs, mcpMgr, lspMgr)
	if err != nil {
		log.Fwarn("plugin: failed to initialize: %v", err)
	}
	if pluginMgr != nil {
		defer pluginMgr.Close()
	}

	log.Info("application started")

	app := cli.New(bus, cfg, log, fs, sess, mgr, decMgr, memMgr, mcpMgr, lspMgr, cacheStore, pluginMgr, sched)
	if err := app.Execute(args); err != nil {
		return fmt.Errorf("bootstrap: cli: %w", err)
	}

	log.Info("application stopped")
	return nil
}

func sessionModel(sess *session.Manager, dir string) (string, string, bool) {
	q := query.Query{
		Page: query.Page{Number: 1, Size: 20},
		Sort: []query.Sort{{Column: "updated_at", Dir: query.Desc}},
	}
	if dir != "" {
		q.Filters = []query.Filter{
			{Column: "project_dir", Operator: query.OpEqual, Value: dir},
		}
	}
	sessions, _, err := sess.List(q)
	if err != nil {
		return "", "", false
	}
	for _, s := range sessions {
		if subagent.IsSubagentSession(s.ID) {
			continue
		}
		if s.Provider == "" || s.Model == "" {
			continue
		}
		return s.Provider, s.Model, true
	}
	return "", "", false
}
