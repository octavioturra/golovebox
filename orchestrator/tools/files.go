package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/user/golovebox/core"
)

func ReadFile(ctx context.Context, sb core.Sandbox, remotePath string) ([]byte, error) {
	return sb.GetFile(ctx, remotePath)
}

func WriteFile(ctx context.Context, sb core.Sandbox, remotePath string, data []byte) error {
	return sb.PutFile(ctx, remotePath, data)
}

func ListDir(ctx context.Context, sb core.Sandbox, remotePath string) ([]string, error) {
	o, err := sb.Exec(ctx, "ls -1 "+remotePath)
	if err != nil {
		return nil, fmt.Errorf("list dir %s: %w", remotePath, err)
	}
	var entries []string
	for _, line := range strings.Split(strings.TrimSpace(o.Stdout), "\n") {
		if line != "" {
			entries = append(entries, line)
		}
	}
	return entries, nil
}
