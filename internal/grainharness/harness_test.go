package grainharness

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateAndReadEvents(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig(root, "grain-test")
	cfg.SupervisorCommand = "/bin/true"

	manager := NewManager()
	state, err := manager.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if state.Status != StatusCreated {
		t.Fatalf("status = %q, want %q", state.Status, StatusCreated)
	}

	events, err := ReadEvents(root, "grain-test")
	if err != nil {
		t.Fatalf("ReadEvents() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if events[0].Kind != "grain.created" {
		t.Fatalf("event kind = %q, want grain.created", events[0].Kind)
	}

	if _, err := manager.Load(root, "grain-test"); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestAppendEventAndTailLog(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig(root, "grain-test")
	cfg.SupervisorCommand = "/bin/true"

	manager := NewManager()
	if _, err := manager.Create(cfg); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := AppendEvent(cfg, "custom.event", map[string]any{"ok": true}); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}

	logPath := filepath.Join(root, "grains", "grain-test", "logs", "supervisor.log")
	if err := os.WriteFile(logPath, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	data, err := TailLog(root, "grain-test", "supervisor")
	if err != nil {
		t.Fatalf("TailLog() error = %v", err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("TailLog() = %q, want %q", string(data), "hello\n")
	}
}

func TestParseEnv(t *testing.T) {
	env, err := ParseEnv([]string{"A=1", "B=2"})
	if err != nil {
		t.Fatalf("ParseEnv() error = %v", err)
	}
	if env["A"] != "1" || env["B"] != "2" {
		t.Fatalf("ParseEnv() = %#v", env)
	}
}

func TestParseNameValues(t *testing.T) {
	values, err := ParseNameValues([]string{"X-Test=1", "session=abc"})
	if err != nil {
		t.Fatalf("ParseNameValues() error = %v", err)
	}
	if values["X-Test"] != "1" || values["session"] != "abc" {
		t.Fatalf("ParseNameValues() = %#v", values)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig("/tmp/root", "grain-x")
	if cfg.KeepAliveInterval != 30*time.Second {
		t.Fatalf("KeepAliveInterval = %v", cfg.KeepAliveInterval)
	}
	if got, want := cfg.SupervisorSocket, "/tmp/root/grains/grain-x/var/socket"; got != want {
		t.Fatalf("SupervisorSocket = %q, want %q", got, want)
	}
}

func TestLaunchArgsLegacySupervisor(t *testing.T) {
	cfg := DefaultConfig("/tmp/root", "grain-x")
	cfg.SupervisorArgs = []string{"--socket", "/tmp/root/grains/grain-x/fixture.sock"}

	args, err := cfg.launchArgs(&State{Config: cfg})
	if err != nil {
		t.Fatalf("launchArgs() error = %v", err)
	}
	if got, want := len(args), 2; got != want {
		t.Fatalf("len(args) = %d, want %d", got, want)
	}
}

func TestLaunchArgsManagedSupervisor(t *testing.T) {
	cfg := DefaultConfig("/tmp/root", "grain-x")
	cfg.SupervisorCommand = "/tmp/sandstorm-supervisor"
	cfg.PackagePath = "/tmp/pkg"
	cfg.AppName = "sandstorm-utils-testapp"
	cfg.AppCommand = []string{"/sandstorm-http-bridge", "3000", "--", "/opt/app/testapp/bin/testapp-server"}

	args, err := cfg.launchArgs(&State{Config: cfg})
	if err != nil {
		t.Fatalf("launchArgs() error = %v", err)
	}

	want := []string{
		"--pkg", "/tmp/pkg",
		"--var", "/tmp/root/grains/grain-x/var",
		"--new",
		"sandstorm-utils-testapp", "grain-x",
		"/sandstorm-http-bridge", "3000", "--", "/opt/app/testapp/bin/testapp-server",
	}
	if got, wantLen := len(args), len(want); got != wantLen {
		t.Fatalf("len(args) = %d, want %d: %#v", got, wantLen, args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q; full=%#v", i, args[i], want[i], args)
		}
	}

	started := time.Now().UTC()
	args, err = cfg.launchArgs(&State{Config: cfg, StartedAt: &started})
	if err != nil {
		t.Fatalf("launchArgs() second start error = %v", err)
	}
	for _, arg := range args {
		if arg == "--new" {
			t.Fatalf("launchArgs() included --new for an already-started grain: %#v", args)
		}
	}
}

func TestCreateManagedSupervisorLeavesVarDirForSupervisor(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig(root, "grain-test")
	cfg.SupervisorCommand = "/tmp/sandstorm-supervisor"
	cfg.PackagePath = "/tmp/pkg"
	cfg.AppName = "sandstorm-utils-testapp"
	cfg.AppCommand = []string{"/bin/true"}

	if _, err := NewManager().Create(cfg); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "grains", "grain-test", "var")); !os.IsNotExist(err) {
		t.Fatalf("managed supervisor mode pre-created var dir; stat err = %v", err)
	}
}

func TestWriteHTTPResponseText(t *testing.T) {
	rec := httptest.NewRecorder()
	writeHTTPResponse(rec, &ResponsePayload{
		StatusCode: 201,
		MimeType:   "text/plain; charset=utf-8",
		Encoding:   "identity",
		Language:   "en-US",
		BodyText:   "hello",
	})

	res := rec.Result()
	if got, want := res.StatusCode, 201; got != want {
		t.Fatalf("StatusCode = %d, want %d", got, want)
	}
	if got, want := res.Header.Get("Content-Type"), "text/plain; charset=utf-8"; got != want {
		t.Fatalf("Content-Type = %q, want %q", got, want)
	}
	if got, want := res.Header.Get("Content-Encoding"), "identity"; got != want {
		t.Fatalf("Content-Encoding = %q, want %q", got, want)
	}
	if got, want := res.Header.Get("Content-Language"), "en-US"; got != want {
		t.Fatalf("Content-Language = %q, want %q", got, want)
	}
	if got, want := rec.Body.String(), "hello"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestWriteHTTPResponseBinaryAndRedirect(t *testing.T) {
	rec := httptest.NewRecorder()
	writeHTTPResponse(rec, &ResponsePayload{
		StatusCode: 307,
		MimeType:   "application/octet-stream",
		Location:   "/next",
		BodyBase64: base64.StdEncoding.EncodeToString([]byte{0x00, 0x01, 0x02}),
	})

	res := rec.Result()
	if got, want := res.StatusCode, http.StatusTemporaryRedirect; got != want {
		t.Fatalf("StatusCode = %d, want %d", got, want)
	}
	if got, want := res.Header.Get("Location"), "/next"; got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
	if got, want := rec.Body.String(), string([]byte{0x00, 0x01, 0x02}); got != want {
		t.Fatalf("body bytes = %v, want %v", []byte(rec.Body.String()), []byte{0x00, 0x01, 0x02})
	}
}

func TestWriteHTTPResponseDefaultsStatusOK(t *testing.T) {
	rec := httptest.NewRecorder()
	writeHTTPResponse(rec, &ResponsePayload{BodyText: "ok"})
	if got, want := rec.Result().StatusCode, http.StatusOK; got != want {
		t.Fatalf("StatusCode = %d, want %d", got, want)
	}
}

func TestRewritePkgDefID(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "pkg.capnp")
	dst := filepath.Join(root, "pkg.rewritten.capnp")
	input := `const pkgdef :Spk.PackageDefinition = (
  id = "old-app-id",
  manifest = ( appTitle = (defaultText = "Demo") )
);`
	if err := os.WriteFile(src, []byte(input), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := rewritePkgDefID(src, dst, "new-app-id"); err != nil {
		t.Fatalf("rewritePkgDefID() error = %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `id = "new-app-id",`) {
		t.Fatalf("rewritten pkgdef missing new id: %s", text)
	}
	if strings.Contains(text, `id = "old-app-id",`) {
		t.Fatalf("rewritten pkgdef still contains old id: %s", text)
	}
}
