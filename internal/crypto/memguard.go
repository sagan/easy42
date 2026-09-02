package crypto

import (
	"errors"
	"sync"

	"github.com/awnumar/memguard"
)

var (
	ErrVaultLocked = errors.New("key vault is locked; user password required")
)

// KeyVault safely manages the in-memory DEK using memguard protected enclaves
type KeyVault struct {
	mu      sync.RWMutex
	enclave *memguard.Enclave
}

// NewKeyVault creates a new locked KeyVault
func NewKeyVault() *KeyVault {
	return &KeyVault{}
}

// IsUnlocked returns true if the DEK is unlocked in memory
func (kv *KeyVault) IsUnlocked() bool {
	kv.mu.RLock()
	defer kv.mu.RUnlock()
	return kv.enclave != nil
}

// Unlock sets the DEK inside a memguard enclave and wipes the input slice
func (kv *KeyVault) Unlock(dek []byte) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	if len(dek) != 32 {
		return errors.New("invalid DEK length: must be 32 bytes")
	}

	// Create new enclave
	kv.enclave = memguard.NewEnclave(dek)
	// Zero source memory
	memzero(dek)
	return nil
}

// Lock wipes and destroys the enclave from memory
func (kv *KeyVault) Lock() {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	kv.enclave = nil
}

// WithKey executes a function with the raw DEK in a protected buffer, destroying it immediately after
func (kv *KeyVault) WithKey(fn func(dek []byte) error) error {
	kv.mu.RLock()
	defer kv.mu.RUnlock()

	if kv.enclave == nil {
		return ErrVaultLocked
	}

	buf, err := kv.enclave.Open()
	if err != nil {
		return err
	}
	defer buf.Destroy()

	return fn(buf.Bytes())
}

// EncryptField encrypts a string using the current in-memory DEK
func (kv *KeyVault) EncryptField(plaintext string) (string, error) {
	var encrypted string
	err := kv.WithKey(func(dek []byte) error {
		var err error
		encrypted, err = EncryptWithKey(dek, []byte(plaintext))
		return err
	})
	if err != nil {
		return "", err
	}
	return encrypted, nil
}

// DecryptField decrypts a base64 encrypted string using the current in-memory DEK
func (kv *KeyVault) DecryptField(encrypted string) (string, error) {
	var plaintext string
	err := kv.WithKey(func(dek []byte) error {
		bytes, err := DecryptWithKey(dek, encrypted)
		if err != nil {
			return err
		}
		plaintext = string(bytes)
		return nil
	})
	if err != nil {
		return "", err
	}
	return plaintext, nil
}
