package embedassets

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kdomanski/iso9660"
)

// CreateCloudInitISO generates a NoCloud cloud-init data disk at destPath.
// The disk is an ISO9660 image with label "CIDATA" containing:
//   - meta-data  (instance-id + hostname)
//   - user-data  (cloud-config: SSH key, packages, poweroff)
//
// Alpine Virt ISO detects this disk and applies the configuration on first boot.
func CreateCloudInitISO(destPath, pubKey string) error {
	w, err := iso9660.NewWriter()
	if err != nil {
		return fmt.Errorf("cloudinit: new writer: %w", err)
	}
	defer w.Cleanup()

	metaData := "instance-id: golovebox-vm\nlocal-hostname: golovebox-vm\n"
	if err := w.AddFile(strings.NewReader(metaData), "meta-data"); err != nil {
		return fmt.Errorf("cloudinit: add meta-data: %w", err)
	}

	userData := buildUserData(pubKey)
	if err := w.AddFile(strings.NewReader(userData), "user-data"); err != nil {
		return fmt.Errorf("cloudinit: add user-data: %w", err)
	}

	tmp := destPath + ".tmp" //nolint:gocritic
	out, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("cloudinit: create: %w", err)
	}
	if err := w.WriteTo(out, "CIDATA"); err != nil {
		out.Close()
		os.Remove(tmp)
		return fmt.Errorf("cloudinit: write: %w", err)
	}
	out.Close()
	return os.Rename(tmp, destPath)
}

func buildUserData(pubKey string) string {
	// The runcmd section runs after packages are installed.
	// poweroff shuts the VM down cleanly; QEMU exits (due to -no-reboot).
	return fmt.Sprintf(`#cloud-config
ssh_authorized_keys:
  - %s
packages:
  - git
  - curl
  - bash
  - openssh
  - python3
  - make
runcmd:
  - rc-update add sshd default
  - /etc/init.d/sshd start
  - poweroff
`, strings.TrimSpace(pubKey))
}

// ensure io is used (w.AddFile takes io.Reader)
var _ io.Reader
