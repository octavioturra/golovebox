// Package web provides the embedded HTTP server for the golovebox web chat UI.
package web

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/user/golovebox/internal/config"
	"github.com/user/golovebox/internal/dag"
	"github.com/user/golovebox/internal/dsl"
	"github.com/user/golovebox/internal/gateway"
	"github.com/user/golovebox/internal/llm"
	"github.com/user/golovebox/internal/orchestrator"
	"github.com/user/golovebox/internal/skills"
)

//go:embed static
var staticFS embed.FS

// sseClient holds the channel through which SSE events are delivered.
type sseClient struct {
	ch chan string
}

// Server is the embedded HTTP server for web chat and DAG visualisation.
type Server struct {
	gw         *gateway.Gateway
	orch       *orchestrator.Orchestrator
	checkpoint *dag.CheckpointManager
	skillsReg  *skills.Registry
	llmClient  *llm.Client
	store      *RunStore
	cfg        *config.Config
	mu         sync.RWMutex
	sseClients map[string][]*sseClient
	activeRuns map[string]*dag.DAG
}

// New creates the Server wiring all components together.
func New(gw *gateway.Gateway, orch *orchestrator.Orchestrator, cm *dag.CheckpointManager, reg *skills.Registry, llmClient *llm.Client, cfg *config.Config, runsDir string) (*Server, error) {
	store, err := NewRunStore(runsDir)
	if err != nil {
		return nil, err
	}
	return &Server{
		gw:         gw,
		orch:       orch,
		checkpoint: cm,
		skillsReg:  reg,
		llmClient:  llmClient,
		store:      store,
		cfg:        cfg,
		sseClients: make(map[string][]*sseClient),
		activeRuns: make(map[string]*dag.DAG),
	}, nil
}

// Start registers all routes and begins serving on addr.
// Blocks until ctx is cancelled.
func (s *Server) Start(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("POST /api/run", s.handleRun)
	mux.HandleFunc("GET /api/runs", s.handleListRuns)
	mux.HandleFunc("GET /api/runs/{id}", s.handleGetRun)
	mux.HandleFunc("GET /api/runs/{id}/stream", s.handleStream)
	mux.HandleFunc("POST /api/runs/{id}/approve/{nodeID}", s.handleApprove)
	mux.HandleFunc("POST /api/runs/{id}/reject/{nodeID}", s.handleReject)
	mux.HandleFunc("GET /api/skills", s.handleListSkills)
	mux.HandleFunc("POST /api/skills/generate", s.handleGenerateSkill)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/runs/{id}/nodes/{nodeID}/log", s.handleNodeLog)

	srv := &http.Server{Addr: addr, Handler: mux}

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

	specFiles, err := dsl.ParseDir(s.store.SpecsDir(runID))
	if err != nil || len(specFiles) == 0 {
		http.Error(w, "no valid .md spec files found", http.StatusBadRequest)
		return
	}

	s.launchRun(w, r.Context(), runID, specFiles)
}

func (s *Server) startRunFromTask(w http.ResponseWriter, ctx context.Context, task string) {
	runID, err := s.store.NewRun()
	if err != nil {
		http.Error(w, "create run: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Wrap free-form task as a minimal spec file so the orchestrator can plan it.
	content := "# Task\n\n" + task + "\n"
	_ = s.store.SaveSpec(runID, "task.md", []byte(content))

	specFiles, err := dsl.ParseDir(s.store.SpecsDir(runID))
	if err != nil || len(specFiles) == 0 {
		// Fallback: treat raw text as single-node task directly.
		s.launchSingleTask(w, ctx, runID, task)
		return
	}
	s.launchRun(w, ctx, runID, specFiles)
}

func (s *Server) launchSingleTask(w http.ResponseWriter, ctx context.Context, runID, task string) {
	runDir := s.store.RunDir(runID)
	nodeID := "task-0"
	nodelog := s.store.NodeLogFor(runID, nodeID)

	s.mu.Lock()
	s.activeRuns[runID] = nil
	s.mu.Unlock()

	go func() {
		_, _ = s.gw.RunSpecTask(context.Background(), task, func(iter int, action, obs string) {
			nodelog.Append(iter, action, obs)
			s.broadcastNodeLog(runID, nodeID, NodeLogEntry{iter, action, obs, time.Now()})
			_ = appendFile(filepath.Join(runDir, "logs", nodeID+".log"),
				fmt.Sprintf("[%d] %s: %s\n", iter, action, obs))
		})
		s.mu.Lock()
		delete(s.activeRuns, runID)
		s.mu.Unlock()
	}()

	writeJSON(w, map[string]string{"run_id": runID})
}

func (s *Server) launchRun(w http.ResponseWriter, ctx context.Context, runID string, specFiles []*dsl.ParsedSpec) {
	runDir := s.store.RunDir(runID)
	d, err := s.orch.Plan(ctx, runID, specFiles, runDir)
	if err != nil {
		http.Error(w, "plan: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.mu.Lock()
	s.activeRuns[runID] = d
	s.mu.Unlock()

	notify := func(node *dag.Node) {
		data, _ := json.Marshal(node)
		s.broadcast(runID, string(data))
	}

	dispatch := func(dCtx context.Context, node *dag.Node) (string, error) {
		nodelog := s.store.NodeLogFor(runID, node.ID)
		logPath := filepath.Join(runDir, "logs", node.ID+".log")
		return s.gw.RunSpecTask(dCtx, node.Task, func(iter int, action, obs string) {
			nodelog.Append(iter, action, obs)
			s.broadcastNodeLog(runID, node.ID, NodeLogEntry{iter, action, obs, time.Now()})
			_ = appendFile(logPath, fmt.Sprintf("[%d] %s: %s\n", iter, action, obs))
		})
	}

	exec := dag.NewExecutor(d, dag.ExecutorConfig{}, dispatch, notify, s.checkpoint, runDir)
	go func() {
		_ = exec.Run(context.Background())
		s.mu.Lock()
		delete(s.activeRuns, runID)
		s.mu.Unlock()
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
	runID := r.PathValue("id")
	d, err := s.store.LoadDAG(runID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	dagJSON, _ := d.ToJSON()

	statesData, _ := os.ReadFile(filepath.Join(s.store.RunDir(runID), "node_states.json"))
	var nodeStates any
	_ = json.Unmarshal(statesData, &nodeStates)

	writeJSON(w, map[string]any{
		"dag":         json.RawMessage(dagJSON),
		"node_states": nodeStates,
	})
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")

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

func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	s.checkpoint.Approve(r.PathValue("nodeID"))
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleReject(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("nodeID")
	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Reason == "" {
		body.Reason = "rejeitado pelo usuário"
	}
	s.checkpoint.Reject(nodeID, body.Reason)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleListSkills(w http.ResponseWriter, _ *http.Request) {
	list := s.skillsReg.List()
	if list == nil {
		list = []*skills.Skill{}
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
	sk, err := skills.Generate(r.Context(), s.llmClient, body.Description, skillsDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.skillsReg.Reload()
	writeJSON(w, sk)
}

// ── New handlers ──────────────────────────────────────────────────────────────

type healthResult struct {
	OK  bool   `json:"ok"`
	Msg string `json:"msg"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
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
	writeJSON(w, out)
}

func (s *Server) healthVM(ctx context.Context) healthResult {
	out, err := s.gw.HealthCheckVM(ctx)
	if err != nil {
		return healthResult{false, err.Error()}
	}
	return healthResult{true, out}
}

func (s *Server) healthLLM(ctx context.Context) healthResult {
	if s.cfg.LLMBaseURL == "" {
		return healthResult{false, "LLMBaseURL não configurado"}
	}
	payload, _ := json.Marshal(map[string]any{
		"model":      s.cfg.LLMModel,
		"max_tokens": 1,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
	})
	req, err := http.NewRequestWithContext(ctx, "POST", s.cfg.LLMBaseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return healthResult{false, err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", s.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	t0 := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return healthResult{false, err.Error()}
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode != 400 {
		return healthResult{false, fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	return healthResult{true, fmt.Sprintf("%s respondeu em %dms", s.cfg.LLMModel, time.Since(t0).Milliseconds())}
}

func (s *Server) healthGitHub(ctx context.Context) healthResult {
	if s.cfg.GitHubToken == "" {
		return healthResult{false, "GitHubToken não configurado"}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
	if err != nil {
		return healthResult{false, err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.GitHubToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return healthResult{false, err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return healthResult{false, fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	var u struct {
		Login string `json:"login"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&u)
	return healthResult{true, "autenticado como " + u.Login}
}

func (s *Server) healthRepo(ctx context.Context) healthResult {
	if s.cfg.DefaultRepo == "" {
		return healthResult{true, "DefaultRepo não configurado (ok)"}
	}
	url := "https://api.github.com/repos/" + s.cfg.DefaultRepo
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return healthResult{false, err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.GitHubToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return healthResult{false, err.Error()}
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return healthResult{false, fmt.Sprintf("%s: HTTP %d", s.cfg.DefaultRepo, resp.StatusCode)}
	}
	return healthResult{true, s.cfg.DefaultRepo + ": acessível"}
}

func (s *Server) handleNodeLog(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	nodeID := r.PathValue("nodeID")

	// Try in-memory buffer first; fall back to disk for historical runs.
	key := runID + "/" + nodeID
	s.store.logsMu.Lock()
	nl, exists := s.store.nodeLogs[key]
	s.store.logsMu.Unlock()

	if exists {
		entries := nl.Entries()
		if entries == nil {
			entries = []NodeLogEntry{}
		}
		writeJSON(w, entries)
		return
	}

	// Not in memory — read from disk (.jsonl).
	entries, err := loadNodeLogFromDisk(nodeID, s.store.RunDir(runID))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []NodeLogEntry{}
	}
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
		"obs":     entry.Observation,
		"ts":      entry.Timestamp,
	})
	s.broadcast(runID, string(data))
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
