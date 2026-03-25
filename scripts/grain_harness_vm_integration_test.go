package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type vmHarnessTestEnv struct {
	ctx    context.Context
	cancel context.CancelFunc
	runSSH func(string) (string, error)
}

func newVMHarnessTestEnv(t *testing.T, timeout time.Duration) vmHarnessTestEnv {
	if os.Getenv("SANDSTORM_VM_INTEGRATION") == "" {
		t.Skip("set SANDSTORM_VM_INTEGRATION=1 to run VM-backed app-harness integration tests")
	}

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path: runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
	spktool := os.Getenv("SPKTOOL")
	if spktool == "" {
		spktool = "spktool"
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	runSSH := func(script string) (string, error) {
		t.Helper()
		cmd := exec.CommandContext(ctx, spktool, "--work-directory", repoRoot, "vm", "ssh", "bash", "-lc", script)
		cmd.Dir = repoRoot
		out, err := cmd.CombinedOutput()
		if err != nil {
			return string(out), fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
		}
		return string(out), nil
	}

	t.Cleanup(cancel)
	return vmHarnessTestEnv{ctx: ctx, cancel: cancel, runSSH: runSSH}
}

func TestGrainHarnessServePkgDefInVM(t *testing.T) {
	t.Log("starting VM-backed app-harness smoke test")
	env := newVMHarnessTestEnv(t, 3*time.Minute)

	t.Cleanup(func() {
		_, _ = env.runSSH(`pkill -f "/opt/app/testapp/bin/app-harness serve --root /tmp/gh-serve-test" || true`)
	})

	t.Log("starting app-harness serve inside the VM")
	if out, err := env.runSSH(`bash /opt/app/scripts/grain-harness-vm-smoke.sh`); err != nil {
		t.Fatalf("start serve in vm: %v", err)
	} else if strings.TrimSpace(out) != "" {
		t.Log(out)
	}

	var body string
	deadline := time.Now().Add(45 * time.Second)
	t.Log("polling the served app until it responds")
	for time.Now().Before(deadline) {
		out, err := env.runSSH(`curl -fsS --max-time 10 http://127.0.0.1:3011/`)
		if err == nil {
			body = out
			break
		}
		time.Sleep(2 * time.Second)
	}

	if body == "" {
		logs, _ := env.runSSH(`
			echo ==serve-log==
			cat /tmp/gh-serve-test.log 2>/dev/null || true
			echo ==grain==
			cat /tmp/gh-serve-test/grains/serve/grain.json 2>/dev/null || true
			echo ==supervisor==
			cat /tmp/gh-serve-test/grains/serve/logs/supervisor.log 2>/dev/null || true
			echo ==monitor==
			cat /tmp/gh-serve-test/grains/serve/logs/monitor.log 2>/dev/null || true
			echo ==events==
			cat /tmp/gh-serve-test/grains/serve/events.jsonl 2>/dev/null || true
		`)
		t.Fatalf("serve endpoint never became ready\n%s", logs)
	}

	if !strings.Contains(body, "sandstorm-utils testapp") {
		t.Fatalf("unexpected response body:\n%s", body)
	}
	t.Log("issuing POST request through app-harness serve")
	postOut, err := env.runSSH(`curl -i -sS --max-time 10 -X POST http://127.0.0.1:3011/api/run-all`)
	if err != nil {
		t.Fatalf("POST request through serve failed: %v", err)
	}
	if !strings.Contains(postOut, "200 OK") {
		t.Fatalf("unexpected POST status:\n%s", postOut)
	}
	if !strings.Contains(postOut, `"results":[`) || !strings.Contains(postOut, `"name":"get-public-id"`) {
		t.Fatalf("unexpected POST body:\n%s", postOut)
	}
	t.Log("received expected testapp response through app-harness serve")
}

func TestGrainHarnessServeBadPkgDefInVM(t *testing.T) {
	t.Log("starting VM-backed bad-pkgdef test")
	env := newVMHarnessTestEnv(t, 2*time.Minute)

	out, err := env.runSSH(`
		/opt/app/testapp/bin/app-harness serve \
		  --root /tmp/gh-bad-pkgdef \
		  --pkg-def /opt/app/.sandstorm/sandstorm-pkgdef.capnp \
		  --port 3012
	`)
	if err == nil {
		t.Fatalf("app-harness serve unexpectedly succeeded with malformed pkg-def output=%s", out)
	}
	combined := strings.TrimSpace(out + "\n" + err.Error())
	if !strings.Contains(combined, "pkgDef must be in path:symbol form") {
		t.Fatalf("unexpected error for malformed pkg-def:\n%s", combined)
	}
	t.Log("received expected pkg-def validation error")
}
