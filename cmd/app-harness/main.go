package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mnutt/sandstorm-utils/internal/grainharness"
)

const commandName = "app-harness"
const commandPurpose = "Manage standalone supervisor-backed app harness workdirs for repeatable tests and debugging."

var commandExamples = []string{
	"Create a harness workdir for an app grain and record how the supervisor should be launched.\nCommand: app-harness create --root .app-harness --grain demo --supervisor /path/to/sandstorm-supervisor --pkg /tmp/testpkg --app-name demo -- /sandstorm-http-bridge 3000 -- /opt/app/testapp/bin/testapp-server\nArguments: trailing arguments after `--` become the app command executed under the Sandstorm supervisor.\nReturns: JSON describing the created grain state.\nUse this before starting a new acceptance-test or debugging grain instance.",
	"Start the configured supervisor plus the keepalive monitor, then inspect its state and logs.\nCommand: app-harness start --root .app-harness --grain demo\nArguments: none.\nReturns: JSON with the running supervisor and monitor PIDs.\nUse this to boot the harness-managed grain and begin collecting structured SandstormCore events.",
	"Open a reusable session spec and send a request through the grain's WebSession interface.\nCommand: app-harness session-open --root .app-harness --grain demo --session web\nArguments: none.\nReturns: JSON describing the stored session spec.\nUse this with `app-harness request --session web --method GET --path /` to drive acceptance tests or automated debugging.",
	"Expose a running app grain on a local HTTP port by forwarding requests through WebSession.\nCommand: app-harness serve --pkg-def /opt/app/.sandstorm/sandstorm-pkgdef.capnp:pkgdef --port 3000\nArguments: none.\nReturns: no JSON; runs until interrupted.\nUse this for host or VM-driven debug loops against sandstorm-http-bridge apps.",
}

const synopsis = `<command> [flags]

Commands:
  create   create a managed grain workdir
  start    start the configured supervisor and keepalive monitor
  session-open  create a reusable WebSession spec
  request  open a WebSession and issue a request
  serve    expose a running grain on a local HTTP port
  status   print the current grain state as JSON
  stop     stop the monitor and supervisor
  logs     print a log file
  dump     print state plus file locations as JSON`

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: app-harness %s", synopsis)
	}

	if args[0] == "_keepalive" {
		return runKeepAlive(ctx, args[1:])
	}

	switch args[0] {
	case "create":
		return runCreate(args[1:])
	case "start":
		return runStart(ctx, args[1:])
	case "session-open":
		return runSessionOpen(args[1:])
	case "request":
		return runRequest(ctx, args[1:])
	case "serve":
		return runServe(ctx, args[1:])
	case "status":
		return runStatus(args[1:])
	case "stop":
		return runStop(args[1:])
	case "logs":
		return runLogs(args[1:])
	case "dump":
		return runDump(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runCreate(args []string) error {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	root := fs.String("root", ".app-harness", "harness root directory")
	grainID := fs.String("grain", "", "grain ID")
	mode := fs.String("mode", "test", "mode: test or debug")
	socket := fs.String("socket", "", "unix socket path for the supervisor")
	supervisorCmd := fs.String("supervisor", "", "path to sandstorm-supervisor or wrapper command")
	packagePath := fs.String("pkg", "", "package directory passed to sandstorm-supervisor --pkg")
	appName := fs.String("app-name", "", "app name/id passed to sandstorm-supervisor")
	bootMainView := fs.Bool("boot-main-view", true, "call Supervisor.getMainView after monitor connect")
	connectTimeout := fs.Duration("connect-timeout", 20*time.Second, "wait for supervisor socket during monitor startup")
	keepAlive := fs.Duration("keepalive-interval", 30*time.Second, "Supervisor.keepAlive interval")
	var envVars multiFlag
	fs.Var(&envVars, "env", "extra supervisor env KEY=VALUE")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *grainID == "" {
		return errors.New("create requires --grain")
	}

	cfg := grainharness.DefaultConfig(*root, *grainID)
	cfg.Mode = *mode
	cfg.SupervisorCommand = *supervisorCmd
	cfg.PackagePath = *packagePath
	cfg.AppName = *appName
	cfg.BootMainView = *bootMainView
	cfg.ConnectTimeout = *connectTimeout
	cfg.KeepAliveInterval = *keepAlive
	if *socket != "" {
		cfg.SupervisorSocket = *socket
	}
	if cfg.PackagePath != "" || cfg.AppName != "" {
		cfg.AppCommand = fs.Args()
	} else {
		cfg.SupervisorArgs = fs.Args()
	}

	parsedEnv, err := grainharness.ParseEnv(envVars)
	if err != nil {
		return err
	}
	cfg.SupervisorEnv = parsedEnv

	state, err := grainharness.NewManager().Create(cfg)
	if err != nil {
		return err
	}
	return writeJSON(state)
}

func runStart(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	root := fs.String("root", ".app-harness", "harness root directory")
	grainID := fs.String("grain", "", "grain ID")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *grainID == "" {
		return errors.New("start requires --grain")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	state, err := grainharness.NewManager().Start(ctx, *root, *grainID, exe)
	if err != nil {
		return err
	}
	return writeJSON(state)
}

func runStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	root := fs.String("root", ".app-harness", "harness root directory")
	grainID := fs.String("grain", "", "grain ID")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *grainID == "" {
		return errors.New("status requires --grain")
	}
	state, err := grainharness.NewManager().Refresh(*root, *grainID)
	if err != nil {
		return err
	}
	return writeJSON(state)
}

func runSessionOpen(args []string) error {
	fs := flag.NewFlagSet("session-open", flag.ContinueOnError)
	root := fs.String("root", ".app-harness", "harness root directory")
	grainID := fs.String("grain", "", "grain ID")
	sessionID := fs.String("session", "", "session ID")
	basePath := fs.String("base-path", "http://grain.local", "base URL exposed to the app")
	userAgent := fs.String("user-agent", "app-harness/0", "session user agent")
	languages := fs.String("languages", "en-US", "comma-separated acceptable languages")
	tabID := fs.String("tab-id", "", "tab ID to expose to the app")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *grainID == "" {
		return errors.New("session-open requires --grain")
	}
	spec := grainharness.SessionSpec{
		ID:                  *sessionID,
		BasePath:            *basePath,
		UserAgent:           *userAgent,
		AcceptableLanguages: splitCSV(*languages),
		TabID:               *tabID,
	}
	session, err := grainharness.NewManager().OpenSession(*root, *grainID, spec)
	if err != nil {
		return err
	}
	return writeJSON(session)
}

func runRequest(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("request", flag.ContinueOnError)
	root := fs.String("root", ".app-harness", "harness root directory")
	grainID := fs.String("grain", "", "grain ID")
	sessionID := fs.String("session", "", "session ID")
	method := fs.String("method", "GET", "HTTP-like method to issue")
	path := fs.String("path", "/", "grain-relative path")
	body := fs.String("body", "", "request body")
	mimeType := fs.String("mime-type", "text/plain; charset=utf-8", "request MIME type")
	encoding := fs.String("encoding", "", "optional content encoding")
	var headers multiFlag
	var cookies multiFlag
	fs.Var(&headers, "header", "extra request header KEY=VALUE")
	fs.Var(&cookies, "cookie", "request cookie NAME=VALUE")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *grainID == "" {
		return errors.New("request requires --grain")
	}
	if *sessionID == "" {
		return errors.New("request requires --session")
	}
	parsedHeaders, err := grainharness.ParseNameValues(headers)
	if err != nil {
		return err
	}
	parsedCookies, err := grainharness.ParseNameValues(cookies)
	if err != nil {
		return err
	}
	response, err := grainharness.NewManager().DoRequest(ctx, *root, *grainID, grainharness.RequestSpec{
		SessionID: *sessionID,
		Method:    *method,
		Path:      *path,
		Body:      []byte(*body),
		MimeType:  *mimeType,
		Encoding:  *encoding,
		Headers:   parsedHeaders,
		Cookies:   parsedCookies,
	})
	if err != nil {
		return err
	}
	return writeJSON(response)
}

func runStop(args []string) error {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	root := fs.String("root", ".app-harness", "harness root directory")
	grainID := fs.String("grain", "", "grain ID")
	force := fs.Bool("force", false, "use SIGKILL instead of SIGTERM")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *grainID == "" {
		return errors.New("stop requires --grain")
	}
	state, err := grainharness.NewManager().Stop(*root, *grainID, *force)
	if err != nil {
		return err
	}
	return writeJSON(state)
}

func runServe(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	root := fs.String("root", ".app-harness", "harness root directory")
	grainID := fs.String("grain", "", "grain ID")
	pkgDef := fs.String("pkg-def", "", "package definition in path:symbol form")
	port := fs.String("port", "3000", "listen port")
	addr := fs.String("addr", "127.0.0.1:3000", "listen address")
	sessionID := fs.String("session", "serve", "session ID to reuse for proxied requests")
	baseURL := fs.String("base-url", "", "base URL exposed to the app; defaults to http://<addr>")
	userAgent := fs.String("user-agent", "app-harness-serve/0", "session user agent")
	mocks := fs.String("mocks", "", "optional JSON file describing mocked Sandstorm API responses")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *grainID == "" && *pkgDef == "" {
		return errors.New("serve requires --grain or --pkg-def")
	}
	if *addr == "127.0.0.1:3000" && *port != "3000" {
		*addr = "127.0.0.1:" + *port
	}
	return grainharness.NewManager().Serve(ctx, grainharness.ServeOptions{
		RootDir:   *root,
		GrainID:   *grainID,
		Addr:      *addr,
		SessionID: *sessionID,
		BaseURL:   *baseURL,
		UserAgent: *userAgent,
		PkgDef:    *pkgDef,
		MocksPath: *mocks,
		Port:      *port,
	})
}

func runLogs(args []string) error {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	root := fs.String("root", ".app-harness", "harness root directory")
	grainID := fs.String("grain", "", "grain ID")
	name := fs.String("name", "supervisor", "log name: supervisor or monitor")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *grainID == "" {
		return errors.New("logs requires --grain")
	}
	data, err := grainharness.TailLog(*root, *grainID, *name)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(data)
	return err
}

func runDump(args []string) error {
	fs := flag.NewFlagSet("dump", flag.ContinueOnError)
	root := fs.String("root", ".app-harness", "harness root directory")
	grainID := fs.String("grain", "", "grain ID")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *grainID == "" {
		return errors.New("dump requires --grain")
	}
	data, err := grainharness.NewManager().Dump(*root, *grainID)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(data, '\n'))
	return err
}

func runKeepAlive(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("_keepalive", flag.ContinueOnError)
	root := fs.String("root", ".app-harness", "harness root directory")
	grainID := fs.String("grain", "", "grain ID")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *grainID == "" {
		return errors.New("_keepalive requires --grain")
	}
	return grainharness.RunKeepAlive(ctx, *root, *grainID)
}

func writeJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = os.Stdout.Write(data)
	return err
}

type multiFlag []string

func (m *multiFlag) String() string {
	return strings.Join(*m, ",")
}

func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
