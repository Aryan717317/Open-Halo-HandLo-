// crypto/ecdh.go — Ephemeral Curve25519 ECDH key exchange, new keypair per transfer
package crypto

import (
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

// DeriveSharedSecret performs ECDH and hashes result to produce AES-256 key
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
	key := sha256.Sum256(shared[:])
	return key[:], nil
}

func (kp *KeyPair) PublicKeyHex() string {
	return hex.EncodeToString(kp.PublicKey[:])
}
