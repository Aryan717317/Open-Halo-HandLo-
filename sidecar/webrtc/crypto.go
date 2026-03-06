package webrtc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"

	"golang.org/x/crypto/curve25519"
)

// TransferCrypto handles ephemeral ECDH key exchange and AES-GCM chunk encryption
type TransferCrypto struct {
	privateKey [32]byte
	PublicKey  [32]byte
}

func NewTransferCrypto() *TransferCrypto {
	tc := &TransferCrypto{}
	// Generate ephemeral Curve25519 keypair
	rand.Read(tc.privateKey[:])
	// Clamp private key per Curve25519 spec
	tc.privateKey[0] &= 248
	tc.privateKey[31] &= 127
	tc.privateKey[31] |= 64
	curve25519.ScalarBaseMult(&tc.PublicKey, &tc.privateKey)
	return tc
}

// DeriveSharedKey performs ECDH with peer's public key → 32-byte shared secret
func (tc *TransferCrypto) DeriveSharedKey(peerPublicKey []byte) ([]byte, error) {
	if len(peerPublicKey) != 32 {
		return nil, fmt.Errorf("invalid peer public key length: %d", len(peerPublicKey))
	}
	var peerKey [32]byte
	copy(peerKey[:], peerPublicKey)

	var shared [32]byte
	curve25519.ScalarMult(&shared, &tc.privateKey, &peerKey)
	return shared[:], nil
}

// encryptChunk encrypts a single file chunk with AES-256-GCM
// Format: [12-byte nonce][encrypted data][16-byte auth tag]
func encryptChunk(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}

	// Fresh random nonce per chunk — forward secrecy
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("nonce gen: %w", err)
	}

	// Seal prepends nonce: output = nonce || ciphertext || tag
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// decryptChunk decrypts a chunk produced by encryptChunk
func decryptChunk(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce := ciphertext[:nonceSize]
	data := ciphertext[nonceSize:]

	return gcm.Open(nil, nonce, data, nil)
}
