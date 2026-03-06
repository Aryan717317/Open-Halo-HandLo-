// crypto/hmac.go — HMAC-SHA256 calculation for file integrity
package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
)

// ComputeHMAC generates a 32-byte HMAC-SHA256 tag over the given data.
func ComputeHMAC(data []byte, key []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

// VerifyHMAC checks if the provided tag matches the computed HMAC-SHA256 of the data.
func VerifyHMAC(data []byte, mac []byte, key []byte) bool {
	expectedMAC := ComputeHMAC(data, key)
	return hmac.Equal(mac, expectedMAC)
}
