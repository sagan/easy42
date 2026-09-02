package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// GenerateSessionSecret generates a 32-byte cryptographically secure session secret
func GenerateSessionSecret() (string, error) {
	bytes, err := GenerateRandomBytes(32)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func hashFingerprint(passwordHash string) string {
	h := sha256.Sum256([]byte(passwordHash))
	return base64.RawURLEncoding.EncodeToString(h[:16])
}

// CreateSessionToken creates an HMAC signed session token containing expiration timestamp
func CreateSessionToken(sessionSecret, passwordHash string, duration time.Duration) string {
	exp := time.Now().Add(duration).Unix()
	hashPrefix := hashFingerprint(passwordHash)

	payload := fmt.Sprintf("%d:%s", exp, hashPrefix)
	mac := hmac.New(sha256.New, []byte(sessionSecret))
	mac.Write([]byte(payload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	b64Payload := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return fmt.Sprintf("%s.%s", b64Payload, signature)
}

// VerifySessionToken verifies the HMAC signature and expiration of a session token
func VerifySessionToken(token, sessionSecret, passwordHash string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}

	b64Payload, signature := parts[0], parts[1]
	payloadBytes, err := base64.RawURLEncoding.DecodeString(b64Payload)
	if err != nil {
		return false
	}

	payload := string(payloadBytes)
	payloadParts := strings.Split(payload, ":")
	if len(payloadParts) != 2 {
		return false
	}

	expUnix, err := strconv.ParseInt(payloadParts[0], 10, 64)
	if err != nil {
		return false
	}

	if time.Now().Unix() > expUnix {
		return false // Expired
	}

	expectedPrefix := hashFingerprint(passwordHash)

	if payloadParts[1] != expectedPrefix {
		return false // Password changed
	}

	mac := hmac.New(sha256.New, []byte(sessionSecret))
	mac.Write([]byte(payload))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expectedSig))
}
