package httpserver

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	platformcrypto "github.com/srapi/srapi/apps/api/internal/platform/crypto"
)

func (s *Server) masterKeyAEAD() ([]byte, error) {
	return platformcrypto.DeriveAESKey(s.cfg.Security.MasterKey)
}

func (s *Server) encryptMasterSecret(plaintext string, version string) (string, error) {
	key, err := s.masterKeyAEAD()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), []byte(version))
	return fmt.Sprintf("%s:%s:%s", version, base64.RawURLEncoding.EncodeToString(nonce), base64.RawURLEncoding.EncodeToString(ciphertext)), nil
}

func (s *Server) decryptMasterSecret(ciphertextValue string, version string) (string, error) {
	parts := strings.Split(ciphertextValue, ":")
	if len(parts) != 3 || parts[0] != version {
		return "", errors.New("invalid encrypted secret")
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	encrypted, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", err
	}
	key, err := s.masterKeyAEAD()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	raw, err := gcm.Open(nil, nonce, encrypted, []byte(version))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
