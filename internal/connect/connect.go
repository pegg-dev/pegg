package connect

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/peggco/pegg/internal/core/cache"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/decision"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/lsp"
	"github.com/peggco/pegg/internal/mcp"
	"github.com/peggco/pegg/internal/memory"
	"github.com/peggco/pegg/internal/plugin"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/vfs"
)

const (
	defaultHeartbeatSecs = 20
	handshakeTimeout     = 30 * time.Second
	outQueueSize         = 4096
	maxFrameBytesDefault = 8 << 20
)

type Deps struct {
	Config   *config.Config
	Bus      event.Bus
	Log      *logger.Logger
	VFS      *vfs.VFS
	Sessions *session.Manager
	LLM      *llm.Manager
	Decision *decision.Manager
	Memory   *memory.Manager
	MCP      *mcp.Manager
	LSP      *lsp.Manager
	Cache    cache.Cache
	Plugin   *plugin.Manager
	Version  string
	Trace    func(format string, args ...any)
}

type Connector struct {
	deps     Deps
	cfg      *config.ConnectConfig
	identity *Identity
	dispatch *Dispatcher
	runs     *runManager

	startedAt time.Time
	log       *logger.Logger

	cur atomic.Pointer[connState]

	mu       sync.Mutex
	scopes   []string
	sessID   string
	hbSecs   int
	stopOnce sync.Once
	stopped  chan struct{}

	runCtx    context.Context
	runCancel context.CancelFunc
}

type connState struct {
	conn   *websocket.Conn
	out    chan *Frame
	done   chan struct{}
	cancel context.CancelFunc
}

func New(deps Deps, cfg *config.ConnectConfig) (*Connector, error) {
	if cfg == nil {
		cfg = config.DefaultConfig().Connect
	}
	identity, err := LoadOrCreateIdentity(cfg.DeviceName)
	if err != nil {
		return nil, err
	}
	if deps.Version == "" {
		deps.Version = config.AppVersion
	}
	c := &Connector{
		deps:      deps,
		cfg:       cfg,
		identity:  identity,
		log:       deps.Log,
		dispatch:  newDispatcher(),
		startedAt: time.Now(),
		stopped:   make(chan struct{}),
	}
	c.runCtx, c.runCancel = context.WithCancel(context.Background())
	c.runs = newRunManager(c)
	c.registerAllMethods()
	if err := c.subscribeBus(deps.Bus); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Connector) Identity() *Identity { return c.identity }

func (c *Connector) RelayURL() string { return c.cfg.RelayURL }

func (c *Connector) Endpoint() string {
	endpoint, err := Endpoint(c.cfg.RelayURL, c.cfg.Token)
	if err != nil {
		return c.cfg.RelayURL
	}
	return endpoint
}

func (c *Connector) MethodNames() []string { return c.dispatch.Names() }

func (c *Connector) Connected() bool { return c.cur.Load() != nil }

func (c *Connector) Stop() {
	c.stopOnce.Do(func() {
		close(c.stopped)
		if c.runCancel != nil {
			c.runCancel()
		}
		c.runs.cancelAll()
		if cs := c.cur.Swap(nil); cs != nil {
			if cs.cancel != nil {
				cs.cancel()
			}
			if cs.conn != nil {
				_ = cs.conn.Close(websocket.StatusNormalClosure, "daemon shutting down")
			}
		}
	})
}

func (c *Connector) Run(ctx context.Context) error {
	if c.cfg.RelayURL == "" {
		return errors.New("connect: relay_url is not configured (set connect.relay_url or PEGG_CONNECT_RELAY)")
	}
	if err := c.validateRelayURL(); err != nil {
		return err
	}
	if _, err := Endpoint(c.cfg.RelayURL, c.cfg.Token); err != nil {
		return err
	}

	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if isStopped(c.stopped) {
			return nil
		}

		err := c.runOnce(ctx)
		if ctx.Err() != nil || isStopped(c.stopped) {
			return nil
		}
		c.log.Fwarn("connect: relay connection ended: %v (retrying in %s)", err, backoff)

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return ctx.Err()
		case <-c.stopped:
			return nil
		}
		backoff *= 2
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
}

func (c *Connector) validateRelayURL() error {
	url := c.cfg.RelayURL
	if strings.HasPrefix(url, "wss://") {
		return nil
	}
	if strings.HasPrefix(url, "ws://") {
		rest := strings.TrimPrefix(url, "ws://")
		host := rest
		if i := strings.IndexAny(rest, "/?#"); i != -1 {
			host = rest[:i]
		}
		if c.cfg.Insecure || isLoopbackHost(host) {
			return nil
		}
		return fmt.Errorf("connect: refusing insecure ws:// to non-loopback host %q (use wss:// or set insecure=true)", host)
	}
	return fmt.Errorf("connect: unsupported relay_url scheme in %q (want ws:// or wss://)", url)
}

func isLoopbackHost(host string) bool {
	h := host
	if i := strings.LastIndex(h, ":"); i != -1 {
		h = h[:i]
	}
	h = strings.Trim(h, "[]")
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

func (c *Connector) runOnce(ctx context.Context) error {
	dialCtx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	conn, _, err := websocket.Dial(dialCtx, c.Endpoint(), &websocket.DialOptions{})
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	conn.SetReadLimit(maxFrameBytesDefault)

	if err := c.handshake(dialCtx, conn); err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "handshake failed")
		return err
	}

	Audit("connected", map[string]any{
		"relay":     c.cfg.RelayURL,
		"device_id": c.identity.DeviceID,
	})
	c.log.Finfo("connect: connected to relay %s", c.cfg.RelayURL)
	return c.serve(ctx, conn)
}
