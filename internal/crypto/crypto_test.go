package crypto

import (
	"encoding/base64"
	"testing"
	"time"
)

func TestPasswordHashingAndVerification(t *testing.T) {
	password, err := GenerateRandomPassword()
	if err != nil {
		t.Fatalf("GenerateRandomPassword failed: %v", err)
	}
	if len(password) != 22 {
		t.Fatalf("Expected password len 22, got %d", len(password))
	}

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	valid, err := VerifyPassword(password, hash)
	if err != nil || !valid {
		t.Fatalf("VerifyPassword expected true, got %v (err: %v)", valid, err)
	}

	invalid, _ := VerifyPassword("wrongpassword", hash)
	if invalid {
		t.Fatalf("VerifyPassword with wrong password expected false, got true")
	}
}

func TestDEKEncryptionAndVault(t *testing.T) {
	password := "TestSecretPassword123"
	dek, err := GenerateRandomBytes(32)
	if err != nil {
		t.Fatalf("GenerateRandomBytes failed: %v", err)
	}

	encryptedDEK, err := EncryptDEK(password, dek)
	if err != nil {
		t.Fatalf("EncryptDEK failed: %v", err)
	}

	decryptedDEK, err := DecryptDEK(password, encryptedDEK)
	if err != nil {
		t.Fatalf("DecryptDEK failed: %v", err)
	}

	if len(decryptedDEK) != 32 {
		t.Fatalf("Expected 32 bytes DEK, got %d", len(decryptedDEK))
	}

	vault := NewKeyVault()
	if vault.IsUnlocked() {
		t.Fatalf("Vault should be locked initially")
	}

	if err := vault.Unlock(decryptedDEK); err != nil {
		t.Fatalf("Vault Unlock failed: %v", err)
	}
	if !vault.IsUnlocked() {
		t.Fatalf("Vault should be unlocked now")
	}

	testPlaintext := "wireguard-private-key-mock-12345"
	encrypted, err := vault.EncryptField(testPlaintext)
	if err != nil {
		t.Fatalf("EncryptField failed: %v", err)
	}

	decrypted, err := vault.DecryptField(encrypted)
	if err != nil {
		t.Fatalf("DecryptField failed: %v", err)
	}

	if decrypted != testPlaintext {
		t.Fatalf("Expected %s, got %s", testPlaintext, decrypted)
	}

	vault.Lock()
	if vault.IsUnlocked() {
		t.Fatalf("Vault should be locked after Lock()")
	}
}

func TestSessionToken(t *testing.T) {
	secret, err := GenerateSessionSecret()
	if err != nil {
		t.Fatalf("GenerateSessionSecret failed: %v", err)
	}

	passHash := "$argon2id$v=19$m=65536,t=3,p=2$mockSalt$mockHash"
	token := CreateSessionToken(secret, passHash, 1*time.Hour)

	if !VerifySessionToken(token, secret, passHash) {
		t.Fatalf("VerifySessionToken failed for valid token")
	}

	if VerifySessionToken(token, secret, "differentHash") {
		t.Fatalf("VerifySessionToken should fail for modified passHash")
	}

	expiredToken := CreateSessionToken(secret, passHash, -1*time.Minute)
	if VerifySessionToken(expiredToken, secret, passHash) {
		t.Fatalf("VerifySessionToken should fail for expired token")
	}
}

func TestWireguardKeyPair(t *testing.T) {
	kp, err := GenerateWgKeyPair()
	if err != nil {
		t.Fatalf("GenerateWgKeyPair failed: %v", err)
	}

	if kp.PrivateKey == "" || kp.PublicKey == "" {
		t.Fatalf("Generated keypair has empty keys")
	}

	pub, err := GetWgPublicKey(kp.PrivateKey)
	if err != nil {
		t.Fatalf("GetWgPublicKey failed: %v", err)
	}

	if pub != kp.PublicKey {
		t.Fatalf("Calculated public key %s doesn't match generated %s", pub, kp.PublicKey)
	}
}

func TestGenerateWgPsk(t *testing.T) {
	psk, err := GenerateWgPsk()
	if err != nil {
		t.Fatalf("GenerateWgPsk failed: %v", err)
	}
	if err := ValidateWgKey(psk); err != nil {
		t.Fatalf("Generated PSK is invalid: %v", err)
	}

	psk2, err := GenerateWgPsk()
	if err != nil {
		t.Fatalf("GenerateWgPsk (2) failed: %v", err)
	}
	if psk == psk2 {
		t.Fatalf("Consecutive generated PSKs should not be equal")
	}
}

func TestValidateWgKey(t *testing.T) {
	if err := ValidateWgKey(""); err == nil {
		t.Fatalf("expected error for empty key")
	}
	if err := ValidateWgKey("invalid-base64-???"); err == nil {
		t.Fatalf("expected error for non-base64 key")
	}
	// 16 bytes base64
	shortKey := base64.StdEncoding.EncodeToString(make([]byte, 16))
	if err := ValidateWgKey(shortKey); err == nil {
		t.Fatalf("expected error for short key")
	}
	// 32 bytes valid key
	validKey := base64.StdEncoding.EncodeToString(make([]byte, 32))
	if err := ValidateWgKey(validKey); err != nil {
		t.Fatalf("expected valid key, got: %v", err)
	}
}
