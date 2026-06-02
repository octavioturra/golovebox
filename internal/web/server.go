// Package web provides the embedded HTTP server for the golovebox web chat UI.
package web

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/user/golovebox/core"
	"github.com/user/golovebox/internal/config"
	"github.com/user/golovebox/internal/gateway"
	"github.com/user/golovebox/promptlang"
	"github.com/user/golovebox/internal/llm"
	"github.com/user/golovebox/orchestrator"
	"github.com/user/golovebox/orchestrator/dag"
	sandboxpkg "github.com/user/golovebox/sandbox"
	"github.com/user/golovebox/toolskills"
)

//go:embed static
var staticFS embed.FS

// sseClient holds the channel through which SSE events are delivered.
type sseClient struct {
	ch chan string
}

// Server is the embedded HTTP server for web chat and DAG visualisation.
type Server struct {
	gw              *gateway.Gateway
	sb              core.Sandbox
	interactive     *sandboxpkg.VM
	engine          *orchestrator.Engine
	skillsReg       *toolskills.Registry
	llmClient       *llm.Client
	store           *RunStore
	cfg             *config.Config
	mu              sync.RWMutex
	sseClients      map[string][]*sseClient
	activeRuns      map[string]*dag.DAG
	vmBootMu        sync.Mutex
	vmBootStartedAt time.Time // time of first health check that saw vm.ok=false; zeroed when vm.ok=true
}

// New creates the Server wiring all components together.
func New(gw *gateway.Gateway, vm *sandboxpkg.VM, engine *orchestrator.Engine, reg *toolskills.Registry, llmClient *llm.Client, cfg *config.Config, runsDir string) (*Server, error) {
	store, err := NewRunStore(runsDir)
	if err != nil {
		return nil, err
	}
	return &Server{
		gw:          gw,
		sb:          vm,
		interactive: vm,
		engine:      engine,
		skillsReg:   reg,
		llmClient:   llmClient,
		store:       store,
		cfg:         cfg,
		sseClients:  make(map[string][]*sseClient),
		activeRuns:  make(map[string]*dag.DAG),
	}, nil
}

// runConfig builds the per-run core.RunConfig from the server's config and the run dir.
func (s *Server) runConfig(runDir string) core.RunConfig {
	return core.RunConfig{
		Repo:          resolveRepo(s.cfg),
		DefaultBranch: s.cfg.Workflow.DefaultBranch,
		GitHubToken:   s.cfg.GitHubToken,
		WorkDir:       runDir,
		ClonePath:     s.cfg.Workflow.ClonePath,
		RunMode:       s.cfg.Workflow.RunMode,
	}
}

// Start registers all routes and begins serving on addr.
// Blocks until ctx is cancelled.
func (s *Server) Start(ctx context.Context, addr string) error {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	r.Get("/", s.handleIndex)

	r.Route("/api", func(r chi.Router) {
		r.Get("/status", s.handleStatus)
		r.Get("/health", s.handleHealth)
		r.Post("/run", s.handleRun)
		r.Get("/runs", s.handleListRuns)

		r.Route("/runs/{id}", func(r chi.Router) {
			r.Get("/", s.handleGetRun)
			r.Get("/stream", s.handleStream)
			r.Post("/stop", s.handleStop)
			r.Post("/approve/{nodeID}", s.handleApprove)
			r.Post("/reject/{nodeID}", s.handleReject)
			r.Get("/nodes/{nodeID}/log", s.handleNodeLog)
		})

		r.Route("/skills", func(r chi.Router) {
			r.Get("/", s.handleListSkills)
			r.Post("/generate", s.handleGenerateSkill)
		})

		r.Get("/vm/files", s.handleVMFiles)
		r.Get("/vm/file",  s.handleVMFile)
	})

	r.Get("/ws/terminal", s.handleTerminal)

	srv := &http.Server{Addr: addr, Handler: r}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("web: listen %s: %w", addr, err)
	}

	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()

	return srv.Serve(ln)
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	active := len(s.activeRuns)
	s.mu.RUnlock()
	writeJSON(w, map[string]any{
		"vm_running":    true,
		"active_agents": active,
	})
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	// Accept JSON body: {"task": "..."}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			Task string `json:"task"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Task == "" {
			http.Error(w, "task required", http.StatusBadRequest)
			return
		}
		s.startRunFromTask(w, r.Context(), body.Task)
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "bad multipart: "+err.Error(), http.StatusBadRequest)
		return
	}

	runID, err := s.store.NewRun()
	if err != nil {
		http.Error(w, "create run: "+err.Error(), http.StatusInternalServerError)
		return
	}

	for _, fh := range r.MultipartForm.File["spec"] {
		f, err := fh.Open()
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(f)
		f.Close()
		_ = s.store.SaveSpec(runID, fh.Filename, data)
	}

	parsed, err := promptlang.ParseDir(s.store.SpecsDir(runID))
	if err != nil || len(parsed) == 0 {
		http.Error(w, "no valid .md spec files found", http.StatusBadRequest)
		return
	}
	intents := make([]core.Intent, len(parsed))
	for i, p := range parsed {
		intents[i] = promptlang.ToIntent(p)
	}

	s.launchRun(w, r.Context(), runID, intents)
}

func (s *Server) startRunFromTask(w http.ResponseWriter, ctx context.Context, task string) {
	runID, err := s.store.NewRun()
	if err != nil {
		http.Error(w, "create run: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Persist the task text for display purposes.
	_ = os.WriteFile(filepath.Join(s.store.RunDir(runID), "task.txt"), []byte(task), 0o644)

	// Wrap free-form task as a minimal spec file so the orchestrator can plan it.
	content := "# Task\n\n" + task + "\n"
	_ = s.store.SaveSpec(runID, "task.md", []byte(content))

	parsed, err := promptlang.ParseDir(s.store.SpecsDir(runID))
	if err != nil || len(parsed) == 0 {
		// Fallback: treat raw text as single-node task directly.
		s.launchSingleTask(w, ctx, runID, task)
		return
	}
	intents := make([]core.Intent, len(parsed))
	for i, p := range parsed {
		intents[i] = promptlang.ToIntent(p)
	}
	s.launchRun(w, ctx, runID, intents)
}

func (s *Server) launchSingleTask(w http.ResponseWriter, ctx context.Context, runID, task string) {
	runDir := s.store.RunDir(runID)
	nodeID := "task-0"
	nodelog := s.store.NodeLogFor(runID, nodeID)

	runCtx, cancel := context.WithCancel(context.Background())
	s.store.RegisterRun(runID, cancel)

	s.mu.Lock()
	s.activeRuns[runID] = nil
	s.mu.Unlock()

	go func() {
		defer func() {
			cancel()
			s.store.UnregisterRun(runID)
			s.mu.Lock()
			delete(s.activeRuns, runID)
			s.mu.Unlock()
		}()
		_, _ = s.engine.RunSingleTask(runCtx, task, s.runConfig(runDir), func(_ string, iter int, action, params, obs, prompt, reply string) {
			nodelog.Append(iter, action, params, obs, prompt, reply)
			s.broadcastNodeLog(runID, nodeID, NodeLogEntry{iter, action, params, obs, prompt, reply, time.Now()})
			_ = appendFile(filepath.Join(runDir, "logs", nodeID+".log"),
				fmt.Sprintf("[%d] %s: %s\n", iter, action, obs))
		})
	}()

	writeJSON(w, map[string]string{"run_id": runID})
}

func (s *Server) launchRun(w http.ResponseWriter, ctx context.Context, runID string, intents []core.Intent) {
	runDir := s.store.RunDir(runID)
	rc := s.runConfig(runDir)
	d, err := s.engine.Plan(ctx, runID, intents, rc)
	if err != nil {
		http.Error(w, "plan: "+err.Error(), http.StatusInternalServerError)
		return
	}

	runCtx, cancel := context.WithCancel(context.Background())
	s.store.RegisterRun(runID, cancel)

	s.mu.Lock()
	s.activeRuns[runID] = d
	s.mu.Unlock()

	// notify observes node state transitions (SSE broadcast of the whole node).
	notify := func(node *dag.Node) {
		data, _ := json.Marshal(node)
		s.broadcast(runID, string(data))
	}

	// prog records each ReAct iteration (NodeLog + node_log SSE) for task nodes.
	prog := func(nodeID string, iter int, action, params, obs, prompt, reply string) {
		nodelog := s.store.NodeLogFor(runID, nodeID)
		nodelog.Append(iter, action, params, obs, prompt, reply)
		s.broadcastNodeLog(runID, nodeID, NodeLogEntry{iter, action, params, obs, prompt, reply, time.Now()})
		_ = appendFile(filepath.Join(runDir, "logs", nodeID+".log"),
			fmt.Sprintf("[%d] %s: %s\n", iter, action, obs))
	}

	go func() {
		defer func() {
			cancel()
			s.store.UnregisterRun(runID)
			s.mu.Lock()
			delete(s.activeRuns, runID)
			s.mu.Unlock()
		}()
		if err := s.engine.Run(runCtx, d, rc, notify, prog); err != nil {
			slog.Warn("run finished with error", "run_id", runID, "error", err)
		}
	}()

	writeJSON(w, map[string]string{"run_id": runID})
}

func (s *Server) handleListRuns(w http.ResponseWriter, _ *http.Request) {
	runs, _ := s.store.ListRuns()
	if runs == nil {
		runs = []RunMeta{}
	}
	writeJSON(w, runs)
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")

	// If the run directory doesn't exist at all, it's a genuine 404.
	if _, err := os.Stat(s.store.RunDir(runID)); os.IsNotExist(err) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// dag.json may be absent for runs that were aborted during planning.
	// Return a valid response with a null dag so the frontend degrades gracefully.
	d, err := s.store.LoadDAG(runID)
	var dagJSON json.RawMessage
	if err == nil {
		dagJSON, _ = d.ToJSON()
	}

	statesData, _ := os.ReadFile(filepath.Join(s.store.RunDir(runID), "node_states.json"))
	var nodeStates any
	_ = json.Unmarshal(statesData, &nodeStates)

	writeJSON(w, map[string]any{
		"dag":            dagJSON,
		"node_states":    nodeStates,
		"task":           s.store.ReadTask(runID),
		"current_branch": s.store.GetRunMeta(runID, "current_branch"),
	})
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	client := &sseClient{ch: make(chan string, 32)}
	s.mu.Lock()
	s.sseClients[runID] = append(s.sseClients[runID], client)
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		clients := s.sseClients[runID]
		for i, c := range clients {
			if c == client {
				s.sseClients[runID] = append(clients[:i], clients[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
	}()

	for {
		select {
		case msg := <-client.ch:
			_, _ = fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	ok := s.store.CancelRun(runID)
	writeJSON(w, map[string]bool{"ok": ok})
}

func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	s.engine.Approve(chi.URLParam(r, "nodeID"))
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleReject(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "nodeID")
	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Reason == "" {
		body.Reason = "rejeitado pelo usuário"
	}
	s.engine.Reject(nodeID, body.Reason)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleListSkills(w http.ResponseWriter, _ *http.Request) {
	list := s.skillsReg.List()
	if list == nil {
		list = []*toolskills.Skill{}
	}
	writeJSON(w, list)
}

func (s *Server) handleGenerateSkill(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Description == "" {
		http.Error(w, "description required", http.StatusBadRequest)
		return
	}
	skillsDir, err := s.cfg.SkillsDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sk, err := toolskills.Generate(r.Context(), llm.NewCompleter(s.llmClient), body.Description, skillsDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.skillsReg.Reload()
	writeJSON(w, sk)
}

// ── New handlers ──────────────────────────────────────────────────────────────

type healthResult struct {
	OK              bool   `json:"ok"`
	Msg             string `json:"msg"`
	BootElapsedSecs int    `json:"boot_elapsed_secs,omitempty"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	type result struct {
		key string
		res healthResult
	}
	ch := make(chan result, 4)

	checks := []struct {
		key string
		fn  func(context.Context) healthResult
	}{
		{"vm", s.healthVM},
		{"llm", s.healthLLM},
		{"github", s.healthGitHub},
		{"repo", s.healthRepo},
	}

	for _, c := range checks {
		go func(key string, fn func(context.Context) healthResult) {
			ch <- result{key, fn(ctx)}
		}(c.key, c.fn)
	}

	out := make(map[string]healthResult, 4)
	for range checks {
		r := <-ch
		out[r.key] = r.res
	}

	// Track VM boot duration so the UI can show "VM 0:23 ..." while warming up.
	now := time.Now()
	vm := out["vm"]
	s.vmBootMu.Lock()
	switch {
	case vm.OK:
		s.vmBootStartedAt = time.Time{}
	case s.vmBootStartedAt.IsZero():
		s.vmBootStartedAt = now
	default:
		vm.BootElapsedSecs = int(now.Sub(s.vmBootStartedAt).Seconds())
		out["vm"] = vm
	}
	s.vmBootMu.Unlock()

	writeJSON(w, out)
}

// healthHTTP is a shared HTTP client for health checks — 4s timeout, respects context.
var healthHTTP = &http.Client{Timeout: 4 * time.Second}

func (s *Server) healthVM(ctx context.Context) healthResult {
	out, err := s.gw.HealthCheckVM(ctx)
	if err != nil {
		return healthResult{OK: false, Msg:err.Error()}
	}
	return healthResult{OK: true, Msg:out}
}

func (s *Server) healthLLM(ctx context.Context) healthResult {
	if s.cfg.LLMBaseURL == "" {
		return healthResult{OK: false, Msg:"LLMBaseURL não configurado"}
	}
	payload, _ := json.Marshal(map[string]any{
		"model":      s.cfg.LLMModel,
		"max_tokens": 1,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
	})
	req, err := http.NewRequestWithContext(ctx, "POST", s.cfg.LLMBaseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return healthResult{OK: false, Msg:err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", s.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	t0 := time.Now()
	resp, err := healthHTTP.Do(req)
	if err != nil {
		return healthResult{OK: false, Msg:err.Error()}
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		return healthResult{OK: false, Msg:fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	return healthResult{OK: true, Msg:fmt.Sprintf("%s respondeu em %dms", s.cfg.LLMModel, time.Since(t0).Milliseconds())}
}

func (s *Server) healthGitHub(ctx context.Context) healthResult {
	if s.cfg.GitHubToken == "" {
		return healthResult{OK: false, Msg:"GitHubToken não configurado"}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
	if err != nil {
		return healthResult{OK: false, Msg:err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.GitHubToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := healthHTTP.Do(req)
	if err != nil {
		return healthResult{OK: false, Msg:err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return healthResult{OK: false, Msg:fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	var u struct {
		Login string `json:"login"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&u)
	return healthResult{OK: true, Msg:"autenticado como " + u.Login}
}

func (s *Server) healthRepo(ctx context.Context) healthResult {
	if s.cfg.DefaultRepo == "" {
		return healthResult{OK: true, Msg:"DefaultRepo não configurado (ok)"}
	}
	url := "https://api.github.com/repos/" + s.cfg.DefaultRepo
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return healthResult{OK: false, Msg:err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.GitHubToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := healthHTTP.Do(req)
	if err != nil {
		return healthResult{OK: false, Msg:err.Error()}
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return healthResult{OK: false, Msg:fmt.Sprintf("%s: HTTP %d", s.cfg.DefaultRepo, resp.StatusCode)}
	}
	return healthResult{OK: true, Msg:s.cfg.DefaultRepo + ": acessível"}
}

func (s *Server) handleNodeLog(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	nodeID := chi.URLParam(r, "nodeID")
	entries := s.store.GetNodeLog(runID, nodeID)
	writeJSON(w, entries)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (s *Server) broadcast(runID, msg string) {
	s.mu.RLock()
	clients := append([]*sseClient(nil), s.sseClients[runID]...)
	s.mu.RUnlock()
	for _, c := range clients {
		select {
		case c.ch <- msg:
		default:
		}
	}
}

// broadcastNodeLog emits a node_log SSE event for a single ReAct iteration.
func (s *Server) broadcastNodeLog(runID, nodeID string, entry NodeLogEntry) {
	data, _ := json.Marshal(map[string]any{
		"type":    "node_log",
		"node_id": nodeID,
		"iter":    entry.Iteration,
		"action":  entry.Action,
		"params":  entry.Params,
		"obs":     entry.Observation,
		"prompt":  entry.Prompt,
		"reply":   entry.Reply,
		"ts":      entry.Timestamp,
	})
	s.broadcast(runID, string(data))
}

// resolveRepo returns workflow.default_repo with fallback to top-level default_repo.
func resolveRepo(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if cfg.Workflow.DefaultRepo != "" {
		return cfg.Workflow.DefaultRepo
	}
	return cfg.DefaultRepo
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func appendFile(path, data string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(data)
	return err
}
