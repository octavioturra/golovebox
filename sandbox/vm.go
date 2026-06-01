package sandbox

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/user/golovebox/core"
)

// VM implements core.Sandbox and sandbox.Interactive for a running QEMU VM.
// Create via NewVM (starts QEMU + SSH pool) or Attach (pool only, QEMU already running).
type VM struct {
	manager *Manager // nil when QEMU was not started by this VM
	pool    *Pool
	cfg     Config
}

// NewVM starts a fresh QEMU VM and returns a VM connected to it.
// The caller must call VM.Stop() to shut down QEMU and close the pool.
func NewVM(cfg Config) (*VM, error) {
	user := cfg.SSHUser
	if user == "" {
		user = "root"
	}
	mgr, err := Start(cfg)
	if err != nil {
		return nil, fmt.Errorf("sandbox: start qemu: %w", err)
	}
	pool := NewPool(PoolConfig{
		Host:    "127.0.0.1",
		Port:    cfg.SSHPort,
		User:    user,
		KeyPath: filepath.Join(cfg.VMDir, "id_rsa"),
	})
	return &VM{manager: mgr, pool: pool, cfg: cfg}, nil
}

// Attach creates a VM that connects to an already-running QEMU via SSH pool.
// Use this for commands that don't need to start QEMU (exec, status).
func Attach(cfg Config) *VM {
	user := cfg.SSHUser
	if user == "" {
		user = "root"
	}
	pool := NewPool(PoolConfig{
		Host:    "127.0.0.1",
		Port:    cfg.SSHPort,
		User:    user,
		KeyPath: filepath.Join(cfg.VMDir, "id_rsa"),
	})
	return &VM{pool: pool, cfg: cfg}
}

// Stop shuts down the QEMU VM (if started by NewVM) and closes the SSH pool.
func (v *VM) Stop() error {
	v.pool.Close()
	if v.manager != nil {
		return v.manager.Stop()
	}
	return nil
}

// --- core.Sandbox ---

func (v *VM) Exec(ctx context.Context, cmd string) (core.Output, error) {
	c, err := v.pool.Acquire(ctx)
	if err != nil {
		return core.Output{}, fmt.Errorf("sandbox exec: acquire: %w", err)
	}
	defer v.pool.Release(c)
	stdout, stderr, err := runCmd(c, cmd)
	return core.Output{Stdout: stdout, Stderr: stderr}, err
}

func (v *VM) PutFile(ctx context.Context, path string, data []byte) error {
	c, err := v.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("sandbox putfile: acquire: %w", err)
	}
	defer v.pool.Release(c)
	return writeFile(c, path, data)
}

func (v *VM) GetFile(ctx context.Context, path string) ([]byte, error) {
	c, err := v.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("sandbox getfile: acquire: %w", err)
	}
	defer v.pool.Release(c)
	return readFile(c, path)
}

func (v *VM) Ready(ctx context.Context) bool {
	c, err := v.pool.Acquire(ctx)
	if err != nil {
		return false
	}
	v.pool.Release(c)
	return true
}

// HealthCheck runs "echo ok" via SSH to verify the VM is healthy.
func (v *VM) HealthCheck(ctx context.Context) error {
	c, err := v.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("ssh health check: %w", err)
	}
	defer v.pool.Release(c)
	_, _, err = runCmd(c, "echo ok")
	return err
}

// DirectDial returns a raw SSH client suitable for one-shot use (e.g. exec/status CLI commands).
// The caller is responsible for closing it.
func DirectDial(host string, port int, user, keyPath string) (func(cmd string) (string, string, error), func() error, error) {
	c, err := Dial(host, strconv.Itoa(port), user, keyPath)
	if err != nil {
		return nil, nil, err
	}
	run := func(cmd string) (string, string, error) {
		return runCmd(c, cmd)
	}
	return run, c.Close, nil
}
