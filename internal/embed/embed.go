// Package embedassets exposes QEMU binaries and Alpine ISO embedded at build time.
// OS-specific files (embed_windows.go, embed_linux.go, embed_darwin.go) declare
// QEMUAssets and qemuAssetDir via //go:embed directives.
// Run "mage fetch" before building to populate the asset directories.
package embedassets

import "embed"

//go:embed assets/alpine
var alpineAssets embed.FS

const alpineISOName = "alpine-virt-x86_64.iso"

// ReadAlpineISO returns the embedded Alpine ISO bytes.
// Returns an error if the placeholder file is present instead of the real ISO
// (i.e., "mage fetch" has not been run).
func ReadAlpineISO() ([]byte, error) {
	path := "assets/alpine/" + alpineISOName
	return alpineAssets.ReadFile(path)
}
