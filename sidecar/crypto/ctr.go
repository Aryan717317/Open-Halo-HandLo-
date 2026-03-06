// crypto/ctr.go — AES-256-CTR encryption/decryption
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

// EncryptCTR encrypts or decrypts data using AES-256-CTR.
// Since CTR is a stream cipher, encryption and decryption are identical operations.
func EncryptCTR(data []byte, key []byte, counter []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes, got %d", len(key))
	}
	if len(counter) != aes.BlockSize {
		return nil, fmt.Errorf("counter must be %d bytes, got %d", aes.BlockSize, len(counter))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// We must not modify the original counter passed in, as the cipher.NewCTR modifies it internally.
	iv := make([]byte, aes.BlockSize)
	copy(iv, counter)

	stream := cipher.NewCTR(block, iv)
	out := make([]byte, len(data))
	stream.XORKeyStream(out, data)

	return out, nil
}

// GenerateCounterHelper generates a 16-byte random counter (nonce).
func GenerateCounter() ([]byte, error) {
	counter := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, counter); err != nil {
		return nil, err
	}
	return counter, nil
}
