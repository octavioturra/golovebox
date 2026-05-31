// Package embedassets exposes QEMU binaries and Alpine ISO embedded at build time.
// OS-specific files (embed_windows.go, embed_linux.go, embed_darwin.go) declare
// QEMUAssets and qemuAssetDir via //go:embed directives.
// Run "mage fetch" before building to populate the asset directories.
package embedassets

import "embed"

//go:embed assets/alpine
var alpineAssets embed.FS

// AlpineImageName is the embedded cloud image file (qcow2, ready-to-boot).
const AlpineImageName = "alpine-cloud-x86_64.qcow2"

// ReadAlpineImage returns the embedded Alpine cloud qcow2 bytes.
// Returns ErrAssetsNotFetched if only the placeholder is present.
func ReadAlpineImage() ([]byte, error) {
	path := "assets/alpine/" + AlpineImageName
	return alpineAssets.ReadFile(path)
}
