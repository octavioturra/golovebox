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

// StartTimeout controls how long Start waits for QMP to become reachable.
// Override before calling Start if a slower host needs more time.
var StartTimeout = 15 * time.Second

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
	logPath := filepath.Join(vmDir, "qemu.log")

	// Sanity check: make sure base.img is a real qcow2, not an empty placeholder
	// from an older install. SeaBIOS gives a generic "could not read boot disk"
	// otherwise, which is impossible to diagnose from inside QEMU.
	if f, err := os.Open(imgPath); err == nil {
		var magic [4]byte
		_, _ = f.Read(magic[:])
		f.Close()
		if magic != [4]byte{'Q', 'F', 'I', 0xfb} {
			return nil, fmt.Errorf("%s is not a qcow2 image (magic=%x) — delete it and re-run 'golovebox init'", imgPath, magic)
		}
	} else {
		return nil, fmt.Errorf("base.img missing at %s — run 'golovebox init' first", imgPath)
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open qemu log: %w", err)
	}

	args := []string{
		"-nographic",
		"-m", "2048",
		"-boot", "order=c,menu=off",
		// Main disk: if=none + virtio-blk-pci with bootindex=0 ensures SeaBIOS
		// recognises it as the boot device. The older "if=virtio" syntax does
		// not always set the bootindex correctly.
		"-drive", "file=" + imgPath + ",format=qcow2,if=none,id=disk0",
		"-device", "virtio-blk-pci,drive=disk0,bootindex=0",
		"-netdev", fmt.Sprintf("user,id=net0,hostfwd=tcp::%d-:22", cfg.SSHPort),
		"-device", "virtio-net-pci,netdev=net0,bootindex=99",
		"-qmp", fmt.Sprintf("tcp:%s,server,nowait", qmpAddr),
	}

	// Attach cloud-init NoCloud datasource (CIDATA ISO) if present. It is
	// detected by cloud-init at first boot. bootindex omitted on purpose —
	// SeaBIOS must never try to boot from it.
	cidataPath := filepath.Join(vmDir, "cidata.iso")
	if _, err := os.Stat(cidataPath); err == nil {
		args = append(args,
			"-drive", "file="+cidataPath+",format=raw,if=none,id=cidata,readonly=on",
			"-device", "virtio-blk-pci,drive=cidata",
		)
	}

	// Pass firmware directory when using the embedded QEMU binary.
	shareDir := filepath.Join(qemuDir, "share", "qemu")
	if _, serr := os.Stat(shareDir); serr == nil {
		args = append([]string{"-L", shareDir}, args...)
	}

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("open devnull: %w", err)
	}

	cmd := exec.Command(qemuExe, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = devNull
	if err := cmd.Start(); err != nil {
		devNull.Close()
		logFile.Close()
		return nil, fmt.Errorf("start qemu: %w", err)
	}
	devNull.Close()

	// Retry QMP connection for up to 15s — QEMU may take a few seconds to bind.
	// If QEMU crashes, cmd.Process.Wait() returns, detected via process state.
	var qmpClient *QMPClient
	deadline := time.Now().Add(StartTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		// Check if the process already exited (crash).
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			logFile.Close()
			return nil, fmt.Errorf("qemu exited immediately — check %s", logPath)
		}
		qmpClient, err = Connect(qmpAddr)
		if err == nil {
			break
		}
	}
	if qmpClient == nil {
		cmd.Process.Kill() //nolint:errcheck
		logFile.Close()
		return nil, fmt.Errorf("qmp connect (%s timeout): check %s for details", StartTimeout, logPath)
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
