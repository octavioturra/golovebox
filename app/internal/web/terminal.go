package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	sandboxpkg "github.com/user/golovebox/sandbox"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// FileEntry describes one entry returned by the SFTP file explorer.
type FileEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
	Mode  string `json:"mode"`
}

// handleTerminal bridges a WebSocket connection to an SSH PTY on the VM.
func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	// Wait up to ~5s for the VM to be ready before upgrading — avoids the
	// "conectado → desconectado em 1s" race when the page opens right after boot.
	for _, d := range []time.Duration{0, 250, 500, 1000, 2000} {
		if d > 0 {
			time.Sleep(d * time.Millisecond)
		}
		if s.gw.IsVMReady() {
			break
		}
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws terminal upgrade", "error", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Retry PTY open up to 3× with 500ms backoff.
	var pty *sandboxpkg.PTY
	var ptyErr error
	for attempt := 1; attempt <= 3; attempt++ {
		pty, ptyErr = s.interactive.OpenPTY(ctx, 80, 24)
		if ptyErr == nil {
			break
		}
		if attempt < 3 {
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
		}
	}
	if ptyErr != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("erro: VM não disponível\r\n"))
		return
	}
	defer pty.Close()

	// SSH stdout → WebSocket
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := pty.Read(buf)
			if n > 0 {
				if werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					cancel()
					return
				}
			}
			if err != nil {
				cancel()
				return
			}
		}
	}()

	// SSH stderr → WebSocket
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := pty.ReadStderr(buf)
			if n > 0 {
				_ = conn.WriteMessage(websocket.BinaryMessage, buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	// WebSocket → SSH stdin (+ resize messages)
	type resizeMsg struct {
		Type string `json:"type"`
		Cols int    `json:"cols"`
		Rows int    `json:"rows"`
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var rm resizeMsg
		if json.Unmarshal(msg, &rm) == nil && rm.Type == "resize" {
			_ = pty.WindowChange(rm.Rows, rm.Cols)
			continue
		}

		if err := pty.Write(msg); err != nil {
			return
		}
	}
}

// handleVMFiles lists a directory on the VM via SFTP.
func (s *Server) handleVMFiles(w http.ResponseWriter, r *http.Request) {
	if !s.gw.IsVMReady() {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"vm_not_ready"}`, http.StatusServiceUnavailable)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/root"
	}

	sftpConn, err := s.interactive.OpenSFTP(r.Context())
	if err != nil {
		http.Error(w, "vm unavailable", http.StatusServiceUnavailable)
		return
	}
	defer sftpConn.Close()

	infos, err := sftpConn.ReadDir(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	entries := make([]FileEntry, 0, len(infos))
	for _, fi := range infos {
		entries = append(entries, FileEntry{
			Name:  fi.Name(),
			Path:  path + "/" + fi.Name(),
			IsDir: fi.IsDir(),
			Size:  fi.Size(),
			Mode:  fi.Mode().String(),
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})

	writeJSON(w, entries)
}

// handleVMFile streams a single file from the VM via SFTP.
func (s *Server) handleVMFile(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}

	sftpConn, err := s.interactive.OpenSFTP(r.Context())
	if err != nil {
		http.Error(w, "vm unavailable", http.StatusServiceUnavailable)
		return
	}
	defer sftpConn.Close()

	f, err := sftpConn.Open(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer f.Close()

	stat, _ := f.Stat()

	ext := strings.ToLower(filepath.Ext(path))
	ctMap := map[string]string{
		".go":   "text/plain; charset=utf-8",
		".sh":   "text/plain; charset=utf-8",
		".md":   "text/plain; charset=utf-8",
		".json": "application/json; charset=utf-8",
		".toml": "text/plain; charset=utf-8",
		".log":  "text/plain; charset=utf-8",
		".txt":  "text/plain; charset=utf-8",
		".yaml": "text/plain; charset=utf-8",
		".yml":  "text/plain; charset=utf-8",
	}
	ct, ok := ctMap[ext]
	if !ok {
		ct = "application/octet-stream"
	}

	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filepath.Base(path)))
	if stat != nil {
		w.Header().Set("Content-Length", fmt.Sprint(stat.Size()))
	}
	_, _ = io.Copy(w, f)
}
