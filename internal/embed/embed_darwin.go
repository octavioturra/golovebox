//go:build darwin

package embedassets

import "embed"

//go:embed assets/qemu/darwin-arm64
var QEMUAssets embed.FS

const qemuAssetDir = "assets/qemu/darwin-arm64"
