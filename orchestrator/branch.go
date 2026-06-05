package orchestrator

import (
	"fmt"
	"math/rand"
)

var branchAdjectives = []string{
	"amber", "bold", "calm", "dark", "eager", "fair", "gold", "hard", "idle",
	"jade", "keen", "lime", "mild", "neat", "open", "pale", "quick", "rich",
	"safe", "tall", "urban", "vast", "warm", "xray", "young", "zeal",
	"brave", "crisp", "dense", "fleet", "grand", "heavy", "inky", "jolly",
	"lush", "mute", "noble", "opal", "prime", "quiet",
}

var branchNouns = []string{
	"atlas", "basin", "cedar", "delta", "ember", "forge", "grove", "haven",
	"inlet", "jetty", "knoll", "ledge", "marsh", "nexus", "orbit", "pixel",
	"quill", "ridge", "stone", "tidal", "umbra", "vault", "whirl", "xenon",
	"yield", "zenith", "arrow", "blaze", "crest", "drift", "epoch", "flare",
	"glyph", "hinge", "ingot", "joust", "kappa", "lumen", "manor", "notch",
}

// GenerateBranchName returns a unique branch name with the glb- prefix.
// Format: glb-<adjective>-<noun>-<4hex>
func GenerateBranchName() string {
	adj := branchAdjectives[rand.Intn(len(branchAdjectives))]
	noun := branchNouns[rand.Intn(len(branchNouns))]
	hex := fmt.Sprintf("%04x", rand.Intn(0x10000))
	return fmt.Sprintf("glb-%s-%s-%s", adj, noun, hex)
}
