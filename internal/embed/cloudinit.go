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
	// Cloud-init on the Alpine cloud image runs at first boot:
	//   1. drops a sshd drop-in that allows root key login
	//      (the base image ships with PermitRootLogin no, so the key alone is not enough)
	//   2. installs the SSH public key into root's authorized_keys
	//   3. installs the listed packages
	//   4. restarts sshd so the drop-in takes effect
	// The VM stays up so the host can SSH in and run the smoke test.
	key := strings.TrimSpace(pubKey)
	return fmt.Sprintf(`#cloud-config
disable_root: false
ssh_pwauth: false
users:
  - name: root
    lock_passwd: false
    ssh_authorized_keys:
      - %s
ssh_authorized_keys:
  - %s
write_files:
  - path: /etc/ssh/sshd_config.d/99-golovebox.conf
    permissions: '0644'
    owner: root:root
    content: |
      PermitRootLogin prohibit-password
      PubkeyAuthentication yes
      PasswordAuthentication no
      ChallengeResponseAuthentication no
packages:
  - git
  - curl
  - bash
  - openssh
  - python3
  - make
runcmd:
  - rc-update add sshd default
  - rc-service sshd restart
`, key, key)
}

// ensure io is used (w.AddFile takes io.Reader)
var _ io.Reader
