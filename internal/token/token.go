package token

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func GenerateRenewToken() (string, error) {
	return generateRandomHex(32)
}

func GenerateSessionID() (string, error) {
	return generateRandomHex(16)
}

func generateRandomHex(size int) (string, error) {
	b := make([]byte, size)
	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("generate random token failed: %w", err)
	}

	return hex.EncodeToString(b), nil
}
