package sandbox

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/user/golovebox/internal/config"
)

type Manager struct {
	cmd *exec.Cmd
	qmp *QMPClient
	cfg config.Config
}

func Start(cfg config.Config) (*Manager, error) {
	vmDir, err := cfg.VMDir()
	if err != nil {
		return nil, err
	}
	imgPath := filepath.Join(vmDir, "base.img")
	qmpAddr := fmt.Sprintf("127.0.0.1:%d", cfg.QMPPort)

	args := []string{
		"-hda", imgPath,
		"-m", "2048",
		"-nographic",
		"-net", fmt.Sprintf("user,hostfwd=tcp::%d-:22", cfg.SSHPort),
		"-qmp", fmt.Sprintf("tcp:%s,server,nowait", qmpAddr),
	}

	cmd := exec.Command(cfg.QEMUPath, args...)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start qemu: %w", err)
	}

	time.Sleep(2 * time.Second)

	qmpClient, err := Connect(qmpAddr)
	if err != nil {
		cmd.Process.Kill() //nolint:errcheck
		return nil, fmt.Errorf("qmp connect: %w", err)
	}

	return &Manager{cmd: cmd, qmp: qmpClient, cfg: cfg}, nil
}

func (m *Manager) Stop() error {
	if err := m.qmp.Quit(); err != nil {
		if m.cmd.Process != nil {
			m.cmd.Process.Kill() //nolint:errcheck
		}
	}
	return m.cmd.Wait()
}

func (m *Manager) HealthCheck() error {
	vmDir, err := m.cfg.VMDir()
	if err != nil {
		return err
	}
	client, err := Dial(
		"127.0.0.1",
		strconv.Itoa(m.cfg.SSHPort),
		"root",
		filepath.Join(vmDir, "id_rsa"),
	)
	if err != nil {
		return fmt.Errorf("ssh health check: %w", err)
	}
	return client.Close()
}
