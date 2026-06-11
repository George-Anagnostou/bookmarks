package bookmarks

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate bookmark id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
