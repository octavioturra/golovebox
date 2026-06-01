// Package sandbox provides QEMU/SSH execution implementing core.Sandbox.
package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"github.com/user/golovebox/core"
	gossh "golang.org/x/crypto/ssh"
)

// Config holds SSH connection parameters needed to reach the sandbox VM.
type Config struct {
	Host    string
	Port    int
	User    string
	KeyPath string
}

// Sandbox implements core.Sandbox over SSH.
type Sandbox struct {
	mu   sync.Mutex
	idle []*gossh.Client
	cfg  Config
}

// New creates a Sandbox with the given SSH connection parameters.
func New(cfg Config) core.Sandbox {
	return &Sandbox{cfg: cfg}
}

// Ready returns true if an SSH connection can be established.
func (s *Sandbox) Ready(ctx context.Context) bool {
	c, err := s.acquire(ctx)
	if err != nil {
		return false
	}
	s.release(c)
	return true
}

// Exec runs cmd inside the sandbox and returns the output.
func (s *Sandbox) Exec(ctx context.Context, cmd string) (core.Output, error) {
	c, err := s.acquire(ctx)
	if err != nil {
		return core.Output{}, fmt.Errorf("sandbox: acquire: %w", err)
	}
	defer s.release(c)

	session, err := c.NewSession()
	if err != nil {
		return core.Output{}, fmt.Errorf("sandbox: new session: %w", err)
	}
	defer session.Close()

	var outBuf, errBuf bytes.Buffer
	session.Stdout = &outBuf
	session.Stderr = &errBuf
	runErr := session.Run(cmd)
	out := core.Output{
		Stdout: outBuf.String(),
		Stderr: errBuf.String(),
	}
	if exitErr, ok := runErr.(*gossh.ExitError); ok {
		out.ExitCode = exitErr.ExitStatus()
	} else if runErr != nil {
		return out, runErr
	}
	return out, nil
}

// PutFile writes data to path inside the sandbox.
func (s *Sandbox) PutFile(ctx context.Context, path string, data []byte) error {
	c, err := s.acquire(ctx)
	if err != nil {
		return fmt.Errorf("sandbox: acquire: %w", err)
	}
	defer s.release(c)

	sc, err := sftp.NewClient(c)
	if err != nil {
		return fmt.Errorf("sandbox: sftp: %w", err)
	}
	defer sc.Close()

	if err := sc.MkdirAll(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sandbox: mkdir %s: %w", filepath.Dir(path), err)
	}
	f, err := sc.Create(path)
	if err != nil {
		return fmt.Errorf("sandbox: sftp create %s: %w", path, err)
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

func (s *Sandbox) acquire(ctx context.Context) (*gossh.Client, error) {
	s.mu.Lock()
	if len(s.idle) > 0 {
		c := s.idle[len(s.idle)-1]
		s.idle = s.idle[:len(s.idle)-1]
		s.mu.Unlock()
		_, _, err := c.SendRequest("keepalive@openssh.com", true, nil)
		if err == nil {
			return c, nil
		}
		_ = c.Close()
	} else {
		s.mu.Unlock()
	}

	var lastErr error
	for attempt := 1; attempt <= 4; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		c, err := dial(s.cfg.Host, strconv.Itoa(s.cfg.Port), s.cfg.User, s.cfg.KeyPath)
		if err == nil {
			return c, nil
		}
		lastErr = err
		if attempt < 4 {
			time.Sleep(time.Duration(attempt) * 400 * time.Millisecond)
		}
	}
	return nil, lastErr
}

func (s *Sandbox) release(c *gossh.Client) {
	s.mu.Lock()
	s.idle = append(s.idle, c)
	s.mu.Unlock()
}

func dial(host, port, user, keyPath string) (*gossh.Client, error) {
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read key %s: %w", keyPath, err)
	}
	signer, err := gossh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	cfg := &gossh.ClientConfig{
		User:            user,
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(signer)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), //nolint:gosec
	}
	return gossh.Dial("tcp", net.JoinHostPort(host, port), cfg)
}

// ReadFile retrieves a file from the sandbox via SFTP.
func ReadFile(c *gossh.Client, remotePath string) ([]byte, error) {
	sc, err := sftp.NewClient(c)
	if err != nil {
		return nil, fmt.Errorf("sftp client: %w", err)
	}
	defer sc.Close()
	f, err := sc.Open(remotePath)
	if err != nil {
		return nil, fmt.Errorf("sftp open %s: %w", remotePath, err)
	}
	defer f.Close()
	return io.ReadAll(f)
}
