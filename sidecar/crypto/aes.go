// crypto/aes.go — AES-256-GCM encryption: new random nonce per chunk
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

const (
	NonceSize = 12
	TagSize   = 16
	Overhead  = NonceSize + TagSize
)

type ChunkCipher struct {
	gcm cipher.AEAD
}

func NewChunkCipher(key []byte) (*ChunkCipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &ChunkCipher{gcm: gcm}, nil
}

// Encrypt returns [12-byte nonce][ciphertext + 16-byte auth tag]
func (c *ChunkCipher) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return c.gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func (c *ChunkCipher) Decrypt(data []byte) ([]byte, error) {
	if len(data) < Overhead {
		return nil, fmt.Errorf("chunk too short: %d bytes", len(data))
	}
	return c.gcm.Open(nil, data[:NonceSize], data[NonceSize:], nil)
}
