package toolskills

import (
	"context"
	"fmt"

	"github.com/user/golovebox/core"
)

type provisioner struct{ sb core.Sandbox }

// NewProvisioner returns a core.Provisioner that runs skill setup commands via sb.
func NewProvisioner(sb core.Sandbox) core.Provisioner {
	return &provisioner{sb}
}

func (p *provisioner) Provision(ctx context.Context, skill core.SkillMeta) error {
	for _, cmd := range skill.Provision {
		out, err := p.sb.Exec(ctx, cmd)
		if err != nil {
			return fmt.Errorf("provision %q: exec: %w", cmd, err)
		}
		if out.ExitCode != 0 {
			return fmt.Errorf("provision %q: exit %d: %s", cmd, out.ExitCode, out.Stderr)
		}
	}
	return nil
}
