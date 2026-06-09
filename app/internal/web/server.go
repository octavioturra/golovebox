// Package web provides the embedded HTTP server for the golovebox web shell.
package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/user/golovebox/core"
	sandboxpkg "github.com/user/golovebox/sandbox"
)

//go:embed static
var staticFS embed.FS

// Server is the embedded HTTP server: health, terminal SSH, SFTP file explorer.
type Server struct {
	sb          core.Sandbox
	interactive *sandboxpkg.VM
	vmBootMu    interface{} // unused placeholder; boot tracking removed with DAG
}

// New creates a Server backed by a running VM.
func New(vm *sandboxpkg.VM) *Server {
	return &Server{
		sb:          vm,
		interactive: vm,
	}
}

// Start registers all routes and begins serving on addr.
// Blocks until ctx is cancelled.
func (s *Server) Start(ctx context.Context, addr string) error {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	r.Get("/", s.handleIndex)

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", s.handleHealth)
		r.Get("/vm/files", s.handleVMFiles)
		r.Get("/vm/file", s.handleVMFile)
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

func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

type healthResult struct {
	OK  bool   `json:"ok"`
	Msg string `json:"msg"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	res := healthResult{OK: s.sb.Ready(ctx), Msg: "vm ready"}
	if !res.OK {
		res.Msg = "vm not ready"
	}
	writeJSON(w, map[string]healthResult{"vm": res})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
