package tui

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
)

var keypadDigits = map[int]byte{
	57417: '4', // KP_LEFT
	57418: '6', // KP_RIGHT
	57419: '8', // KP_UP
	57420: '2', // KP_DOWN
	57421: '9', // KP_PAGE_UP
	57422: '3', // KP_PAGE_DOWN
	57423: '7', // KP_HOME
	57424: '1', // KP_END
	57425: '0', // KP_INSERT
	57426: '.', // KP_DELETE
	57427: '5', // KP_BEGIN
}

const numLockMod = 0x80

var winNavVK = map[int]bool{
	0x0c: true, // VK_CLEAR
	0x21: true, // VK_PRIOR (Page Up)
	0x22: true, // VK_NEXT (Page Down)
	0x23: true, // VK_END
	0x24: true, // VK_HOME
	0x25: true, // VK_LEFT
	0x26: true, // VK_UP
	0x27: true, // VK_RIGHT
	0x28: true, // VK_DOWN
	0x2d: true, // VK_INSERT
	0x2e: true, // VK_DELETE
}

type keypadTty struct {
	tcell.Tty
	logPath string
	mu      sync.Mutex
}

func newKeypadTty(logPath string) tcell.Tty {
	dt, err := tcell.NewDevTty()
	if err != nil {
		return nil
	}
	return &keypadTty{Tty: dt, logPath: logPath}
}

func (t *keypadTty) Read(p []byte) (int, error) {
	n, err := t.Tty.Read(p)
	if n <= 0 {
		return n, err
	}
	raw := p[:n]
	if t.logPath != "" {
		t.mu.Lock()
		if f, e := os.OpenFile(t.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); e == nil {
			fmt.Fprintf(f, "%q\n", string(raw))
			_ = f.Close()
		}
		t.mu.Unlock()
	}
	if out := rewriteKeypad(raw); len(out) != n {
		copy(p, out)
		return len(out), err
	}
	return n, err
}

func rewriteKeypad(data []byte) []byte {
	if !bytes.Contains(data, []byte{0x1b, '['}) {
		return data
	}
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); {
		if data[i] == 0x1b && i+1 < len(data) && data[i+1] == '[' {
			j := i + 2
			for j < len(data) {
				c := data[j]
				if c >= 0x40 && c <= 0x7e {
					break
				}
				if !((c >= 0x30 && c <= 0x3f) || (c >= 0x20 && c <= 0x2f)) {
					break
				}
				j++
			}
			if j < len(data) {
				switch data[j] {
				case 'u':
					if digit, ok := keypadDigit(string(data[i+2 : j])); ok {
						out = append(out, digit)
						i = j + 1
						continue
					}
				case '_':
					if digit, ok := win32KeypadDigit(string(data[i+2 : j])); ok {
						out = append(out, digit)
						i = j + 1
						continue
					}
				}
			}
		}
		out = append(out, data[i])
		i++
	}
	return out
}

func keypadDigit(params string) (byte, bool) {
	fields := strings.Split(params, ";")
	codeStr := fields[0]
	if idx := strings.IndexByte(codeStr, ':'); idx >= 0 {
		codeStr = codeStr[:idx]
	}
	code, err := strconv.Atoi(codeStr)
	if err != nil {
		return 0, false
	}
	digit, ok := keypadDigits[code]
	if !ok {
		return 0, false
	}
	mod := 1
	if len(fields) > 1 {
		modStr := fields[1]
		if idx := strings.IndexByte(modStr, ':'); idx >= 0 {
			modStr = modStr[:idx]
		}
		if m, err := strconv.Atoi(modStr); err == nil {
			mod = m
		}
	}
	if mod < 1 || (mod-1)&numLockMod == 0 {
		return 0, false
	}
	return digit, true
}

func win32KeypadDigit(params string) (byte, bool) {
	fields := strings.Split(params, ";")
	if len(fields) < 4 {
		return 0, false
	}
	vk, err1 := strconv.Atoi(fields[0])
	uc, err2 := strconv.Atoi(fields[2])
	kd, err3 := strconv.Atoi(fields[3])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, false
	}
	if kd != 1 || !winNavVK[vk] {
		return 0, false
	}
	if (uc >= '0' && uc <= '9') || uc == '.' {
		return byte(uc), true
	}
	return 0, false
}
