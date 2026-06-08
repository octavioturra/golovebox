//go:build linux

package embedassets

import "embed"

//go:embed assets/qemu/linux-amd64
var QEMUAssets embed.FS

const qemuAssetDir = "assets/qemu/linux-amd64"
