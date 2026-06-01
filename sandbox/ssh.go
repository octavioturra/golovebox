package sandbox

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func Dial(host, port, user, keyPath string) (*ssh.Client, error) {
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read key %s: %w", keyPath, err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec
	}
	return ssh.Dial("tcp", net.JoinHostPort(host, port), cfg)
}

func runCmd(client *ssh.Client, cmd string) (stdout, stderr string, err error) {
	session, err := client.NewSession()
	if err != nil {
		return "", "", fmt.Errorf("new session: %w", err)
	}
	defer session.Close()
	var outBuf, errBuf bytes.Buffer
	session.Stdout = &outBuf
	session.Stderr = &errBuf
	err = session.Run(cmd)
	return outBuf.String(), errBuf.String(), err
}

func readFile(client *ssh.Client, remotePath string) ([]byte, error) {
	sc, err := sftp.NewClient(client)
	if err != nil {
		return nil, fmt.Errorf("sftp client: %w", err)
	}
	defer sc.Close()
	f, err := sc.Open(remotePath)
	if err != nil {
		return nil, fmt.Errorf("sftp open %s: %w", remotePath, err)
	}
	defer f.Close()
	return io.ReadAll(f)
}

func writeFile(client *ssh.Client, remotePath string, data []byte) error {
	sc, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("sftp client: %w", err)
	}
	defer sc.Close()
	f, err := sc.Create(remotePath)
	if err != nil {
		return fmt.Errorf("sftp create %s: %w", remotePath, err)
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}
