package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

type Sealer struct {
	aead cipher.AEAD
}

func NewSealer(base64Key string) (*Sealer, error) {
	key, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return nil, fmt.Errorf("decode key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("expected 32 decoded bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	return &Sealer{aead: aead}, nil
}

func (s *Sealer) SealString(plaintext string) (string, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext := s.aead.Seal(nil, nonce, []byte(plaintext), nil)
	payload := append(nonce, ciphertext...)
	return base64.StdEncoding.EncodeToString(payload), nil
}

func (s *Sealer) OpenString(ciphertext string) (string, error) {
	payload, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	if len(payload) < s.aead.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce := payload[:s.aead.NonceSize()]
	body := payload[s.aead.NonceSize():]
	plaintext, err := s.aead.Open(nil, nonce, body, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt ciphertext: %w", err)
	}
	return string(plaintext), nil
}
