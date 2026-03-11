package util

import (
	"fmt"
	"regexp"
	"strings"
)

var nonHex = regexp.MustCompile(`[^0-9a-fA-F]`)

// NormalizeMAC returns a strict AA:BB:CC:DD:EE:FF representation.
func NormalizeMAC(in string) (string, error) {
	clean := strings.TrimSpace(in)
	if clean == "" {
		return "", fmt.Errorf("empty mac")
	}
	hexOnly := nonHex.ReplaceAllString(clean, "")
	if len(hexOnly) != 12 {
		return "", fmt.Errorf("invalid mac length")
	}
	hexOnly = strings.ToUpper(hexOnly)
	var b strings.Builder
	b.Grow(17)
	for i := 0; i < len(hexOnly); i += 2 {
		if i > 0 {
			b.WriteByte(':')
		}
		b.WriteString(hexOnly[i : i+2])
	}
	return b.String(), nil
}
