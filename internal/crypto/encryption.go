package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"filippo.io/xaes256gcm"
)

const (
	DEKLength   = 32
	NonceLength = 24
	SaltLength  = 16
)

// GenerateRandomBytes generates n cryptographically secure random bytes
func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return b, nil
}

// EncryptWithKey encrypts plaintext using XAES-256-GCM with a 24-byte nonce.
// Returns base64(nonce + ciphertext + tag)
func EncryptWithKey(key []byte, plaintext []byte) (string, error) {
	if len(key) != 32 {
		return "", errors.New("key must be exactly 32 bytes for XAES-256-GCM")
	}

	aead, err := xaes256gcm.NewWithManualNonces(key)
	if err != nil {
		return "", fmt.Errorf("failed to initialize xaes256gcm: %w", err)
	}

	nonce, err := GenerateRandomBytes(NonceLength)
	if err != nil {
		return "", err
	}

	ciphertext := aead.Seal(nil, nonce, plaintext, nil)
	combined := append(nonce, ciphertext...)
	return base64.StdEncoding.EncodeToString(combined), nil
}

// DecryptWithKey decrypts base64(nonce + ciphertext + tag) using XAES-256-GCM
func DecryptWithKey(key []byte, encoded string) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("key must be exactly 32 bytes for XAES-256-GCM")
	}

	combined, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}

	if len(combined) < NonceLength+16 {
		return nil, errors.New("ciphertext too short")
	}

	nonce := combined[:NonceLength]
	ciphertext := combined[NonceLength:]

	aead, err := xaes256gcm.NewWithManualNonces(key)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize xaes256gcm: %w", err)
	}

	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// EncryptDEK encrypts the DEK using a password-derived key.
// Format: base64(salt_16B + nonce_24B + ciphertext)
func EncryptDEK(password string, dek []byte) (string, error) {
	salt, err := GenerateRandomBytes(SaltLength)
	if err != nil {
		return "", err
	}

	kek := DeriveKeyFromPassword(password, salt)
	defer memzero(kek)

	aead, err := xaes256gcm.NewWithManualNonces(kek)
	if err != nil {
		return "", fmt.Errorf("failed to initialize xaes256gcm: %w", err)
	}

	nonce, err := GenerateRandomBytes(NonceLength)
	if err != nil {
		return "", err
	}

	ciphertext := aead.Seal(nil, nonce, dek, nil)

	combined := make([]byte, 0, len(salt)+len(nonce)+len(ciphertext))
	combined = append(combined, salt...)
	combined = append(combined, nonce...)
	combined = append(combined, ciphertext...)

	return base64.StdEncoding.EncodeToString(combined), nil
}

// DecryptDEK decrypts the DEK using the password
func DecryptDEK(password string, encryptedDEK string) ([]byte, error) {
	combined, err := base64.StdEncoding.DecodeString(encryptedDEK)
	if err != nil {
		return nil, fmt.Errorf("failed to decode encrypted DEK: %w", err)
	}

	minLen := SaltLength + NonceLength + 16
	if len(combined) < minLen {
		return nil, errors.New("encrypted DEK payload is too short")
	}

	salt := combined[:SaltLength]
	nonce := combined[SaltLength : SaltLength+NonceLength]
	ciphertext := combined[SaltLength+NonceLength:]

	kek := DeriveKeyFromPassword(password, salt)
	defer memzero(kek)

	aead, err := xaes256gcm.NewWithManualNonces(kek)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize xaes256gcm: %w", err)
	}

	dek, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt DEK (invalid password?): %w", err)
	}

	return dek, nil
}

func memzero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
