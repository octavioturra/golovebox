package sandbox

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Interactive exposes PTY and SFTP capabilities consumed only by the web layer.
// Kept out of core.Sandbox — PTY/SFTP are substrate details, not domain contracts.
type Interactive interface {
	OpenPTY(ctx context.Context, cols, rows int) (*PTY, error)
	OpenSFTP(ctx context.Context) (*SFTP, error)
}

// PTY encapsulates an SSH PTY session without leaking *ssh.Session.
type PTY struct {
	session *ssh.Session
	stdin   io.WriteCloser
	stdout  io.Reader
	stderr  io.Reader
	pool    *Pool
	client  *ssh.Client
}

func (p *PTY) Write(data []byte) error          { _, err := p.stdin.Write(data); return err }
func (p *PTY) Read(buf []byte) (int, error)     { return p.stdout.Read(buf) }
func (p *PTY) ReadStderr(buf []byte) (int, error) { return p.stderr.Read(buf) }
func (p *PTY) WindowChange(rows, cols int) error { return p.session.WindowChange(rows, cols) }
func (p *PTY) Close() {
	p.session.Close()
	p.pool.Release(p.client)
}

// SFTP encapsulates an SFTP client without leaking *sftp.Client.
type SFTP struct {
	client    *sftp.Client
	sshClient *ssh.Client
	pool      *Pool
}

func (s *SFTP) ReadDir(path string) ([]os.FileInfo, error) { return s.client.ReadDir(path) }
func (s *SFTP) Open(path string) (*sftp.File, error)       { return s.client.Open(path) }
func (s *SFTP) Close() {
	s.client.Close()
	s.pool.Release(s.sshClient)
}

// --- VM implements Interactive ---

func (v *VM) OpenPTY(ctx context.Context, cols, rows int) (*PTY, error) {
	c, err := v.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("pty: acquire: %w", err)
	}
	session, err := c.NewSession()
	if err != nil {
		v.pool.Release(c)
		return nil, fmt.Errorf("pty: new session: %w", err)
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		session.Close()
		v.pool.Release(c)
		return nil, fmt.Errorf("pty: request pty: %w", err)
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		v.pool.Release(c)
		return nil, err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		v.pool.Release(c)
		return nil, err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		session.Close()
		v.pool.Release(c)
		return nil, err
	}
	if err := session.Start("/bin/sh"); err != nil {
		session.Close()
		v.pool.Release(c)
		return nil, fmt.Errorf("pty: start shell: %w", err)
	}
	return &PTY{
		session: session,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		pool:    v.pool,
		client:  c,
	}, nil
}

func (v *VM) OpenSFTP(ctx context.Context) (*SFTP, error) {
	c, err := v.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("sftp: acquire: %w", err)
	}
	sc, err := sftp.NewClient(c)
	if err != nil {
		v.pool.Release(c)
		return nil, fmt.Errorf("sftp: new client: %w", err)
	}
	return &SFTP{client: sc, sshClient: c, pool: v.pool}, nil
}
