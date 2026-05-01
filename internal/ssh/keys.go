// Package ssh provides SSH public key discovery and host key generation.
package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	gossh "golang.org/x/crypto/ssh"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
)

// DiscoverPublicKey returns the contents of the first Public_Key found on the Host.
// If sshKeyFlag is non-empty it is tried first (highest precedence), followed by
// each path in constants.PublicKeyDefaultPaths in order (Req 4.1).
func DiscoverPublicKey(sshKeyFlag string) (string, error) {
	candidates := make([]string, 0, len(constants.PublicKeyDefaultPaths)+1)
	if sshKeyFlag != "" {
		candidates = append(candidates, sshKeyFlag)
	}
	candidates = append(candidates, constants.PublicKeyDefaultPaths...)

	for _, p := range candidates {
		expanded := expandHome(p)
		data, err := os.ReadFile(expanded)
		if err == nil {
			return string(data), nil
		}
	}
	return "", errors.New("no SSH public key found; use --ssh-key to specify one")
}

// GenerateHostKeyPair generates a new SSH host key pair using the algorithm
// defined by constants.SSHHostKeyType (currently ed25519).
// Returns the PEM-encoded private key and the authorised-keys-format public key.
func GenerateHostKeyPair() (priv, pub string, err error) {
	// constants.SSHHostKeyType = "ed25519" — the crypto call must match.
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generating ed25519 key pair: %w", err)
	}

	privPEM, err := gossh.MarshalPrivateKey(privKey, "")
	if err != nil {
		return "", "", fmt.Errorf("marshalling private key: %w", err)
	}
	privStr := string(pem.EncodeToMemory(privPEM))

	sshPub, err := gossh.NewPublicKey(pubKey)
	if err != nil {
		return "", "", fmt.Errorf("creating SSH public key: %w", err)
	}
	pubStr := string(gossh.MarshalAuthorizedKey(sshPub))

	return privStr, pubStr, nil
}

func expandHome(p string) string {
	if len(p) >= 2 && p[:2] == "~/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}
