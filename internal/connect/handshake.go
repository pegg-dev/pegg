package connect

import (
	"context"
	"fmt"

	"github.com/coder/websocket"
)

func (c *Connector) handshake(ctx context.Context, conn *websocket.Conn) error {
	nonce := randomHex(16)
	ts := now()
	hello := HelloPayload{
		ProtocolVersion: ProtocolVersion,
		DeviceID:        c.identity.DeviceID,
		DeviceName:      c.identity.Name,
		PublicKey:       c.identity.PublicKey,
		PeggVersion:     c.deps.Version,
		Workspace:       c.workspace(),
		Capabilities:    c.capabilities(),
		Nonce:           nonce,
		TS:              ts,
	}
	hello.Signature = c.identity.Sign(helloMessage(c.identity.DeviceID, nonce, ts))

	payload, err := marshal(hello)
	if err != nil {
		return err
	}
	if err := writeFrame(ctx, conn, &Frame{Type: frameHello, Params: payload}); err != nil {
		return fmt.Errorf("send hello: %w", err)
	}

	f, err := readFrame(ctx, conn)
	if err != nil {
		return fmt.Errorf("read welcome: %w", err)
	}
	if f.Type == frameError {
		return fmt.Errorf("relay rejected hello: %s", f.Error)
	}
	if f.Type != frameWelcome {
		return fmt.Errorf("unexpected handshake frame %q", f.Type)
	}

	var welcome WelcomePayload
	if len(f.Result) > 0 {
		_ = jsonUnmarshal(f.Result, &welcome)
	} else if len(f.Params) > 0 {
		_ = jsonUnmarshal(f.Params, &welcome)
	}
	if welcome.ProtocolVersion != 0 && welcome.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("relay protocol version %d, daemon supports %d", welcome.ProtocolVersion, ProtocolVersion)
	}

	c.mu.Lock()
	c.scopes = normalizeScopes(welcome.GrantedScopes)
	c.sessID = welcome.SessionID
	c.hbSecs = welcome.HeartbeatSecs
	if c.hbSecs <= 0 {
		c.hbSecs = defaultHeartbeatSecs
	}
	c.mu.Unlock()

	if len(welcome.GrantedScopes) == 0 {
		c.log.Fwarn("connect: relay granted no explicit scopes; defaulting to full scope set")
	}
	if welcome.MaxFrameBytes > 0 {
		conn.SetReadLimit(int64(welcome.MaxFrameBytes))
	}
	return nil
}
