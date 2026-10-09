package connect

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	json "github.com/goccy/go-json"
)

func marshal(v any) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func writeFrame(ctx context.Context, conn *websocket.Conn, f *Frame) error {
	if f.V == 0 {
		f.V = ProtocolVersion
	}
	if f.TS == 0 {
		f.TS = now()
	}
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, data)
}

func readFrame(ctx context.Context, conn *websocket.Conn) (*Frame, error) {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	var f Frame
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

func now() int64 { return time.Now().Unix() }

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return uuid.NewString()
	}
	return hex.EncodeToString(b)
}

func isStopped(ch chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}
