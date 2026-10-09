package connect

import (
	"context"
	"time"

	"github.com/coder/websocket"
)

func (c *Connector) serve(ctx context.Context, conn *websocket.Conn) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cs := &connState{
		conn:   conn,
		out:    make(chan *Frame, outQueueSize),
		done:   make(chan struct{}),
		cancel: cancel,
	}
	c.cur.Store(cs)
	defer func() {
		c.cur.CompareAndSwap(cs, nil)
		c.mu.Lock()
		c.scopes = nil
		c.sessID = ""
		c.mu.Unlock()
		close(cs.done)
		Audit("disconnected", map[string]any{"relay": c.cfg.RelayURL})
	}()

	go c.writerLoop(ctx, cs)
	go c.heartbeatLoop(ctx, cs)
	c.emitStatus()

	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		if typ != websocket.MessageText {
			continue
		}
		var f Frame
		if err := jsonUnmarshal(data, &f); err != nil {
			continue
		}
		switch f.Type {
		case frameRequest:
			go c.handleRequest(ctx, &f)
		case framePing:
			c.send(&Frame{Type: framePong})
		case framePong, frameResp, frameEvent:
		}
	}
}

func (c *Connector) writerLoop(ctx context.Context, cs *connState) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-cs.done:
			return
		case f := <-cs.out:
			wctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			err := writeFrame(wctx, cs.conn, f)
			cancel()
			if err != nil {
				c.log.Fwarn("connect: write failed: %v", err)
				cs.cancel()
				return
			}
		}
	}
}

func (c *Connector) heartbeatLoop(ctx context.Context, cs *connState) {
	c.mu.Lock()
	secs := c.hbSecs
	c.mu.Unlock()
	if secs <= 0 {
		secs = defaultHeartbeatSecs
	}
	ticker := time.NewTicker(time.Duration(secs) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-cs.done:
			return
		case <-ticker.C:
			pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := cs.conn.Ping(pctx)
			cancel()
			if err != nil {
				c.log.Fwarn("connect: heartbeat failed: %v", err)
				cs.cancel()
				return
			}
			c.send(&Frame{Type: frameHeartbeat, Event: eventHeartbeat})
		}
	}
}

func (c *Connector) send(f *Frame) {
	cs := c.cur.Load()
	if cs == nil {
		return
	}
	if f.V == 0 {
		f.V = ProtocolVersion
	}
	if f.TS == 0 {
		f.TS = now()
	}
	select {
	case cs.out <- f:
	case <-cs.done:
	case <-time.After(3 * time.Second):
		c.log.Fwarn("connect: event queue stalled, dropping connection")
		cs.cancel()
	}
}

func (c *Connector) emit(name, sessionID, runID string, data any) {
	payload, err := marshal(data)
	if err != nil {
		return
	}
	c.send(&Frame{
		Type:      frameEvent,
		Event:     name,
		SessionID: sessionID,
		RunID:     runID,
		Data:      payload,
	})
}

func (c *Connector) emitStatus() {
	c.emit(eventStatus, "", "", map[string]any{
		"connected":   true,
		"device_id":   c.identity.DeviceID,
		"device_name": c.identity.Name,
		"workspace":   c.workspace(),
		"version":     c.deps.Version,
		"uptime_secs": int(time.Since(c.startedAt).Seconds()),
	})
}
