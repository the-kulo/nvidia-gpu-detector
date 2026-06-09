package token

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func GenerateRenewToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("generate random token failed: %w", err)
	}

	return hex.EncodeToString(b), nil
}
