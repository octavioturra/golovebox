package tools

import (
	"fmt"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/user/golovebox/internal/sandbox"
)

func ReadFile(client *ssh.Client, remotePath string) ([]byte, error) {
	return sandbox.ReadFile(client, remotePath)
}

func WriteFile(client *ssh.Client, remotePath string, data []byte) error {
	return sandbox.WriteFile(client, remotePath, data)
}

func ListDir(client *ssh.Client, remotePath string) ([]string, error) {
	sc, err := sftp.NewClient(client)
	if err != nil {
		return nil, fmt.Errorf("sftp client: %w", err)
	}
	defer sc.Close()

	entries, err := sc.ReadDir(remotePath)
	if err != nil {
		return nil, fmt.Errorf("list dir %s: %w", remotePath, err)
	}

	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names, nil
}
