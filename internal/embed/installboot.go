package embedassets

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// CreateDisk creates an empty qcow2 disk image using qemu-img.
// Idempotent: skips if base.img already exists and is > 100 MB (previous install).
func CreateDisk(qemuDir, vmDir string) error {
	diskPath := filepath.Join(vmDir, "base.img")
	if fi, err := os.Stat(diskPath); err == nil && fi.Size() > 100<<20 {
		return nil // already installed
	}

	qemuImg := QEMUImgPath(qemuDir)
	cmd := exec.Command(qemuImg, "create", "-f", "qcow2", diskPath, "8G")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("qemu-img create: %w", err)
	}
	return nil
}

// RunInstallBoot boots Alpine from the ISO with the cloud-init CIDATA disk.
// The VM configures itself (SSH key, packages) and powers off automatically
// (due to `poweroff` in user-data). QEMU exits after the VM shuts down (-no-reboot).
//
// Idempotent: skips if base.img already exists and is > 100 MB (previous install).
// Timeout: 10 minutes. If the VM does not exit within this window, the context
// is cancelled and the QEMU process is killed.
func RunInstallBoot(ctx context.Context, qemuDir, vmDir string) error {
	diskPath := filepath.Join(vmDir, "base.img")
	if fi, err := os.Stat(diskPath); err == nil && fi.Size() > 100<<20 {
		return nil // already installed
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	qemuExe := QEMUExePath(qemuDir)
	isoPath := filepath.Join(vmDir, alpineISOName)
	cidataPath := filepath.Join(vmDir, "cidata.iso")

	for _, f := range []string{diskPath, isoPath, cidataPath} {
		if _, err := os.Stat(f); err != nil {
			return fmt.Errorf("install boot: missing %s: %w", f, err)
		}
	}

	args := []string{
		"-m", "1024",
		"-nographic",
	}

	// Point QEMU at its firmware directory when using the embedded binary.
	shareDir := filepath.Join(qemuDir, "share", "qemu")
	if _, err := os.Stat(shareDir); err == nil {
		args = append(args, "-L", shareDir)
	}

	args = append(args,
		// System disk (install target)
		"-drive", "file="+diskPath+",format=qcow2,if=virtio",
		// Alpine ISO (boot medium)
		"-drive", "file="+isoPath+",format=raw,media=cdrom",
		// cloud-init NoCloud disk (detected by Alpine cloud-init)
		"-drive", "file="+cidataPath+",format=raw,if=virtio",
		// Network (no host port forwarding needed during install)
		"-netdev", "user,id=net0",
		"-device", "virtio-net-pci,netdev=net0",
		// Exit after VM powers off
		"-no-reboot",
	)

	cmd := exec.CommandContext(ctx, qemuExe, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println("[install] Booting Alpine for first-time configuration (≤10 min)...")
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("install boot: timed out after 10 minutes")
		}
		return fmt.Errorf("install boot: qemu: %w", err)
	}
	return nil
}
