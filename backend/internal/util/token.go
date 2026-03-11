package util

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const tokenAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func NormalizeTokenCode(code string) string {
	code = strings.TrimSpace(strings.ToUpper(code))
	code = strings.ReplaceAll(code, " ", "")
	return code
}

func HashTokenCode(code string) string {
	normalized := NormalizeTokenCode(code)
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func GenerateTokenCode() (string, error) {
	parts := []string{randomPart(4), randomPart(4), randomPart(4)}
	if parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", fmt.Errorf("failed to generate token")
	}
	return strings.Join(parts, "-"), nil
}

func randomPart(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	chars := make([]byte, n)
	for i := range buf {
		chars[i] = tokenAlphabet[int(buf[i])%len(tokenAlphabet)]
	}
	return string(chars)
}
