package embedassets

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
)

// GenerateSSHKeypair creates id_rsa and id_rsa.pub in vmDir.
// Returns the public key in authorized_keys format.
// Idempotent: returns existing key if both files are present.
func GenerateSSHKeypair(vmDir string) (pubKey string, err error) {
	privPath := filepath.Join(vmDir, "id_rsa")
	pubPath := filepath.Join(vmDir, "id_rsa.pub")

	// Idempotency: reuse existing key pair.
	if _, err := os.Stat(privPath); err == nil {
		if _, err := os.Stat(pubPath); err == nil {
			data, err := os.ReadFile(pubPath)
			if err != nil {
				return "", fmt.Errorf("keygen: read pub: %w", err)
			}
			return string(data), nil
		}
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return "", fmt.Errorf("keygen: generate: %w", err)
	}

	// Encode private key as PEM.
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	if err := os.WriteFile(privPath, privPEM, 0o600); err != nil {
		return "", fmt.Errorf("keygen: write private: %w", err)
	}

	// Encode public key in authorized_keys format.
	sshPub, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", fmt.Errorf("keygen: public key: %w", err)
	}
	authorizedKey := string(ssh.MarshalAuthorizedKey(sshPub))
	if err := os.WriteFile(pubPath, []byte(authorizedKey), 0o644); err != nil {
		return "", fmt.Errorf("keygen: write public: %w", err)
	}

	return authorizedKey, nil
}
