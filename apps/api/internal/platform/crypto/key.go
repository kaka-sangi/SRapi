package crypto

import (
	"crypto/sha256"
	"errors"
	"strings"
)

var ErrWeakMasterKey = errors.New("master key must be at least 32 bytes")

func DeriveAESKey(masterKey string) ([]byte, error) {
	masterKey = strings.TrimSpace(masterKey)
	if len(masterKey) < 32 {
		return nil, ErrWeakMasterKey
	}
	sum := sha256.Sum256([]byte(masterKey))
	key := make([]byte, len(sum))
	copy(key, sum[:])
	return key, nil
}
