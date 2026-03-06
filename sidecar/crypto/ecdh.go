// crypto/ecdh.go — Ephemeral Curve25519 ECDH key exchange, new keypair per transfer
package crypto

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

type KeyPair struct {
	PrivateKey [32]byte
	PublicKey  [32]byte
}

func NewKeyPair() (*KeyPair, error) {
	var priv [32]byte
	if _, err := rand.Read(priv[:]); err != nil {
		return nil, fmt.Errorf("generate private key: %w", err)
	}
	// Clamp per Curve25519 spec
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64
	var pub [32]byte
	curve25519.ScalarBaseMult(&pub, &priv)
	return &KeyPair{PrivateKey: priv, PublicKey: pub}, nil
}

// DeriveSharedSecret performs ECDH and returns the raw 32-byte shared secret
func (kp *KeyPair) DeriveSharedSecret(theirPublicKeyHex string) ([]byte, error) {
	theirPubBytes, err := hex.DecodeString(theirPublicKeyHex)
	if err != nil {
		return nil, fmt.Errorf("decode public key: %w", err)
	}
	if len(theirPubBytes) != 32 {
		return nil, fmt.Errorf("invalid public key length: %d", len(theirPubBytes))
	}
	var theirPub [32]byte
	copy(theirPub[:], theirPubBytes)
	var shared [32]byte
	curve25519.ScalarMult(&shared, &kp.PrivateKey, &theirPub)
	return shared[:], nil
}

// DeriveSessionKeys splits the shared secret into encryption and HMAC keys
func DeriveSessionKeys(sharedSecret []byte) (encKey, hmacKey []byte) {
	// Simple HKDF-Expand style derivation
	h := hmac.New(sha256.New, sharedSecret)
	h.Write([]byte("AES-256-CTR"))
	encKey = h.Sum(nil)

	h.Reset()
	h.Write([]byte("HMAC-SHA256"))
	hmacKey = h.Sum(nil)
	return
}

func (kp *KeyPair) PublicKeyHex() string {
	return hex.EncodeToString(kp.PublicKey[:])
}
