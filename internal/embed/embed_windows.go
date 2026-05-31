//go:build windows

package embedassets

import "embed"

//go:embed assets/qemu/windows-amd64
var QEMUAssets embed.FS

const qemuAssetDir = "assets/qemu/windows-amd64"
