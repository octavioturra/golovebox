package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/user/golovebox/internal/config"
	embedassets "github.com/user/golovebox/internal/embed"
)

type Manager struct {
	cmd     *exec.Cmd
	qmp     *QMPClient
	cfg     config.Config
	logFile *os.File
}

func Start(cfg config.Config) (*Manager, error) {
	vmDir, err := cfg.VMDir()
	if err != nil {
		return nil, err
	}
	qemuDir, err := cfg.QEMUDir()
	if err != nil {
		return nil, err
	}

	qemuExe := cfg.QEMUPath
	if qemuExe == "" {
		qemuExe = embedassets.QEMUExePath(qemuDir)
	}

	imgPath := filepath.Join(vmDir, "base.img")
	qmpAddr := fmt.Sprintf("127.0.0.1:%d", cfg.QMPPort)

	args := []string{
		"-drive", "file=" + imgPath + ",format=qcow2,if=virtio",
		"-m", "2048",
		"-nographic",
		"-netdev", fmt.Sprintf("user,id=net0,hostfwd=tcp::%d-:22", cfg.SSHPort),
		"-device", "virtio-net-pci,netdev=net0",
		"-qmp", fmt.Sprintf("tcp:%s,server,nowait", qmpAddr),
	}

	// Pass firmware directory when using the embedded QEMU binary.
	shareDir := filepath.Join(qemuDir, "share", "qemu")
	if _, serr := os.Stat(shareDir); serr == nil {
		args = append([]string{"-L", shareDir}, args...)
	}

	vmDir, err2 := cfg.VMDir()
	if err2 != nil {
		return nil, err2
	}
	logPath := filepath.Join(vmDir, "qemu.log")
	logFile, err2 := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err2 != nil {
		return nil, fmt.Errorf("open qemu log: %w", err2)
	}

	cmd := exec.Command(qemuExe, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("start qemu: %w", err)
	}

	time.Sleep(2 * time.Second)

	qmpClient, err := Connect(qmpAddr)
	if err != nil {
		cmd.Process.Kill() //nolint:errcheck
		return nil, fmt.Errorf("qmp connect: %w", err)
	}

	return &Manager{cmd: cmd, qmp: qmpClient, cfg: cfg, logFile: logFile}, nil
}

func (m *Manager) Stop() error {
	if err := m.qmp.Quit(); err != nil {
		if m.cmd.Process != nil {
			m.cmd.Process.Kill() //nolint:errcheck
		}
	}
	err := m.cmd.Wait()
	if m.logFile != nil {
		m.logFile.Close()
	}
	return err
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
