package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// Config holds the parameters needed to start and connect to the QEMU sandbox.
type Config struct {
	QEMUExe string // path to qemu-system-x86_64[.exe]; auto-detected in QEMUDir if empty
	QEMUDir string // dir containing the QEMU binary and share/qemu/
	VMDir   string // dir for base.img, cidata.iso, id_rsa, qemu.log
	SSHPort int
	QMPPort int
	SSHUser string // default "root"
}

// Manager manages the QEMU VM process lifecycle.
type Manager struct {
	cmd     *exec.Cmd
	qmp     *QMPClient
	cfg     Config
	logFile *os.File
}

// StartTimeout controls how long Start waits for QMP to become reachable.
var StartTimeout = 15 * time.Second

// Start launches the QEMU VM described by cfg and returns a Manager.
func Start(cfg Config) (*Manager, error) {
	qemuExe := cfg.QEMUExe
	if qemuExe == "" {
		qemuExe = defaultQEMUExe(cfg.QEMUDir)
	}

	imgPath := filepath.Join(cfg.VMDir, "base.img")
	qmpAddr := fmt.Sprintf("127.0.0.1:%d", cfg.QMPPort)
	logPath := filepath.Join(cfg.VMDir, "qemu.log")

	// Sanity check: make sure base.img is a real qcow2.
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
		// recognises it as the boot device.
		"-drive", "file=" + imgPath + ",format=qcow2,if=none,id=disk0",
		"-device", "virtio-blk-pci,drive=disk0,bootindex=0",
		"-netdev", fmt.Sprintf("user,id=net0,hostfwd=tcp::%d-:22", cfg.SSHPort),
		"-device", "virtio-net-pci,netdev=net0,bootindex=99",
		"-qmp", fmt.Sprintf("tcp:%s,server,nowait", qmpAddr),
	}

	// Attach cloud-init NoCloud datasource (CIDATA ISO) if present.
	cidataPath := filepath.Join(cfg.VMDir, "cidata.iso")
	if _, err := os.Stat(cidataPath); err == nil {
		args = append(args,
			"-drive", "file="+cidataPath+",format=raw,if=none,id=cidata,readonly=on",
			"-device", "virtio-blk-pci,drive=cidata",
		)
	}

	// Pass firmware directory when the share/ dir exists alongside the binary.
	shareDir := filepath.Join(cfg.QEMUDir, "share")
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

	// Retry QMP connection for up to StartTimeout.
	var qmpClient *QMPClient
	deadline := time.Now().Add(StartTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
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

// IsRunning reports whether the QEMU process is still alive.
func (m *Manager) IsRunning() bool {
	return m.cmd != nil && m.cmd.Process != nil
}

// Stop sends a QMP quit and waits for the process to exit.
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

// defaultQEMUExe returns the expected QEMU binary path for the current OS.
func defaultQEMUExe(qemuDir string) string {
	name := "qemu-system-x86_64"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(qemuDir, name)
}
