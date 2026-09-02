package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

// WgKeyPair holds base64 encoded private and public keys
type WgKeyPair struct {
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
}

// GenerateWgKeyPair generates a standard WireGuard Curve25519 keypair
func GenerateWgKeyPair() (*WgKeyPair, error) {
	var privateKey [32]byte
	if _, err := rand.Read(privateKey[:]); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Clamp the private key for Curve25519
	privateKey[0] &= 248
	privateKey[31] &= 127
	privateKey[31] |= 64

	var publicKey [32]byte
	curve25519.ScalarBaseMult(&publicKey, &privateKey)

	return &WgKeyPair{
		PrivateKey: base64.StdEncoding.EncodeToString(privateKey[:]),
		PublicKey:  base64.StdEncoding.EncodeToString(publicKey[:]),
	}, nil
}

// GetWgPublicKey computes the public key from a base64 encoded WireGuard private key
func GetWgPublicKey(privateKeyB64 string) (string, error) {
	privBytes, err := base64.StdEncoding.DecodeString(privateKeyB64)
	if err != nil {
		return "", fmt.Errorf("invalid base64 private key: %w", err)
	}
	if len(privBytes) != 32 {
		return "", fmt.Errorf("private key must be 32 bytes, got %d", len(privBytes))
	}

	var privateKey [32]byte
	copy(privateKey[:], privBytes)

	privateKey[0] &= 248
	privateKey[31] &= 127
	privateKey[31] |= 64

	var publicKey [32]byte
	curve25519.ScalarBaseMult(&publicKey, &privateKey)

	return base64.StdEncoding.EncodeToString(publicKey[:]), nil
}
