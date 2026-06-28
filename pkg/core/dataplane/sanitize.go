package dataplane

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func sanitizeName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}
	var builder strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
			builder.WriteRune(char)
		case char >= 'A' && char <= 'Z':
			builder.WriteRune(char)
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
		default:
			builder.WriteByte('_')
		}
	}
	result := strings.Trim(builder.String(), "_")
	if result == "" {
		return "default"
	}
	if len(result) > 60 {
		return result[:60]
	}
	return result
}

func sanitizeComment(value string) string {
	value = strings.ReplaceAll(value, `"`, "")
	value = strings.ReplaceAll(value, `\`, "")
	return value
}

func nftablesTableNameForInstance(instanceID string) string {
	safe := sanitizeName(instanceID)
	if len(safe) > 48 {
		safe = safe[:48]
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(instanceID)))
	return "prism_" + safe + "_" + hex.EncodeToString(sum[:])[:10]
}
