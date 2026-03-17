package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const sessionHeader = "X-Sandstorm-Session-Id"

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	app, err := newApp()
	if err != nil {
		log.Fatal(err)
	}

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           app.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("sandstorm-utils testapp listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}

type app struct {
	binDir string
	state  *stateStore
}

type stateStore struct {
	mu      sync.Mutex
	lastRun map[string]scenarioRun
	history []scenarioRun
}

type scenarioRun struct {
	Name      string          `json:"name"`
	StartedAt time.Time       `json:"startedAt"`
	EndedAt   time.Time       `json:"endedAt"`
	OK        bool            `json:"ok"`
	Error     string          `json:"error,omitempty"`
	SessionID string          `json:"sessionId"`
	Steps     []commandResult `json:"steps"`
}

type commandResult struct {
	Command    string   `json:"command"`
	Args       []string `json:"args"`
	Stdout     string   `json:"stdout"`
	Stderr     string   `json:"stderr"`
	ExitCode   int      `json:"exitCode"`
	DurationMS int64    `json:"durationMs"`
	ParsedJSON any      `json:"parsedJson,omitempty"`
}

type runRequest struct {
	SessionID string `json:"sessionId"`
	Recipient string `json:"recipient"`
}

func newApp() (*app, error) {
	binDir, err := resolveBinDir()
	if err != nil {
		return nil, err
	}

	return &app{
		binDir: binDir,
		state: &stateStore{
			lastRun: map[string]scenarioRun{},
		},
	}, nil
}

func resolveBinDir() (string, error) {
	if len(os.Args) == 0 || strings.TrimSpace(os.Args[0]) == "" {
		return "", errors.New("resolve executable path: argv[0] is empty")
	}

	exePath := os.Args[0]
	if !filepath.IsAbs(exePath) {
		absPath, err := filepath.Abs(exePath)
		if err != nil {
			return "", fmt.Errorf("resolve executable path: %w", err)
		}
		exePath = absPath
	}

	return filepath.Dir(exePath), nil
}

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleIndex)
	mux.HandleFunc("/api/results", a.handleResults)
	mux.HandleFunc("/api/run-all", a.handleRunAll)
	mux.HandleFunc("/api/run/", a.handleRunScenario)
	return mux
}

func (a *app) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, indexHTML)
}

func (a *app) handleResults(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"scenarios":       a.state.snapshot(),
		"headerSessionId": r.Header.Get(sessionHeader),
	})
}

func (a *app) handleRunScenario(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/run/")
	if name == "" {
		http.Error(w, "missing scenario name", http.StatusBadRequest)
		return
	}

	req, err := decodeRunRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sessionID := resolveSessionID(r, req)
	if sessionID == "" {
		http.Error(w, "missing Sandstorm session ID", http.StatusBadRequest)
		return
	}

	run := a.runScenario(r.Context(), name, sessionID, req)
	status := http.StatusOK
	if !run.OK {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, run)
}

func (a *app) handleRunAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req, err := decodeRunRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sessionID := resolveSessionID(r, req)
	if sessionID == "" {
		http.Error(w, "missing Sandstorm session ID", http.StatusBadRequest)
		return
	}

	scenarios := []string{"get-public-id", "get-user-address", "post-activity", "stay-awake"}
	results := make([]scenarioRun, 0, len(scenarios))
	for _, name := range scenarios {
		run := a.runScenario(r.Context(), name, sessionID, req)
		results = append(results, run)
		if !run.OK {
			break
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"results": results,
	})
}

func (a *app) runScenario(parent context.Context, name, sessionID string, req runRequest) scenarioRun {
	run := scenarioRun{
		Name:      name,
		StartedAt: time.Now().UTC(),
		SessionID: sessionID,
	}

	ctx, cancel := context.WithTimeout(parent, scenarioTimeout(name))
	defer cancel()

	var (
		steps []commandResult
		err   error
	)

	switch name {
	case "get-public-id":
		steps, err = a.runGetPublicID(ctx, sessionID)
	case "get-user-address":
		steps, err = a.runGetUserAddress(ctx, sessionID)
	case "open-view":
		steps, err = a.runOpenView(ctx, sessionID)
	case "close-session":
		steps, err = a.runCloseSession(ctx, sessionID)
	case "post-activity":
		steps, err = a.runPostActivity(ctx, sessionID)
	case "get-session-request":
		steps, err = a.runGetSessionRequest(ctx, sessionID)
	case "get-session-offer":
		steps, err = a.runGetSessionOffer(ctx, sessionID)
	case "send-email":
		steps, err = a.runSendEmail(ctx, sessionID, req.Recipient)
	case "stay-awake":
		steps, err = a.runStayAwake(ctx, sessionID)
	default:
		err = fmt.Errorf("unknown scenario %q", name)
	}

	run.EndedAt = time.Now().UTC()
	run.OK = err == nil
	run.Steps = steps
	if err != nil {
		run.Error = err.Error()
	}

	a.state.record(run)
	return run
}

func (a *app) runGetPublicID(ctx context.Context, sessionID string) ([]commandResult, error) {
	step, err := a.execJSON(ctx, "get-public-id", []string{"--json", sessionID})
	if err != nil {
		return []commandResult{step}, err
	}
	return []commandResult{step}, nil
}

func (a *app) runGetUserAddress(ctx context.Context, sessionID string) ([]commandResult, error) {
	step, err := a.execJSON(ctx, "get-user-address", []string{"--json", sessionID})
	if err != nil {
		return []commandResult{step}, err
	}
	return []commandResult{step}, nil
}

func (a *app) runOpenView(ctx context.Context, sessionID string) ([]commandResult, error) {
	step, err := a.execCommand(ctx, "open-view", []string{"--path", "/testapp/opened", "--new-tab", sessionID})
	if err != nil {
		return []commandResult{step}, err
	}
	return []commandResult{step}, nil
}

func (a *app) runCloseSession(ctx context.Context, sessionID string) ([]commandResult, error) {
	step, err := a.execCommand(ctx, "close-session", []string{sessionID})
	if err != nil {
		return []commandResult{step}, err
	}
	return []commandResult{step}, nil
}

func (a *app) runPostActivity(ctx context.Context, sessionID string) ([]commandResult, error) {
	step, err := a.execCommand(ctx, "post-activity", []string{
		"--path", "/testapp/activity",
		"--type", "0",
		"--thread-path", "/testapp/activity",
		"--thread-title", "sandstorm-utils testapp",
		"--caption", "Posted by sandstorm-utils testapp",
		sessionID,
	})
	if err != nil {
		return []commandResult{step}, err
	}
	return []commandResult{step}, nil
}

func (a *app) runGetSessionRequest(ctx context.Context, sessionID string) ([]commandResult, error) {
	step, err := a.execJSON(ctx, "get-session-request", []string{sessionID})
	if err != nil {
		return []commandResult{step}, err
	}
	return []commandResult{step}, nil
}

func (a *app) runGetSessionOffer(ctx context.Context, sessionID string) ([]commandResult, error) {
	step, err := a.execJSON(ctx, "get-session-offer", []string{sessionID})
	if err != nil {
		return []commandResult{step}, err
	}
	return []commandResult{step}, nil
}

func (a *app) runSendEmail(ctx context.Context, sessionID, recipient string) ([]commandResult, error) {
	if strings.TrimSpace(recipient) == "" {
		return nil, errors.New("recipient is required for send-email")
	}

	step, err := a.execCommand(ctx, "send-email", []string{
		"--to", recipient,
		"--subject", "sandstorm-utils testapp",
		"--text", "This is a test email from the sandstorm-utils integration harness.",
		sessionID,
	})
	if err != nil {
		return []commandResult{step}, err
	}
	return []commandResult{step}, nil
}

func (a *app) runStayAwake(ctx context.Context, sessionID string) ([]commandResult, error) {
	step, err := a.execCommand(ctx, "stay-awake", []string{
		"--for", "2s",
		"--title", "sandstorm-utils testapp",
		"--caption", "Running integration task",
		sessionID,
	})
	if err != nil {
		return []commandResult{step}, err
	}
	return []commandResult{step}, nil
}

func (a *app) execJSON(ctx context.Context, name string, args []string) (commandResult, error) {
	result, err := a.execCommand(ctx, name, args)
	if strings.TrimSpace(result.Stdout) != "" {
		var parsed any
		if parseErr := json.Unmarshal([]byte(result.Stdout), &parsed); parseErr == nil {
			result.ParsedJSON = parsed
		}
	}
	return result, err
}

func (a *app) execCommand(ctx context.Context, name string, args []string) (commandResult, error) {
	cmdPath := filepath.Join(a.binDir, name)
	start := time.Now()

	cmd := exec.CommandContext(ctx, cmdPath, args...)
	cmd.Env = os.Environ()
	output, err := cmd.Output()

	result := commandResult{
		Command:    name,
		Args:       append([]string(nil), args...),
		DurationMS: time.Since(start).Milliseconds(),
	}

	if len(output) > 0 {
		result.Stdout = string(output)
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.Stderr = string(exitErr.Stderr)
		result.ExitCode = exitCode(exitErr)
		return result, fmt.Errorf("%s failed with exit code %d", name, result.ExitCode)
	}

	if err != nil {
		result.ExitCode = -1
		result.Stderr = err.Error()
		return result, err
	}

	result.ExitCode = 0
	return result, nil
}

func (s *stateStore) record(run scenarioRun) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastRun[run.Name] = run
	s.history = append(s.history, run)
	if len(s.history) > 50 {
		s.history = append([]scenarioRun(nil), s.history[len(s.history)-50:]...)
	}
}

func (s *stateStore) snapshot() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()

	last := make(map[string]scenarioRun, len(s.lastRun))
	for name, run := range s.lastRun {
		last[name] = run
	}
	history := append([]scenarioRun(nil), s.history...)
	return map[string]any{
		"lastRun": last,
		"history": history,
	}
}

func scenarioTimeout(name string) time.Duration {
	if name == "stay-awake" {
		return 45 * time.Second
	}
	return 15 * time.Second
}

func resolveSessionID(r *http.Request, req runRequest) string {
	if value := strings.TrimSpace(req.SessionID); value != "" {
		return value
	}
	return strings.TrimSpace(r.Header.Get(sessionHeader))
}

func decodeRunRequest(r *http.Request) (runRequest, error) {
	if r.Body == nil || r.ContentLength == 0 {
		return runRequest{}, nil
	}

	defer r.Body.Close()
	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return runRequest{}, fmt.Errorf("decode request body: %w", err)
	}
	return req, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func exitCode(err *exec.ExitError) int {
	if err.ProcessState != nil {
		return err.ProcessState.ExitCode()
	}
	return 1
}

func parsedString(value any, key string) (string, bool) {
	m, ok := value.(map[string]any)
	if !ok {
		return "", false
	}
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

const indexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>sandstorm-utils testapp</title>
  <style>
    :root {
      color-scheme: light;
      --bg: #f3f3f0;
      --panel: #fbfbf8;
      --ink: #1e1f1c;
      --muted: #666a63;
      --accent: #2d5f4f;
      --accent-2: #8a4b36;
      --accent-3: #465b6b;
      --border: #d7d8d1;
      --ok: #3a6a47;
      --fail: #8d4333;
      --idle: #73776f;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "SFMono-Regular", "Menlo", "Monaco", "Liberation Mono", monospace;
      background: var(--bg);
      color: var(--ink);
    }
    main {
      max-width: 980px;
      margin: 0 auto;
      padding: 20px 16px 32px;
    }
    h1 {
      margin: 0 0 6px;
      font-size: 1.6rem;
      font-weight: 700;
      letter-spacing: 0.02em;
    }
    p, label { color: var(--muted); font-size: 0.95rem; }
    .grid {
      display: grid;
      gap: 10px;
      grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
      margin: 14px 0;
    }
    .panel {
      background: var(--panel);
      border: 1px solid var(--border);
      border-radius: 8px;
      padding: 12px;
    }
    .controls { display: flex; gap: 8px; flex-wrap: wrap; margin: 10px 0 0; }
    button {
      border: 0;
      border-radius: 6px;
      background: var(--accent);
      color: white;
      padding: 8px 10px;
      font: inherit;
      cursor: pointer;
      font-size: 0.9rem;
    }
    button.warn { background: var(--accent-2); }
    button.alt { background: var(--accent-3); }
    input {
      width: 100%;
      padding: 8px 10px;
      border-radius: 6px;
      border: 1px solid var(--border);
      font: inherit;
      margin-top: 4px;
      background: white;
    }
    pre {
      white-space: pre-wrap;
      word-break: break-word;
      background: #f0f0eb;
      border-radius: 6px;
      padding: 10px;
      border: 1px solid var(--border);
      min-height: 220px;
      overflow: auto;
      font-size: 0.83rem;
      line-height: 1.45;
    }
    .summary-grid {
      display: grid;
      gap: 10px;
      grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
      margin: 14px 0;
    }
    .scenario-card {
      background: var(--panel);
      border: 1px solid var(--border);
      border-radius: 8px;
      padding: 12px;
    }
    .scenario-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 10px;
      margin-bottom: 8px;
    }
    .badge {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      min-width: 56px;
      border-radius: 4px;
      padding: 4px 8px;
      color: white;
      font-size: 0.75rem;
      text-transform: uppercase;
      letter-spacing: 0.04em;
    }
    .badge.ok { background: var(--ok); }
    .badge.fail { background: var(--fail); }
    .badge.idle { background: var(--idle); }
    .steps {
      margin: 0;
      padding-left: 16px;
      color: var(--muted);
      font-size: 0.85rem;
    }
    .meta {
      color: var(--muted);
      font-size: 0.8rem;
      margin-top: 6px;
    }
    .toolbar {
      display: flex;
      gap: 8px;
      flex-wrap: wrap;
      margin: 8px 0;
    }
    .hidden { display: none; }
  </style>
</head>
<body>
  <main>
    <h1>sandstorm-utils testapp</h1>
    <p>Run the utilities inside a real Sandstorm grain and inspect the transcripts.</p>

    <section class="panel">
      <label for="sessionId">Manual session ID override</label>
      <input id="sessionId" placeholder="Uses X-Sandstorm-Session-Id automatically when available">
      <label for="recipient" style="display:block; margin-top: 12px;">Recipient for send-email</label>
      <input id="recipient" placeholder="user@example.com">
      <div class="controls">
        <button data-run-all>Run Safe Scenarios</button>
        <button data-refresh>Refresh Results</button>
        <button class="alt" data-toggle-raw>Toggle Raw JSON</button>
      </div>
    </section>

    <section class="summary-grid" id="summary"></section>

    <section class="grid">
      <div class="panel"><strong>Identity</strong><div class="controls"><button data-scenario="get-public-id">get-public-id</button><button data-scenario="get-user-address">get-user-address</button></div></div>
      <div class="panel"><strong>Session</strong><div class="controls"><button data-scenario="open-view">open-view</button><button class="warn" data-scenario="close-session">close-session</button></div></div>
      <div class="panel"><strong>Activity</strong><div class="controls"><button data-scenario="post-activity">post-activity</button><button data-scenario="get-session-request">get-session-request</button><button data-scenario="get-session-offer">get-session-offer</button></div></div>
      <div class="panel"><strong>Communication</strong><div class="controls"><button class="warn" data-scenario="send-email">send-email</button><button data-scenario="stay-awake">stay-awake</button></div></div>
    </section>

    <section class="panel">
      <strong>Results</strong>
      <div class="toolbar">
        <span id="statusLine" class="meta">Loading…</span>
      </div>
      <pre id="output">Loading…</pre>
    </section>
  </main>
  <script>
    const output = document.getElementById("output");
    const sessionInput = document.getElementById("sessionId");
    const recipientInput = document.getElementById("recipient");
    const summary = document.getElementById("summary");
    const statusLine = document.getElementById("statusLine");
    const rawToggle = document.querySelector("[data-toggle-raw]");
    let showRaw = true;

    const orderedScenarios = [
      "get-public-id",
      "get-user-address",
      "post-activity",
      "stay-awake",
      "get-session-request",
      "get-session-offer",
      "open-view",
      "send-email",
      "close-session",
    ];

    function escapeHtml(value) {
      return String(value)
        .replaceAll("&", "&amp;")
        .replaceAll("<", "&lt;")
        .replaceAll(">", "&gt;");
    }

    function renderSummary(json) {
      const lastRun = json?.scenarios?.lastRun || {};
      const cards = orderedScenarios.map((name) => {
        const run = lastRun[name];
        const badgeClass = !run ? "idle" : run.ok ? "ok" : "fail";
        const badgeText = !run ? "idle" : run.ok ? "pass" : "fail";
        const duration = run ? (new Date(run.endedAt) - new Date(run.startedAt)) : 0;
        const stepItems = run?.steps?.length
          ? run.steps.map((step) => "<li><code>" + escapeHtml(step.command) + "</code> <span>" + (step.exitCode === 0 ? "ok" : "exit " + step.exitCode) + "</span></li>").join("")
          : "<li>Not run yet</li>";
        const meta = run
          ? String(Math.max(0, duration)) + "ms" + (run.error ? " | " + escapeHtml(run.error) : "")
          : "No execution yet";
        return ""
          + "<article class=\"scenario-card\">"
          + "<div class=\"scenario-head\">"
          + "<strong>" + escapeHtml(name) + "</strong>"
          + "<span class=\"badge " + badgeClass + "\">" + badgeText + "</span>"
          + "</div>"
          + "<ol class=\"steps\">" + stepItems + "</ol>"
          + "<div class=\"meta\">" + meta + "</div>"
          + "</article>";
      }).join("");

      summary.innerHTML = cards;

      const history = json?.scenarios?.history || [];
      const passing = Object.values(lastRun).filter((run) => run && run.ok).length;
      const failing = Object.values(lastRun).filter((run) => run && !run.ok).length;
      statusLine.textContent = "Session " + (json.headerSessionId || "(none)") + " | " + passing + " passing | " + failing + " failing | " + history.length + " recorded runs";
    }

    async function refresh() {
      const res = await fetch("/api/results");
      const json = await res.json();
      renderSummary(json);
      output.textContent = JSON.stringify(json, null, 2);
      output.classList.toggle("hidden", !showRaw);
    }

    function body() {
      return JSON.stringify({
        sessionId: sessionInput.value.trim(),
        recipient: recipientInput.value.trim(),
      });
    }

    async function post(url) {
      statusLine.textContent = "Running…";
      output.textContent = "Running…";
      const res = await fetch(url, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: body(),
      });
      const text = await res.text();
      output.textContent = text;
      await refresh();
    }

    document.querySelector("[data-run-all]").addEventListener("click", () => post("/api/run-all"));
    document.querySelector("[data-refresh]").addEventListener("click", refresh);
    rawToggle.addEventListener("click", () => {
      showRaw = !showRaw;
      output.classList.toggle("hidden", !showRaw);
    });
    for (const button of document.querySelectorAll("[data-scenario]")) {
      button.addEventListener("click", () => post("/api/run/" + button.dataset.scenario));
    }

    refresh();
  </script>
</body>
</html>
`
