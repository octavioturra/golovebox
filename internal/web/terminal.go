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
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
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
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws terminal upgrade", "error", err)
		return
	}
	defer conn.Close()

	// Retry SSH dial up to 3× with 500ms backoff — handles stale pool connections
	// and race between browser connect and VM finishing boot.
	var sshConn *ssh.Client
	for attempt := 1; attempt <= 3; attempt++ {
		sshConn, err = s.gw.AcquireSSH(r.Context())
		if err == nil {
			break
		}
		if attempt < 3 {
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
		}
	}
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("erro: VM não disponível\r\n"))
		return
	}
	defer s.gw.ReleaseSSH(sshConn)

	session, err := sshConn.NewSession()
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("erro: %v\r\n", err)))
		return
	}
	defer session.Close()

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", 24, 80, modes); err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("erro pty: %v\r\n", err)))
		return
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		return
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return
	}

	if err := session.Start("/bin/sh"); err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("erro shell: %v\r\n", err)))
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// SSH stdout → WebSocket
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
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
			n, err := stderr.Read(buf)
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
		Cols uint32 `json:"cols"`
		Rows uint32 `json:"rows"`
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
			_ = session.WindowChange(int(rm.Rows), int(rm.Cols))
			continue
		}

		if _, err := stdin.Write(msg); err != nil {
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

	sshConn, err := s.gw.AcquireSSH(r.Context())
	if err != nil {
		http.Error(w, "vm unavailable", http.StatusServiceUnavailable)
		return
	}
	defer s.gw.ReleaseSSH(sshConn)

	client, err := sftp.NewClient(sshConn)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer client.Close()

	infos, err := client.ReadDir(path)
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

	sshConn, err := s.gw.AcquireSSH(r.Context())
	if err != nil {
		http.Error(w, "vm unavailable", http.StatusServiceUnavailable)
		return
	}
	defer s.gw.ReleaseSSH(sshConn)

	client, err := sftp.NewClient(sshConn)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer client.Close()

	f, err := client.Open(path)
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
