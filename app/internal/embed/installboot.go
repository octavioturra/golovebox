package embedassets

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// CreateDisk is a no-op: base.img is created by ExtractAlpineImage, which
// writes the embedded cloud qcow2 directly. Kept for API compatibility.
func CreateDisk(qemuDir, vmDir string) error {
	diskPath := filepath.Join(vmDir, "base.img")
	if _, err := os.Stat(diskPath); err != nil {
		return fmt.Errorf("create disk: base.img missing (run ExtractAlpineImage first): %w", err)
	}
	return nil
}

// RunInstallBoot is a no-op: the Alpine cloud image is already installed.
// Cloud-init runs on the first boot driven by the normal sandbox.Start()
// call, which attaches cidata.iso as a CDROM. Kept for API compatibility.
func RunInstallBoot(ctx context.Context, qemuDir, vmDir string) error {
	return nil
}
