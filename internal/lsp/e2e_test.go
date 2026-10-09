package lsp

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/lsp/diagnostic"
	"github.com/peggco/pegg/internal/vfs"
)

func TestRealSubprocessEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}

	script := filepath.Join(t.TempDir(), "fakelsp.py")
	src := `import sys, json

def read_frame():
    headers = {}
    while True:
        line = sys.stdin.buffer.readline()
        if line in (b"\r\n", b"\n", b""):
            break
        if not line:
            return None
        k, _, v = line.decode().partition(":")
        headers[k.strip().lower()] = v.strip()
    n = int(headers.get("content-length", 0) or 0)
    if n <= 0:
        return None
    return json.loads(sys.stdin.buffer.read(n).decode())

def send(obj):
    data = json.dumps(obj).encode()
    sys.stdout.buffer.write(("Content-Length: %d\r\n\r\n" % len(data)).encode() + data)
    sys.stdout.buffer.flush()

while True:
    msg = read_frame()
    if msg is None:
        break
    if "id" in msg and msg.get("method") == "initialize":
        send({"jsonrpc":"2.0","id":msg["id"],"result":{"capabilities":{},"serverInfo":{"name":"fakepy","version":"1.0"}}})
    elif msg.get("method") == "textDocument/didOpen":
        uri = msg["params"]["textDocument"]["uri"]
        send({"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":uri,"diagnostics":[{"severity":1,"message":"fake python error","range":{"start":{"line":0,"character":0},"end":{"line":0,"character":1}}}]}})
    elif "id" in msg and msg.get("method") == "shutdown":
        send({"jsonrpc":"2.0","id":msg["id"],"result":{}})
    elif msg.get("method") == "exit":
        break
`
	if err := os.WriteFile(script, []byte(src), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(script)+string(filepath.ListSeparator)+os.Getenv("PATH"))

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.py"), []byte("print('hi')\n"), 0644); err != nil {
		t.Fatal(err)
	}
	fs, err := vfs.New(root, vfs.Options{})
	if err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(fs, nil)
	mgr.Start(map[string]config.LanguageServerConfig{
		"fakepy": {Command: "python3", Args: []string{script}, FileTypes: []string{"py"}},
	})
	defer mgr.Close()

	if _, err := fs.Read("main.py"); err != nil {
		t.Fatal(err)
	}

	var diags []diagnostic.Diagnostic
	deadline := time.After(10 * time.Second)
	for {
		diags = mgr.Diagnostics("main.py")
		if len(diags) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("no diagnostics; running=%v", mgr.Running())
		case <-time.After(100 * time.Millisecond):
		}
	}
	if len(diags) != 1 || diags[0].Message != "fake python error" {
		t.Fatalf("diags = %+v", diags)
	}

	res, err := fs.Read("main.py")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res, "fake python error") {
		t.Fatalf("read output must contain diagnostic, got %q", res)
	}
}
