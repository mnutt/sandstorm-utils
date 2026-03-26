package grainharness

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/rpc"

	activity "github.com/mnutt/sandstorm-utils/internal/generated/activity"
	email "github.com/mnutt/sandstorm-utils/internal/generated/email"
	grain "github.com/mnutt/sandstorm-utils/internal/generated/grain"
	hacksession "github.com/mnutt/sandstorm-utils/internal/generated/hacksession"
	identity "github.com/mnutt/sandstorm-utils/internal/generated/identity"
	spk "github.com/mnutt/sandstorm-utils/internal/generated/spk"
	supervisor "github.com/mnutt/sandstorm-utils/internal/generated/supervisor"
	util "github.com/mnutt/sandstorm-utils/internal/generated/util"
	websession "github.com/mnutt/sandstorm-utils/internal/generated/websession"
)

const (
	StatusCreated = "created"
	StatusRunning = "running"
	StatusStopped = "stopped"
)

type Config struct {
	RootDir           string            `json:"rootDir"`
	GrainID           string            `json:"grainId"`
	Mode              string            `json:"mode"`
	SupervisorCommand string            `json:"supervisorCommand"`
	SupervisorArgs    []string          `json:"supervisorArgs,omitempty"`
	SupervisorEnv     map[string]string `json:"supervisorEnv,omitempty"`
	SupervisorSocket  string            `json:"supervisorSocket"`
	PackagePath       string            `json:"packagePath,omitempty"`
	AppName           string            `json:"appName,omitempty"`
	AppCommand        []string          `json:"appCommand,omitempty"`
	MocksFile         string            `json:"mocksFile,omitempty"`
	KeepAliveInterval time.Duration     `json:"keepAliveInterval"`
	ConnectTimeout    time.Duration     `json:"connectTimeout"`
	BootMainView      bool              `json:"bootMainView"`
}

type State struct {
	Config        Config     `json:"config"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"createdAt"`
	StartedAt     *time.Time `json:"startedAt,omitempty"`
	StoppedAt     *time.Time `json:"stoppedAt,omitempty"`
	SupervisorPID int        `json:"supervisorPid,omitempty"`
	MonitorPID    int        `json:"monitorPid,omitempty"`
	LastError     string     `json:"lastError,omitempty"`
}

type Event struct {
	Time   time.Time      `json:"time"`
	Kind   string         `json:"kind"`
	Fields map[string]any `json:"fields,omitempty"`
}

type SessionSpec struct {
	ID                  string    `json:"id"`
	CreatedAt           time.Time `json:"createdAt"`
	BasePath            string    `json:"basePath"`
	UserAgent           string    `json:"userAgent"`
	AcceptableLanguages []string  `json:"acceptableLanguages,omitempty"`
	TabID               string    `json:"tabId"`
}

type RequestSpec struct {
	SessionID string
	Method    string
	Path      string
	MimeType  string
	Body      []byte
	Encoding  string
	Headers   map[string]string
	Cookies   map[string]string
}

type ResponsePayload struct {
	SessionID  string         `json:"sessionId"`
	Method     string         `json:"method"`
	Path       string         `json:"path"`
	Variant    string         `json:"variant"`
	StatusCode int            `json:"statusCode,omitempty"`
	MimeType   string         `json:"mimeType,omitempty"`
	Encoding   string         `json:"encoding,omitempty"`
	Language   string         `json:"language,omitempty"`
	Location   string         `json:"location,omitempty"`
	BodyText   string         `json:"bodyText,omitempty"`
	BodyBase64 string         `json:"bodyBase64,omitempty"`
	BodyBytes  int            `json:"bodyBytes,omitempty"`
	Headers    map[string]any `json:"headers,omitempty"`
}

type RequestCookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type HarnessMocks struct {
	PublicID    MockPublicID    `json:"publicId"`
	UserAddress MockUserAddress `json:"userAddress"`
}

type MockPublicID struct {
	PublicID   string `json:"publicId"`
	Hostname   string `json:"hostname"`
	AutoURL    string `json:"autoUrl"`
	IsDemoUser bool   `json:"isDemoUser"`
}

type MockUserAddress struct {
	Address string `json:"address"`
	Name    string `json:"name"`
}

type materializedPackage struct {
	PackagePath    string
	AppName        string
	Command        []string
	Env            map[string]string
	SupervisorPath string
}

type ServeOptions struct {
	RootDir   string
	GrainID   string
	Addr      string
	SessionID string
	BaseURL   string
	UserAgent string
	PkgDef    string
	MocksPath string
	Port      string
}

type Manager struct {
	mu sync.Mutex
}

func NewManager() *Manager {
	return &Manager{}
}

func DefaultConfig(rootDir, grainID string) Config {
	grainDir := filepath.Join(rootDir, "grains", grainID)
	return Config{
		RootDir:           rootDir,
		GrainID:           grainID,
		Mode:              "test",
		SupervisorSocket:  filepath.Join(grainDir, "var", "socket"),
		KeepAliveInterval: 30 * time.Second,
		ConnectTimeout:    20 * time.Second,
		BootMainView:      true,
	}
}

func (m *Manager) Create(cfg Config) (*State, error) {
	if strings.TrimSpace(cfg.RootDir) == "" {
		return nil, errors.New("rootDir is required")
	}
	if strings.TrimSpace(cfg.GrainID) == "" {
		return nil, errors.New("grainId is required")
	}
	if cfg.Mode == "" {
		cfg.Mode = "test"
	}
	if cfg.KeepAliveInterval <= 0 {
		cfg.KeepAliveInterval = 30 * time.Second
	}
	if cfg.ConnectTimeout <= 0 {
		cfg.ConnectTimeout = 20 * time.Second
	}
	if cfg.SupervisorSocket == "" {
		cfg.SupervisorSocket = filepath.Join(cfg.RootDir, "grains", cfg.GrainID, "var", "socket")
	}

	now := time.Now().UTC()
	state := &State{
		Config:    cfg,
		Status:    StatusCreated,
		CreatedAt: now,
	}

	if err := ensureLayout(cfg); err != nil {
		return nil, err
	}
	if err := writeState(stateFile(cfg), state); err != nil {
		return nil, err
	}
	if err := AppendEvent(cfg, "grain.created", map[string]any{
		"mode":       cfg.Mode,
		"socketPath": cfg.SupervisorSocket,
	}); err != nil {
		return nil, err
	}
	return state, nil
}

func (m *Manager) Load(rootDir, grainID string) (*State, error) {
	return readState(filepath.Join(rootDir, "grains", grainID, "grain.json"))
}

func (m *Manager) Start(ctx context.Context, rootDir, grainID, executable string) (*State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, err := m.Load(rootDir, grainID)
	if err != nil {
		return nil, err
	}
	if state.Config.SupervisorCommand == "" {
		return nil, errors.New("supervisorCommand is required before start")
	}
	if state.Status == StatusRunning && processAlive(state.SupervisorPID) {
		return nil, fmt.Errorf("grain %q is already running", grainID)
	}

	supervisorLog, err := os.OpenFile(filepath.Join(grainDir(state.Config), "logs", "supervisor.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	defer supervisorLog.Close()

	supervisorArgs, err := state.Config.launchArgs(state)
	if err != nil {
		return nil, err
	}

	supervisorCmd := exec.CommandContext(ctx, state.Config.SupervisorCommand, supervisorArgs...)
	supervisorCmd.Dir = grainDir(state.Config)
	supervisorCmd.Stdout = supervisorLog
	supervisorCmd.Stderr = supervisorLog
	supervisorCmd.Env = mergeEnv(os.Environ(), state.Config.SupervisorEnv, map[string]string{
		"SANDSTORM_GRAINHARNESS_ROOT":      state.Config.RootDir,
		"SANDSTORM_GRAINHARNESS_GRAIN_ID":  state.Config.GrainID,
		"SANDSTORM_GRAINHARNESS_GRAIN_DIR": grainDir(state.Config),
		"SANDSTORM_GRAINHARNESS_VAR_DIR":   filepath.Join(grainDir(state.Config), "var"),
		"SANDSTORM_GRAINHARNESS_SOCKET":    state.Config.SupervisorSocket,
	})
	if err := supervisorCmd.Start(); err != nil {
		return nil, fmt.Errorf("start supervisor: %w", err)
	}

	monitorLog, err := os.OpenFile(filepath.Join(grainDir(state.Config), "logs", "monitor.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		_ = terminatePID(supervisorCmd.Process.Pid, false)
		return nil, err
	}
	defer monitorLog.Close()

	monitorCmd := exec.CommandContext(ctx, executable,
		"_keepalive",
		"--root", state.Config.RootDir,
		"--grain", state.Config.GrainID,
	)
	monitorCmd.Dir = grainDir(state.Config)
	monitorCmd.Stdout = monitorLog
	monitorCmd.Stderr = monitorLog
	monitorCmd.Env = os.Environ()
	if err := monitorCmd.Start(); err != nil {
		_ = terminatePID(supervisorCmd.Process.Pid, false)
		return nil, fmt.Errorf("start keepalive monitor: %w", err)
	}

	now := time.Now().UTC()
	state.Status = StatusRunning
	state.StartedAt = &now
	state.StoppedAt = nil
	state.SupervisorPID = supervisorCmd.Process.Pid
	state.MonitorPID = monitorCmd.Process.Pid
	state.LastError = ""
	if err := writeState(stateFile(state.Config), state); err != nil {
		_ = terminatePID(monitorCmd.Process.Pid, false)
		_ = terminatePID(supervisorCmd.Process.Pid, false)
		return nil, err
	}

	if err := AppendEvent(state.Config, "grain.started", map[string]any{
		"supervisorPid": state.SupervisorPID,
		"monitorPid":    state.MonitorPID,
		"socketPath":    state.Config.SupervisorSocket,
	}); err != nil {
		return nil, err
	}
	return state, nil
}

func (m *Manager) Stop(rootDir, grainID string, force bool) (*State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, err := m.Load(rootDir, grainID)
	if err != nil {
		return nil, err
	}

	_ = terminatePID(state.MonitorPID, force)
	_ = terminatePID(state.SupervisorPID, force)

	now := time.Now().UTC()
	state.Status = StatusStopped
	state.StoppedAt = &now
	state.LastError = ""
	if err := writeState(stateFile(state.Config), state); err != nil {
		return nil, err
	}
	if err := AppendEvent(state.Config, "grain.stopped", map[string]any{
		"force": force,
	}); err != nil {
		return nil, err
	}
	return state, nil
}

func (m *Manager) Refresh(rootDir, grainID string) (*State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, err := m.Load(rootDir, grainID)
	if err != nil {
		return nil, err
	}
	running := processAlive(state.SupervisorPID)
	if running {
		state.Status = StatusRunning
	} else if state.Status == StatusRunning {
		state.Status = StatusStopped
		now := time.Now().UTC()
		state.StoppedAt = &now
		if state.LastError == "" {
			state.LastError = "supervisor process is no longer running"
		}
		if err := writeState(stateFile(state.Config), state); err != nil {
			return nil, err
		}
	}
	return state, nil
}

func (m *Manager) Dump(rootDir, grainID string) ([]byte, error) {
	state, err := m.Refresh(rootDir, grainID)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"state":      state,
		"grainDir":   grainDir(state.Config),
		"eventsPath": filepath.Join(grainDir(state.Config), "events.jsonl"),
		"logs": map[string]string{
			"supervisor": filepath.Join(grainDir(state.Config), "logs", "supervisor.log"),
			"monitor":    filepath.Join(grainDir(state.Config), "logs", "monitor.log"),
		},
	}
	return json.MarshalIndent(payload, "", "  ")
}

func (m *Manager) OpenSession(rootDir, grainID string, spec SessionSpec) (*SessionSpec, error) {
	state, err := m.Refresh(rootDir, grainID)
	if err != nil {
		return nil, err
	}
	if state.Status != StatusRunning {
		return nil, fmt.Errorf("grain %q is not running", grainID)
	}
	if spec.ID == "" {
		spec.ID = fmt.Sprintf("session-%d", time.Now().UTC().UnixNano())
	}
	if spec.CreatedAt.IsZero() {
		spec.CreatedAt = time.Now().UTC()
	}
	if spec.BasePath == "" {
		spec.BasePath = "http://grain.local"
	}
	if spec.UserAgent == "" {
		spec.UserAgent = "grain-harness/0"
	}
	if spec.TabID == "" {
		spec.TabID = spec.ID
	}
	if err := writeJSONFile(sessionFile(state.Config, spec.ID), &spec); err != nil {
		return nil, err
	}
	if err := AppendEvent(state.Config, "session.opened", map[string]any{
		"sessionId": spec.ID,
		"basePath":  spec.BasePath,
	}); err != nil {
		return nil, err
	}
	return &spec, nil
}

func (m *Manager) LoadSession(rootDir, grainID, sessionID string) (*SessionSpec, *State, error) {
	state, err := m.Refresh(rootDir, grainID)
	if err != nil {
		return nil, nil, err
	}
	var spec SessionSpec
	if err := readJSONFile(sessionFile(state.Config, sessionID), &spec); err != nil {
		return nil, nil, err
	}
	return &spec, state, nil
}

func (m *Manager) DoRequest(ctx context.Context, rootDir, grainID string, req RequestSpec) (*ResponsePayload, error) {
	spec, state, err := m.LoadSession(rootDir, grainID, req.SessionID)
	if err != nil {
		return nil, err
	}
	if state.Status != StatusRunning {
		return nil, fmt.Errorf("grain %q is not running", grainID)
	}

	logger, err := newEventLogger(state.Config)
	if err != nil {
		return nil, err
	}
	defer logger.Close()

	conn, err := waitForUnixSocket(ctx, state.Config.SupervisorSocket, state.Config.ConnectTimeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	core := &coreServer{logger: logger}
	rpcConn := rpc.NewConn(rpc.NewStreamTransport(conn), &rpc.Options{
		BootstrapClient: capnp.Client(supervisor.SandstormCore_ServerToClient(core)),
	})
	defer rpcConn.Close()

	sv := supervisor.Supervisor(rpcConn.Bootstrap(ctx))
	view, releaseView, err := getMainView(ctx, sv)
	if err != nil {
		return nil, err
	}
	defer releaseView()

	mocks, err := loadMocks(state.Config.MocksFile)
	if err != nil {
		return nil, err
	}
	sessionCtx := &sessionContextServer{logger: logger, mocks: mocks}
	web, releaseWeb, err := openWebSession(ctx, view, spec, sessionCtx)
	if err != nil {
		return nil, err
	}
	defer releaseWeb()

	payload, err := performWebRequest(ctx, web, req)
	if err != nil {
		return nil, err
	}
	payload.SessionID = req.SessionID
	payload.Method = strings.ToUpper(req.Method)
	payload.Path = req.Path

	if err := logger.Append("session.request", map[string]any{
		"sessionId":  req.SessionID,
		"method":     payload.Method,
		"path":       req.Path,
		"variant":    payload.Variant,
		"statusCode": payload.StatusCode,
	}); err != nil {
		return nil, err
	}
	return payload, nil
}

func RunKeepAlive(ctx context.Context, rootDir, grainID string) error {
	state, err := readState(filepath.Join(rootDir, "grains", grainID, "grain.json"))
	if err != nil {
		return err
	}

	logger, err := newEventLogger(state.Config)
	if err != nil {
		return err
	}
	defer logger.Close()

	core := &coreServer{logger: logger}
	if err := logger.Append("monitor.starting", map[string]any{
		"socketPath": state.Config.SupervisorSocket,
	}); err != nil {
		return err
	}

	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	for {
		if err := sigCtx.Err(); err != nil {
			return nil
		}
		conn, err := waitForUnixSocket(sigCtx, state.Config.SupervisorSocket, state.Config.ConnectTimeout)
		if err != nil {
			_ = logger.Append("monitor.connectFailed", map[string]any{"error": err.Error()})
			return err
		}

		rpcConn := rpc.NewConn(rpc.NewStreamTransport(conn), &rpc.Options{
			BootstrapClient: capnp.Client(supervisor.SandstormCore_ServerToClient(core)),
		})

		sv := supervisor.Supervisor(rpcConn.Bootstrap(sigCtx))
		if err := logger.Append("monitor.connected", nil); err != nil {
			_ = rpcConn.Close()
			return err
		}

		if state.Config.BootMainView {
			if err := bootMainView(sigCtx, sv, logger); err != nil {
				_ = logger.Append("monitor.bootMainViewFailed", map[string]any{"error": err.Error()})
			}
		}

		ticker := time.NewTicker(state.Config.KeepAliveInterval)
		runErr := runKeepAliveLoop(sigCtx, sv, core, logger, ticker)
		ticker.Stop()
		_ = rpcConn.Close()
		if runErr == nil || errors.Is(runErr, context.Canceled) {
			return nil
		}
		_ = logger.Append("monitor.disconnected", map[string]any{"error": runErr.Error()})
		select {
		case <-time.After(2 * time.Second):
		case <-sigCtx.Done():
			return nil
		}
	}
}

func (m *Manager) Serve(ctx context.Context, opts ServeOptions) error {
	if strings.TrimSpace(opts.RootDir) == "" {
		return errors.New("rootDir is required")
	}
	if strings.TrimSpace(opts.Addr) == "" {
		opts.Addr = "127.0.0.1:3000"
	}
	if strings.TrimSpace(opts.Port) == "" {
		if _, port, err := net.SplitHostPort(opts.Addr); err == nil {
			opts.Port = port
		}
	}
	if strings.TrimSpace(opts.GrainID) == "" {
		if strings.TrimSpace(opts.PkgDef) == "" {
			return errors.New("grainId is required")
		}
		opts.GrainID = "serve"
	}
	if strings.TrimSpace(opts.SessionID) == "" {
		opts.SessionID = "serve"
	}
	if strings.TrimSpace(opts.BaseURL) == "" {
		opts.BaseURL = "http://" + opts.Addr
	}
	if strings.TrimSpace(opts.UserAgent) == "" {
		opts.UserAgent = "grain-harness-serve/0"
	}
	if strings.TrimSpace(opts.PkgDef) != "" {
		if err := m.prepareServedGrain(ctx, opts); err != nil {
			return err
		}
	}

	if _, _, err := m.LoadSession(opts.RootDir, opts.GrainID, opts.SessionID); err != nil {
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) {
			return err
		}
		if _, err := m.OpenSession(opts.RootDir, opts.GrainID, SessionSpec{
			ID:        opts.SessionID,
			BasePath:  opts.BaseURL,
			UserAgent: opts.UserAgent,
			TabID:     opts.SessionID,
		}); err != nil {
			return err
		}
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("read request body: %v", err), http.StatusBadRequest)
			return
		}

		headers := make(map[string]string)
		for key, values := range r.Header {
			if len(values) == 0 {
				continue
			}
			headers[key] = strings.Join(values, ", ")
		}
		cookies := make(map[string]string)
		for _, cookie := range r.Cookies() {
			cookies[cookie.Name] = cookie.Value
		}

		targetPath := r.URL.Path
		if r.URL.RawQuery != "" {
			targetPath += "?" + r.URL.RawQuery
		}

		payload, err := m.DoRequest(r.Context(), opts.RootDir, opts.GrainID, RequestSpec{
			SessionID: opts.SessionID,
			Method:    r.Method,
			Path:      targetPath,
			MimeType:  r.Header.Get("Content-Type"),
			Body:      body,
			Encoding:  r.Header.Get("Content-Encoding"),
			Headers:   headers,
			Cookies:   cookies,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("grain request failed: %v", err), http.StatusBadGateway)
			return
		}
		writeHTTPResponse(w, payload)
	})

	server := &http.Server{
		Addr:              opts.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (m *Manager) prepareServedGrain(ctx context.Context, opts ServeOptions) error {
	state, err := m.Load(opts.RootDir, opts.GrainID)
	if err == nil {
		if state.Status == StatusRunning && processAlive(state.SupervisorPID) {
			return nil
		}
	}

	materialized, err := materializePackage(ctx, opts.RootDir, opts.PkgDef)
	if err != nil {
		return err
	}

	cfg := DefaultConfig(opts.RootDir, opts.GrainID)
	cfg.Mode = "serve"
	cfg.SupervisorCommand = materialized.SupervisorPath
	cfg.PackagePath = materialized.PackagePath
	cfg.AppName = materialized.AppName
	cfg.AppCommand = materialized.Command
	cfg.MocksFile = opts.MocksPath
	cfg.SupervisorEnv = materialized.Env

	if _, err := m.Create(cfg); err != nil {
		if !strings.Contains(err.Error(), "file exists") && !strings.Contains(err.Error(), "exists") {
			loadErr := err
			if _, stateErr := m.Load(opts.RootDir, opts.GrainID); stateErr == nil {
				loadErr = nil
			}
			if loadErr != nil {
				return err
			}
		}
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if _, err := m.Start(ctx, opts.RootDir, opts.GrainID, exe); err != nil && !strings.Contains(err.Error(), "already running") {
		return err
	}
	return nil
}

func runKeepAliveLoop(ctx context.Context, sv supervisor.Supervisor, core *coreServer, logger *eventLogger, ticker *time.Ticker) error {
	for {
		if err := sendKeepAlive(ctx, sv, core); err != nil {
			return err
		}
		if err := logger.Append("monitor.keepAlive", nil); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func sendKeepAlive(ctx context.Context, sv supervisor.Supervisor, core *coreServer) error {
	result, release := sv.KeepAlive(ctx, func(p supervisor.Supervisor_keepAlive_Params) error {
		return p.SetCore(supervisor.SandstormCore_ServerToClient(core))
	})
	defer release()
	_, err := result.Struct()
	return err
}

func writeHTTPResponse(w http.ResponseWriter, payload *ResponsePayload) {
	if payload.MimeType != "" {
		w.Header().Set("Content-Type", payload.MimeType)
	}
	if payload.Encoding != "" {
		w.Header().Set("Content-Encoding", payload.Encoding)
	}
	if payload.Language != "" {
		w.Header().Set("Content-Language", payload.Language)
	}
	if payload.Location != "" {
		w.Header().Set("Location", payload.Location)
	}

	status := payload.StatusCode
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)

	switch {
	case payload.BodyText != "":
		_, _ = io.WriteString(w, payload.BodyText)
	case payload.BodyBase64 != "":
		data, err := base64.StdEncoding.DecodeString(payload.BodyBase64)
		if err == nil {
			_, _ = w.Write(data)
		}
	}
}

func materializePackage(ctx context.Context, rootDir, pkgDef string) (*materializedPackage, error) {
	if strings.TrimSpace(pkgDef) == "" {
		return nil, errors.New("pkgDef is required")
	}
	pkgDefPath, symbol, err := splitPkgDef(pkgDef)
	if err != nil {
		return nil, err
	}

	workDir := filepath.Dir(pkgDefPath)
	stageDir := filepath.Join(rootDir, "_serve")
	homeDir := filepath.Join(stageDir, "home")
	spkFile := filepath.Join(stageDir, "app.spk")
	pkgDir := filepath.Join(stageDir, "pkg")
	spkLink := filepath.Join(stageDir, "spk")
	supervisorLink := filepath.Join(stageDir, "sandstorm-supervisor")
	tempPkgDef := filepath.Join(workDir, "grain-harness-serve-pkgdef.capnp")

	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		return nil, err
	}
	_ = os.RemoveAll(pkgDir)
	_ = os.Remove(spkFile)
	_ = os.Remove(spkLink)
	_ = os.Remove(supervisorLink)

	sandstormPath, err := installedSandstormBinary()
	if err != nil {
		return nil, err
	}
	if err := os.Symlink(sandstormPath, spkLink); err != nil {
		return nil, err
	}
	if err := os.Symlink(sandstormPath, supervisorLink); err != nil {
		return nil, err
	}

	appID, err := runOutput(ctx, workDir, map[string]string{"HOME": homeDir}, spkLink, "keygen", "-q")
	if err != nil {
		return nil, fmt.Errorf("spk keygen: %w", err)
	}
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return nil, errors.New("spk keygen returned an empty app id")
	}

	if err := rewritePkgDefID(pkgDefPath, tempPkgDef, appID); err != nil {
		return nil, err
	}
	defer os.Remove(tempPkgDef)

	if _, err := runOutput(ctx, workDir, map[string]string{"HOME": homeDir}, spkLink,
		"pack", "--pkg-def="+tempPkgDef+":"+symbol, spkFile,
	); err != nil {
		return nil, fmt.Errorf("spk pack: %w", err)
	}
	if _, err := runOutput(ctx, workDir, nil, spkLink, "unpack", spkFile, pkgDir); err != nil {
		return nil, fmt.Errorf("spk unpack: %w", err)
	}

	command, env, err := readPackageCommand(filepath.Join(pkgDir, "sandstorm-manifest"))
	if err != nil {
		return nil, err
	}

	return &materializedPackage{
		PackagePath:    pkgDir,
		AppName:        appID,
		Command:        command,
		Env:            env,
		SupervisorPath: supervisorLink,
	}, nil
}

func splitPkgDef(pkgDef string) (path string, symbol string, err error) {
	parts := strings.Split(pkgDef, ":")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("pkgDef must be in path:symbol form")
	}
	return parts[0], parts[1], nil
}

func installedSandstormBinary() (string, error) {
	const sandstorm = "/opt/sandstorm/latest/sandstorm"
	if _, err := os.Stat(sandstorm); err != nil {
		return "", fmt.Errorf("missing installed Sandstorm at %s: %w", sandstorm, err)
	}
	return sandstorm, nil
}

func rewritePkgDefID(srcPath, dstPath, appID string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	updated := regexp.MustCompile(`id = ".*",`).ReplaceAll(data, []byte(`id = "`+appID+`",`))
	return os.WriteFile(dstPath, updated, 0o644)
}

func readPackageCommand(path string) ([]string, map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	msg, err := capnp.NewDecoder(file).Decode()
	if err != nil {
		return nil, nil, err
	}
	manifest, err := spk.ReadRootManifest(msg)
	if err != nil {
		return nil, nil, err
	}
	command, err := manifest.ContinueCommand()
	if err != nil {
		return nil, nil, err
	}
	argvList, err := command.Argv()
	if err != nil {
		return nil, nil, err
	}
	argv := make([]string, 0, argvList.Len())
	for i := 0; i < argvList.Len(); i++ {
		value, err := argvList.At(i)
		if err != nil {
			return nil, nil, err
		}
		argv = append(argv, value)
	}

	env := map[string]string{}
	envList, err := command.Environ()
	if err != nil {
		return nil, nil, err
	}
	for i := 0; i < envList.Len(); i++ {
		entry := envList.At(i)
		keyPtr, err := entry.Key()
		if err != nil {
			return nil, nil, err
		}
		valPtr, err := entry.Value()
		if err != nil {
			return nil, nil, err
		}
		key := keyPtr.Text()
		val := valPtr.Text()
		env[key] = val
	}
	return argv, env, nil
}

func runOutput(ctx context.Context, dir string, extraEnv map[string]string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = mergeEnv(os.Environ(), extraEnv, nil)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func bootMainView(ctx context.Context, sv supervisor.Supervisor, logger *eventLogger) error {
	view, release, err := getMainView(ctx, sv)
	if err != nil {
		return err
	}
	defer release()
	_ = view
	return logger.Append("monitor.bootMainView", nil)
}

func getMainView(ctx context.Context, sv supervisor.Supervisor) (grain.UiView, capnp.ReleaseFunc, error) {
	result, release := sv.GetMainView(ctx, nil)
	results, err := result.Struct()
	if err != nil {
		release()
		return grain.UiView{}, nil, err
	}
	view := results.View()
	if !capnp.Client(view).IsValid() {
		release()
		return grain.UiView{}, nil, errors.New("getMainView returned a null UiView capability")
	}
	return view, func() {
		capnp.Client(view).Release()
		release()
	}, nil
}

func openWebSession(ctx context.Context, view grain.UiView, spec *SessionSpec, sessionCtx grain.SessionContext_Server) (websession.WebSession, capnp.ReleaseFunc, error) {
	userInfo, err := newStubUserInfo()
	if err != nil {
		return websession.WebSession{}, nil, err
	}
	params, err := newWebSessionParams(spec)
	if err != nil {
		return websession.WebSession{}, nil, err
	}
	result, release := view.NewSession(ctx, func(p grain.UiView_newSession_Params) error {
		if err := p.SetUserInfo(userInfo); err != nil {
			return err
		}
		if err := p.SetContext(grain.SessionContext_ServerToClient(sessionCtx)); err != nil {
			return err
		}
		p.SetSessionType(websession.WebSession_TypeID)
		if err := p.SetSessionParams(capnp.Struct(params).ToPtr()); err != nil {
			return err
		}
		return p.SetTabId([]byte(spec.TabID))
	})
	results, err := result.Struct()
	if err != nil {
		release()
		return websession.WebSession{}, nil, err
	}
	session := results.Session()
	ws := websession.WebSession(session)
	if !capnp.Client(ws).IsValid() {
		capnp.Client(session).Release()
		release()
		return websession.WebSession{}, nil, errors.New("newSession returned a null WebSession capability")
	}
	return ws, func() {
		capnp.Client(ws).Release()
		release()
	}, nil
}

func performWebRequest(ctx context.Context, web websession.WebSession, req RequestSpec) (*ResponsePayload, error) {
	stream := &byteStreamServer{}
	webCtx, err := newRequestContext(stream, req.Headers, req.Cookies)
	if err != nil {
		return nil, err
	}
	method := strings.ToUpper(req.Method)
	path := strings.TrimPrefix(req.Path, "/")

	var (
		future  websession.Response_Future
		release capnp.ReleaseFunc
	)

	switch method {
	case "GET":
		future, release = web.Get(ctx, func(p websession.WebSession_get_Params) error {
			if err := p.SetPath(path); err != nil {
				return err
			}
			if err := p.SetContext(webCtx); err != nil {
				return err
			}
			p.SetIgnoreBody(false)
			return nil
		})
	case "POST":
		content, err := newRequestContent(req)
		if err != nil {
			return nil, err
		}
		future, release = web.Post(ctx, func(p websession.WebSession_post_Params) error {
			if err := p.SetPath(path); err != nil {
				return err
			}
			if err := p.SetContent(content); err != nil {
				return err
			}
			return p.SetContext(webCtx)
		})
	case "PUT":
		content, err := newRequestContent(req)
		if err != nil {
			return nil, err
		}
		future, release = web.Put(ctx, func(p websession.WebSession_put_Params) error {
			if err := p.SetPath(path); err != nil {
				return err
			}
			if err := p.SetContent(content); err != nil {
				return err
			}
			return p.SetContext(webCtx)
		})
	case "PATCH":
		content, err := newRequestContent(req)
		if err != nil {
			return nil, err
		}
		future, release = web.Patch(ctx, func(p websession.WebSession_patch_Params) error {
			if err := p.SetPath(path); err != nil {
				return err
			}
			if err := p.SetContent(content); err != nil {
				return err
			}
			return p.SetContext(webCtx)
		})
	case "DELETE":
		future, release = web.Delete(ctx, func(p websession.WebSession_delete_Params) error {
			if err := p.SetPath(path); err != nil {
				return err
			}
			return p.SetContext(webCtx)
		})
	default:
		return nil, fmt.Errorf("unsupported method %q", req.Method)
	}
	defer release()

	response, err := future.Struct()
	if err != nil {
		return nil, err
	}
	if err := stream.wait(); err != nil {
		return nil, err
	}
	return decodeResponse(response, stream.Bytes()), nil
}

func newRequestContent(req RequestSpec) (websession.RequestContent, error) {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return websession.RequestContent{}, err
	}
	content, err := websession.NewRootRequestContent(seg)
	if err != nil {
		return websession.RequestContent{}, err
	}
	if req.MimeType != "" {
		if err := content.SetMimeType(req.MimeType); err != nil {
			return websession.RequestContent{}, err
		}
	}
	if len(req.Body) > 0 {
		if err := content.SetContent(req.Body); err != nil {
			return websession.RequestContent{}, err
		}
	}
	if req.Encoding != "" {
		if err := content.SetEncoding(req.Encoding); err != nil {
			return websession.RequestContent{}, err
		}
	}
	return content, nil
}

func newWebSessionParams(spec *SessionSpec) (websession.Params, error) {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return websession.Params{}, err
	}
	params, err := websession.NewRootParams(seg)
	if err != nil {
		return websession.Params{}, err
	}
	if err := params.SetBasePath(spec.BasePath); err != nil {
		return websession.Params{}, err
	}
	if err := params.SetUserAgent(spec.UserAgent); err != nil {
		return websession.Params{}, err
	}
	list, err := params.NewAcceptableLanguages(int32(len(spec.AcceptableLanguages)))
	if err != nil {
		return websession.Params{}, err
	}
	for i, lang := range spec.AcceptableLanguages {
		if err := list.Set(i, lang); err != nil {
			return websession.Params{}, err
		}
	}
	return params, nil
}

func newStubUserInfo() (identity.UserInfo, error) {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return identity.UserInfo{}, err
	}
	user, err := identity.NewRootUserInfo(seg)
	if err != nil {
		return identity.UserInfo{}, err
	}
	displayName, err := user.NewDisplayName()
	if err != nil {
		return identity.UserInfo{}, err
	}
	if err := displayName.SetDefaultText("Grain Harness"); err != nil {
		return identity.UserInfo{}, err
	}
	if err := user.SetPreferredHandle("grain-harness"); err != nil {
		return identity.UserInfo{}, err
	}
	if _, err := user.NewPermissions(0); err != nil {
		return identity.UserInfo{}, err
	}
	return user, nil
}

func newRequestContext(stream util.ByteStream_Server, headers map[string]string, cookies map[string]string) (websession.Context, error) {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return websession.Context{}, err
	}
	ctx, err := websession.NewRootContext(seg)
	if err != nil {
		return websession.Context{}, err
	}
	if err := ctx.SetResponseStream(util.ByteStream_ServerToClient(stream)); err != nil {
		return websession.Context{}, err
	}
	accept, err := ctx.NewAccept(1)
	if err != nil {
		return websession.Context{}, err
	}
	if err := accept.At(0).SetMimeType("*/*"); err != nil {
		return websession.Context{}, err
	}
	enc, err := ctx.NewAcceptEncoding(1)
	if err != nil {
		return websession.Context{}, err
	}
	if err := enc.At(0).SetContentCoding("identity"); err != nil {
		return websession.Context{}, err
	}
	ctx.ETagPrecondition().SetNone()
	headerList, err := ctx.NewAdditionalHeaders(int32(len(headers)))
	if err != nil {
		return websession.Context{}, err
	}
	for i, entry := range sortedEntries(headers) {
		header := headerList.At(i)
		if err := setKeyValueText(header, entry[0], entry[1]); err != nil {
			return websession.Context{}, err
		}
	}
	cookieList, err := ctx.NewCookies(int32(len(cookies)))
	if err != nil {
		return websession.Context{}, err
	}
	for i, entry := range sortedEntries(cookies) {
		cookie := cookieList.At(i)
		if err := setKeyValueText(cookie, entry[0], entry[1]); err != nil {
			return websession.Context{}, err
		}
	}
	return ctx, nil
}

func decodeResponse(response websession.Response, streamed []byte) *ResponsePayload {
	payload := &ResponsePayload{
		Headers: map[string]any{},
	}
	switch response.Which() {
	case websession.Response_Which_content:
		payload.Variant = "content"
		content := response.Content()
		payload.StatusCode = successStatusCode(content.StatusCode())
		payload.MimeType, _ = content.MimeType()
		payload.Encoding, _ = content.Encoding()
		payload.Language, _ = content.Language()
		body := content.Body()
		if body.Which() == websession.Response_content_body_Which_bytes {
			data, _ := body.Bytes()
			setBody(payload, data)
		} else if body.Which() == websession.Response_content_body_Which_stream {
			setBody(payload, streamed)
		}
	case websession.Response_Which_noContent:
		payload.Variant = "noContent"
		payload.StatusCode = 204
		payload.Headers["shouldResetForm"] = response.NoContent().ShouldResetForm()
	case websession.Response_Which_preconditionFailed:
		payload.Variant = "preconditionFailed"
		payload.StatusCode = 412
	case websession.Response_Which_redirect:
		payload.Variant = "redirect"
		redirect := response.Redirect()
		payload.StatusCode = redirectStatusCode(redirect.IsPermanent(), redirect.SwitchToGet())
		payload.Location, _ = redirect.Location()
	case websession.Response_Which_clientError:
		payload.Variant = "clientError"
		clientError := response.ClientError()
		payload.StatusCode = clientErrorStatusCode(clientError.StatusCode())
		if clientError.HasNonHtmlBody() {
			nonHTML, _ := clientError.NonHtmlBody()
			decodeErrorBody(payload, nonHTML)
		} else {
			body, _ := clientError.DescriptionHtml()
			setBody(payload, []byte(body))
			payload.MimeType = "text/html"
		}
	case websession.Response_Which_serverError:
		payload.Variant = "serverError"
		payload.StatusCode = 500
		serverError := response.ServerError()
		if serverError.HasNonHtmlBody() {
			nonHTML, _ := serverError.NonHtmlBody()
			decodeErrorBody(payload, nonHTML)
		} else {
			body, _ := serverError.DescriptionHtml()
			setBody(payload, []byte(body))
			payload.MimeType = "text/html"
		}
	default:
		payload.Variant = response.Which().String()
	}
	return payload
}

func setBody(payload *ResponsePayload, body []byte) {
	payload.BodyBytes = len(body)
	if len(body) == 0 {
		return
	}
	if isTextBody(body) {
		payload.BodyText = string(body)
		return
	}
	payload.BodyBase64 = encodeBase64(body)
}

type sessionContextServer struct {
	logger *eventLogger
	mocks  HarnessMocks
}

func (s *sessionContextServer) GetSharedPermissions(context.Context, grain.SessionContext_getSharedPermissions) error {
	return s.logger.Append("sessionContext.getSharedPermissions", nil)
}

func (s *sessionContextServer) TieToUser(context.Context, grain.SessionContext_tieToUser) error {
	return s.logger.Append("sessionContext.tieToUser", nil)
}

func (s *sessionContextServer) Offer(context.Context, grain.SessionContext_offer) error {
	return s.logger.Append("sessionContext.offer", nil)
}

func (s *sessionContextServer) Request(context.Context, grain.SessionContext_request) error {
	return s.logger.Append("sessionContext.request", nil)
}

func (s *sessionContextServer) FulfillRequest(context.Context, grain.SessionContext_fulfillRequest) error {
	return s.logger.Append("sessionContext.fulfillRequest", nil)
}

func (s *sessionContextServer) Close(context.Context, grain.SessionContext_close) error {
	return s.logger.Append("sessionContext.close", nil)
}

func (s *sessionContextServer) OpenView(_ context.Context, call grain.SessionContext_openView) error {
	path, _ := call.Args().Path()
	return s.logger.Append("sessionContext.openView", map[string]any{"path": path})
}

func (s *sessionContextServer) ClaimRequest(context.Context, grain.SessionContext_claimRequest) error {
	return s.logger.Append("sessionContext.claimRequest", nil)
}

func (s *sessionContextServer) Activity(_ context.Context, call grain.SessionContext_activity) error {
	args := call.Args()
	event, err := args.Event()
	if err != nil {
		return err
	}
	return s.logger.Append("sessionContext.activity", RenderActivityEvent(event))
}

func (s *sessionContextServer) GetPublicId(_ context.Context, call hacksession.HackSessionContext_getPublicId) error {
	if err := s.logger.Append("sessionContext.getPublicId", map[string]any{
		"publicId":   s.mocks.PublicID.PublicID,
		"hostname":   s.mocks.PublicID.Hostname,
		"autoUrl":    s.mocks.PublicID.AutoURL,
		"isDemoUser": s.mocks.PublicID.IsDemoUser,
	}); err != nil {
		return err
	}
	results, err := call.AllocResults()
	if err != nil {
		return err
	}
	if err := results.SetPublicId(s.mocks.PublicID.PublicID); err != nil {
		return err
	}
	if err := results.SetHostname(s.mocks.PublicID.Hostname); err != nil {
		return err
	}
	if err := results.SetAutoUrl(s.mocks.PublicID.AutoURL); err != nil {
		return err
	}
	results.SetIsDemoUser(s.mocks.PublicID.IsDemoUser)
	return nil
}

func (s *sessionContextServer) GetUserAddress(_ context.Context, call hacksession.HackSessionContext_getUserAddress) error {
	if err := s.logger.Append("sessionContext.getUserAddress", map[string]any{
		"address": s.mocks.UserAddress.Address,
		"name":    s.mocks.UserAddress.Name,
	}); err != nil {
		return err
	}
	results, err := call.AllocResults()
	if err != nil {
		return err
	}
	if err := results.SetAddress(s.mocks.UserAddress.Address); err != nil {
		return err
	}
	return results.SetName(s.mocks.UserAddress.Name)
}

func (s *sessionContextServer) ObsoleteHttpGet(context.Context, hacksession.HackSessionContext_obsoleteHttpGet) error {
	return errors.New("not implemented")
}

func (s *sessionContextServer) ObsoleteGenerateApiToken(context.Context, hacksession.HackSessionContext_obsoleteGenerateApiToken) error {
	return errors.New("not implemented")
}

func (s *sessionContextServer) ObsoleteListApiTokens(context.Context, hacksession.HackSessionContext_obsoleteListApiTokens) error {
	return errors.New("not implemented")
}

func (s *sessionContextServer) ObsoleteRevokeApiToken(context.Context, hacksession.HackSessionContext_obsoleteRevokeApiToken) error {
	return errors.New("not implemented")
}

func (s *sessionContextServer) ObsoleteGetIpNetwork(context.Context, hacksession.HackSessionContext_obsoleteGetIpNetwork) error {
	return errors.New("not implemented")
}

func (s *sessionContextServer) ObsoleteGetIpInterface(context.Context, hacksession.HackSessionContext_obsoleteGetIpInterface) error {
	return errors.New("not implemented")
}

func (s *sessionContextServer) ObsoleteGetUiViewForEndpoint(context.Context, hacksession.HackSessionContext_obsoleteGetUiViewForEndpoint) error {
	return errors.New("not implemented")
}

func (s *sessionContextServer) Send(_ context.Context, call email.EmailSendPort_send) error {
	args := call.Args()
	message, err := args.Email()
	if err != nil {
		return err
	}

	from, err := readEmailAddress(message.From)
	if err != nil {
		return err
	}
	to, err := readEmailAddressList(message.To)
	if err != nil {
		return err
	}
	cc, err := readEmailAddressList(message.Cc)
	if err != nil {
		return err
	}
	bcc, err := readEmailAddressList(message.Bcc)
	if err != nil {
		return err
	}

	fields := map[string]any{
		"from": from,
		"to":   to,
		"cc":   cc,
		"bcc":  bcc,
	}
	if message.HasReplyTo() {
		replyTo, err := readEmailAddress(message.ReplyTo)
		if err != nil {
			return err
		}
		fields["replyTo"] = replyTo
	}
	if subject, err := message.Subject(); err == nil {
		fields["subject"] = subject
	}
	if text, err := message.Text(); err == nil {
		fields["text"] = text
	}
	if html, err := message.Html(); err == nil {
		fields["html"] = html
	}
	if err := s.logger.Append("sessionContext.sendEmail", fields); err != nil {
		return err
	}
	_, err = call.AllocResults()
	return err
}

func (s *sessionContextServer) HintAddress(context.Context, email.EmailSendPort_hintAddress) error {
	return nil
}

type byteStreamServer struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	done chan struct{}
}

func (s *byteStreamServer) Write(_ context.Context, call util.ByteStream_write) error {
	data, err := call.Args().Data()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.buf.Write(data)
	return err
}

func (s *byteStreamServer) Done(context.Context, util.ByteStream_done) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done == nil {
		s.done = make(chan struct{})
	}
	select {
	case <-s.done:
	default:
		close(s.done)
	}
	return nil
}

func (s *byteStreamServer) ExpectSize(context.Context, util.ByteStream_expectSize) error {
	return nil
}

func (s *byteStreamServer) wait() error {
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
	case <-time.After(1 * time.Second):
	}
	return nil
}

func (s *byteStreamServer) Bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.buf.Bytes()...)
}

type coreServer struct {
	logger *eventLogger
}

func (c *coreServer) Restore(context.Context, supervisor.SandstormCore_restore) error {
	return c.logger.Append("core.restore", nil)
}

func (c *coreServer) MakeToken(_ context.Context, call supervisor.SandstormCore_makeToken) error {
	if err := c.logger.Append("core.makeToken", nil); err != nil {
		return err
	}
	results, err := call.AllocResults()
	if err != nil {
		return err
	}
	return results.SetToken([]byte("grain-harness-token"))
}

func (c *coreServer) GetOwnerNotificationTarget(_ context.Context, call supervisor.SandstormCore_getOwnerNotificationTarget) error {
	if err := c.logger.Append("core.getOwnerNotificationTarget", nil); err != nil {
		return err
	}
	_, err := call.AllocResults()
	return err
}

func (c *coreServer) Drop(context.Context, supervisor.SandstormCore_drop) error {
	return c.logger.Append("core.drop", nil)
}

func (c *coreServer) ObsoleteCheckRequirements(context.Context, supervisor.SandstormCore_obsoleteCheckRequirements) error {
	return c.logger.Append("core.obsoleteCheckRequirements", nil)
}

func (c *coreServer) MakeChildToken(_ context.Context, call supervisor.SandstormCore_makeChildToken) error {
	if err := c.logger.Append("core.makeChildToken", nil); err != nil {
		return err
	}
	results, err := call.AllocResults()
	if err != nil {
		return err
	}
	return results.SetToken([]byte("grain-harness-child-token"))
}

func (c *coreServer) ClaimRequest(context.Context, supervisor.SandstormCore_claimRequest) error {
	return c.logger.Append("core.claimRequest", nil)
}

func (c *coreServer) BackgroundActivity(_ context.Context, call supervisor.SandstormCore_backgroundActivity) error {
	args := call.Args()
	event, err := args.Event()
	if err != nil {
		return err
	}
	path, _ := event.Path()
	return c.logger.Append("core.backgroundActivity", map[string]any{
		"path": path,
		"type": event.Type(),
	})
}

func (c *coreServer) ReportGrainSize(_ context.Context, call supervisor.SandstormCore_reportGrainSize) error {
	return c.logger.Append("core.reportGrainSize", map[string]any{
		"bytes": call.Args().Bytes(),
	})
}

func (c *coreServer) GetIdentityId(_ context.Context, call supervisor.SandstormCore_getIdentityId) error {
	if err := c.logger.Append("core.getIdentityId", nil); err != nil {
		return err
	}
	results, err := call.AllocResults()
	if err != nil {
		return err
	}
	return results.SetId([]byte("grain-harness-identity"))
}

func (c *coreServer) Schedule(_ context.Context, call supervisor.SandstormCore_schedule) error {
	job := call.Args()
	name, err := job.Name()
	if err != nil {
		return err
	}
	defaultText, _ := name.DefaultText()
	return c.logger.Append("core.schedule", map[string]any{
		"name": defaultText,
	})
}

type eventLogger struct {
	mu   sync.Mutex
	file *os.File
}

func newEventLogger(cfg Config) (*eventLogger, error) {
	file, err := os.OpenFile(filepath.Join(grainDir(cfg), "events.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &eventLogger{file: file}, nil
}

func (l *eventLogger) Append(kind string, fields map[string]any) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	line, err := json.Marshal(Event{
		Time:   time.Now().UTC(),
		Kind:   kind,
		Fields: fields,
	})
	if err != nil {
		return err
	}
	if _, err := l.file.Write(append(line, '\n')); err != nil {
		return err
	}
	return l.file.Sync()
}

func (l *eventLogger) Close() error {
	return l.file.Close()
}

func AppendEvent(cfg Config, kind string, fields map[string]any) error {
	logger, err := newEventLogger(cfg)
	if err != nil {
		return err
	}
	defer logger.Close()
	return logger.Append(kind, fields)
}

func ensureLayout(cfg Config) error {
	paths := []string{
		filepath.Join(cfg.RootDir, "grains"),
		grainDir(cfg),
		filepath.Join(grainDir(cfg), "logs"),
		filepath.Join(grainDir(cfg), "sessions"),
		filepath.Join(grainDir(cfg), "tokens"),
	}
	if !cfg.usesManagedSupervisor() {
		paths = append(paths, varDir(cfg))
	}
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (cfg Config) usesManagedSupervisor() bool {
	return cfg.PackagePath != "" || cfg.AppName != "" || len(cfg.AppCommand) > 0
}

func (cfg Config) launchArgs(state *State) ([]string, error) {
	if !cfg.usesManagedSupervisor() {
		return append([]string(nil), cfg.SupervisorArgs...), nil
	}
	if strings.TrimSpace(cfg.PackagePath) == "" {
		return nil, errors.New("packagePath is required when using managed supervisor mode")
	}
	if strings.TrimSpace(cfg.AppName) == "" {
		return nil, errors.New("appName is required when using managed supervisor mode")
	}
	if len(cfg.AppCommand) == 0 {
		return nil, errors.New("appCommand is required when using managed supervisor mode")
	}

	args := []string{
		"--pkg", cfg.PackagePath,
		"--var", varDir(cfg),
	}
	if state.StartedAt == nil {
		args = append(args, "--new")
	}
	args = append(args, cfg.AppName, cfg.GrainID)
	args = append(args, cfg.AppCommand...)
	return args, nil
}

func readState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func writeState(path string, state *State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func defaultMocks() HarnessMocks {
	return HarnessMocks{
		PublicID: MockPublicID{
			PublicID:   "grain-harness-public-id",
			Hostname:   "local.sandstorm.test",
			AutoURL:    "https://local.sandstorm.test/shared/grain-harness-public-id",
			IsDemoUser: false,
		},
		UserAddress: MockUserAddress{
			Address: "user@example.com",
			Name:    "Grain Harness User",
		},
	}
}

func loadMocks(path string) (HarnessMocks, error) {
	mocks := defaultMocks()
	if strings.TrimSpace(path) == "" {
		return mocks, nil
	}
	if err := readJSONFile(path, &mocks); err != nil {
		return HarnessMocks{}, err
	}
	return mocks, nil
}

func readJSONFile(path string, dest any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func grainDir(cfg Config) string {
	return filepath.Join(cfg.RootDir, "grains", cfg.GrainID)
}

func varDir(cfg Config) string {
	return filepath.Join(grainDir(cfg), "var")
}

func stateFile(cfg Config) string {
	return filepath.Join(grainDir(cfg), "grain.json")
}

func sessionFile(cfg Config, sessionID string) string {
	return filepath.Join(grainDir(cfg), "sessions", sessionID+".json")
}

func mergeEnv(base []string, maps ...map[string]string) []string {
	merged := map[string]string{}
	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			merged[key] = value
		}
	}
	for _, extra := range maps {
		for key, value := range extra {
			merged[key] = value
		}
	}
	out := make([]string, 0, len(merged))
	for key, value := range merged {
		out = append(out, key+"="+value)
	}
	return out
}

func terminatePID(pid int, force bool) error {
	if pid <= 0 {
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	return proc.Signal(sig)
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func waitForUnixSocket(ctx context.Context, path string, timeout time.Duration) (net.Conn, error) {
	deadline := time.Now().Add(timeout)
	for {
		conn, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
		if err == nil {
			return conn, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if timeout > 0 && time.Now().After(deadline) {
			return nil, fmt.Errorf("dial %s: %w", path, err)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func ReadEvents(rootDir, grainID string) ([]Event, error) {
	data, err := os.ReadFile(filepath.Join(rootDir, "grains", grainID, "events.jsonl"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	events := make([]Event, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func TailLog(rootDir, grainID, name string) ([]byte, error) {
	if name == "" {
		name = "supervisor"
	}
	if !strings.HasSuffix(name, ".log") {
		name += ".log"
	}
	return os.ReadFile(filepath.Join(rootDir, "grains", grainID, "logs", name))
}

func ParseEnv(entries []string) (map[string]string, error) {
	out := make(map[string]string, len(entries))
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid env entry %q", entry)
		}
		out[key] = value
	}
	return out, nil
}

func ParseDurationSeconds(v time.Duration) string {
	return strconv.FormatInt(int64(v/time.Second), 10)
}

func RenderActivityEvent(event activity.ActivityEvent) map[string]any {
	path, _ := event.Path()
	return map[string]any{
		"path": path,
		"type": event.Type(),
	}
}

func successStatusCode(code websession.SuccessCode) int {
	switch code {
	case websession.SuccessCode_ok:
		return 200
	case websession.SuccessCode_created:
		return 201
	case websession.SuccessCode_accepted:
		return 202
	case websession.SuccessCode_noContent:
		return 204
	case websession.SuccessCode_partialContent:
		return 206
	default:
		return 200
	}
}

func clientErrorStatusCode(code websession.ClientErrorCode) int {
	switch code {
	case websession.ClientErrorCode_badRequest:
		return 400
	case websession.ClientErrorCode_forbidden:
		return 403
	case websession.ClientErrorCode_notFound:
		return 404
	case websession.ClientErrorCode_methodNotAllowed:
		return 405
	case websession.ClientErrorCode_notAcceptable:
		return 406
	case websession.ClientErrorCode_conflict:
		return 409
	case websession.ClientErrorCode_gone:
		return 410
	case websession.ClientErrorCode_requestEntityTooLarge:
		return 413
	case websession.ClientErrorCode_requestUriTooLong:
		return 414
	case websession.ClientErrorCode_unsupportedMediaType:
		return 415
	case websession.ClientErrorCode_imATeapot:
		return 418
	default:
		return 400
	}
}

func redirectStatusCode(permanent bool, switchToGet bool) int {
	if permanent && !switchToGet {
		return 308
	}
	if permanent {
		return 301
	}
	if switchToGet {
		return 303
	}
	return 307
}

func isTextBody(body []byte) bool {
	for _, b := range body {
		if b == '\n' || b == '\r' || b == '\t' {
			continue
		}
		if b < 0x20 || b == 0x7f {
			return false
		}
	}
	return true
}

func encodeBase64(body []byte) string {
	return strings.TrimRight(base64.StdEncoding.EncodeToString(body), "\n")
}

func decodeErrorBody(payload *ResponsePayload, body websession.ErrorBody) {
	payload.MimeType, _ = body.MimeType()
	payload.Encoding, _ = body.Encoding()
	payload.Language, _ = body.Language()
	data, _ := body.Data()
	setBody(payload, data)
}

func readEmailAddress(get func() (email.EmailAddress, error)) (map[string]any, error) {
	addr, err := get()
	if err != nil {
		return nil, err
	}
	address, err := addr.Address()
	if err != nil {
		return nil, err
	}
	name, err := addr.Name()
	if err != nil {
		return nil, err
	}
	return map[string]any{"address": address, "name": name}, nil
}

func readEmailAddressList(get func() (email.EmailAddress_List, error)) ([]map[string]any, error) {
	list, err := get()
	if err != nil {
		return nil, err
	}
	values := make([]map[string]any, 0, list.Len())
	for i := 0; i < list.Len(); i++ {
		item := list.At(i)
		address, err := item.Address()
		if err != nil {
			return nil, err
		}
		name, err := item.Name()
		if err != nil {
			return nil, err
		}
		values = append(values, map[string]any{"address": address, "name": name})
	}
	return values, nil
}

func setKeyValueText(value util.KeyValue, key, text string) error {
	if err := capnp.Struct(value).SetText(0, key); err != nil {
		return err
	}
	return capnp.Struct(value).SetText(1, text)
}

func sortedEntries(values map[string]string) [][2]string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([][2]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, [2]string{key, values[key]})
	}
	return out
}

func ParseNameValues(entries []string) (map[string]string, error) {
	out := make(map[string]string, len(entries))
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid key=value entry %q", entry)
		}
		out[key] = value
	}
	return out, nil
}
