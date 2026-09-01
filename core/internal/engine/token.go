package engine

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func LoadOrCreateToken(explicit string) (token string, source string, err error) {
	if value := strings.TrimSpace(explicit); value != "" {
		if len(value) < 32 {
			return "", "", errors.New("MODBUS_CONSOLE_AUTH_TOKEN must be at least 32 characters")
		}
		return value, "environment", nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	dir := filepath.Join(home, ".modbus-console")
	path := filepath.Join(dir, "pairing-token")
	if raw, readErr := os.ReadFile(path); readErr == nil {
		value := strings.TrimSpace(string(raw))
		if len(value) < 32 {
			return "", "", errors.New("pairing token file is invalid")
		}
		return value, path, nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	value := base64.RawURLEncoding.EncodeToString(buf)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(path, []byte(value+"\n"), 0o600); err != nil {
		return "", "", err
	}
	return value, path, nil
}
