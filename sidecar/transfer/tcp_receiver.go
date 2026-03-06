// transfer/tcp_receiver.go — Receives encrypted chunks and reassembles file
package transfer

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/gestureshare/sidecar/crypto"
)

// ReceiveFileTCP connects to a TCP sender securely and writes the decrypted stream to disk.
func ReceiveFileTCP(address string, port int, encKey, hmacKey []byte, outPath string, onProgress func(sent, total int64), totalSize int64) error {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", address, port))
	if err != nil {
		return fmt.Errorf("dial tcp: %w", err)
	}
	defer conn.Close()

	file, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer file.Close()

	var allCiphertext []byte
	var received int64
	start := time.Now()

	for {
		lenBuf := make([]byte, 4)
		if _, rErr := io.ReadFull(conn, lenBuf); rErr != nil {
			return fmt.Errorf("read length: %w", rErr)
		}
		packetLen := binary.BigEndian.Uint32(lenBuf)

		payload := make([]byte, packetLen)
		if _, rErr := io.ReadFull(conn, payload); rErr != nil {
			return fmt.Errorf("read payload: %w", rErr)
		}

		if len(payload) < 16 {
			return fmt.Errorf("payload too short, missing counter")
		}

		counter := payload[:16]
		if isEOF(counter) {
			hmacSig := payload[16:] // HMAC is what follows the EOF counter
			if !crypto.VerifyHMAC(allCiphertext, hmacSig, hmacKey) {
				os.Remove(outPath)
				return fmt.Errorf("HMAC verification failed")
			}
			break
		}

		ciphertext := payload[16:]

		plaintext, dErr := crypto.EncryptCTR(ciphertext, encKey, counter)
		if dErr != nil {
			return fmt.Errorf("decrypt: %w", dErr)
		}

		if _, wErr := file.Write(plaintext); wErr != nil {
			return fmt.Errorf("write: %w", wErr)
		}

		allCiphertext = append(allCiphertext, ciphertext...)
		received += int64(len(plaintext))
		if onProgress != nil {
			onProgress(received, totalSize)
		}
	}

	elapsed := time.Since(start).Seconds()
	if elapsed > 0 {
		log.Printf("[tcp_receiver] received %d bytes in %.1fs (%.1f MB/s)", received, elapsed, float64(received)/elapsed/1e6)
	}
	log.Printf("[tcp_receiver] finished receiving file: %s", outPath)
	return nil
}

func isEOF(counter []byte) bool {
	for _, b := range counter {
		if b != 0xFF {
			return false
		}
	}
	return true
}
