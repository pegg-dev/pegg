package connect

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
)

var confirmMu sync.Mutex

func (c *Connector) requireConfirm(action, detail string) error {
	if c.cfg.AutoApprove {
		return nil
	}
	if !stdinIsTTY() {
		Audit("confirm.denied", map[string]any{"action": action, "detail": detail, "reason": "non-interactive"})
		return &FrameError{
			Code:    ErrCodeForbidden,
			Message: "action '" + action + "' requires local confirmation (start connect with --auto-approve to disable)",
		}
	}

	confirmMu.Lock()
	defer confirmMu.Unlock()

	fmt.Fprintf(os.Stderr, "\npegg connect: a remote client requests to %s (%s).\nApprove? [y/N]: ", action, detail)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	ok := line == "y" || line == "yes"
	Audit("confirm", map[string]any{"action": action, "detail": detail, "approved": ok})
	if !ok {
		return &FrameError{Code: ErrCodeForbidden, Message: "action denied locally"}
	}
	return nil
}

func stdinIsTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
