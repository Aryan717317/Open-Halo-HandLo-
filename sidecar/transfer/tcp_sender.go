// transfer/tcp_sender.go — Raw TCP encrypted file sender
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

// SendFileTCP opens a TCP server on a random port, returns the port.
// When a client connects, it encrypts the file via CTR and sends it via TCP.
func SendFileTCP(filePath string, key []byte, onProgress func(sent, total int64)) (int, error) {
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return 0, fmt.Errorf("tcp listen: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	go func() {
		defer listener.Close()
		log.Printf("[tcp_sender] listening on port %d", port)

		conn, err := listener.Accept()
		if err != nil {
			log.Printf("[tcp_sender] accept error: %v", err)
			return
		}
		defer conn.Close()
		log.Printf("[tcp_sender] accepted connection from %s", conn.RemoteAddr())

		if err := sendTCP(conn, filePath, key, onProgress); err != nil {
			log.Printf("[tcp_sender] send error: %v", err)
		}
	}()

	return port, nil
}

func sendTCP(conn net.Conn, filePath string, key []byte, onProgress func(sent, total int64)) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	stat, _ := file.Stat()
	totalSize := stat.Size()

	buf := make([]byte, 64*1024)
	var sent int64
	var allCiphertext []byte
	start := time.Now()

	for {
		n, err := file.Read(buf)
		if n > 0 {
			plaintext := buf[:n]
			counter, errGenerate := crypto.GenerateCounter()
			if errGenerate != nil {
				return fmt.Errorf("generate counter: %w", errGenerate)
			}

			ciphertext, cErr := crypto.EncryptCTR(plaintext, key, counter)
			if cErr != nil {
				return fmt.Errorf("encrypt: %w", cErr)
			}

			// Packet length = len(counter) + len(ciphertext)
			packetLen := uint32(16 + len(ciphertext))
			header := make([]byte, 4+16)
			binary.BigEndian.PutUint32(header[0:4], packetLen)
			copy(header[4:20], counter)

			if _, wErr := conn.Write(header); wErr != nil {
				return fmt.Errorf("write header: %w", wErr)
			}
			if _, wErr := conn.Write(ciphertext); wErr != nil {
				return fmt.Errorf("write ciphertext: %w", wErr)
			}

			allCiphertext = append(allCiphertext, ciphertext...)
			sent += int64(n)
			if onProgress != nil {
				onProgress(sent, totalSize)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
	}

	// EOF Packet
	hmacSig := crypto.ComputeHMAC(allCiphertext, key)
	eofHeader := make([]byte, 20)
	binary.BigEndian.PutUint32(eofHeader[0:4], 36) // 16 counter + 32 hmac
	for i := 4; i < 20; i++ {
		eofHeader[i] = 0xFF
	}

	if _, err := conn.Write(eofHeader); err != nil {
		return fmt.Errorf("write EOF header: %w", err)
	}
	if _, err := conn.Write(hmacSig); err != nil {
		return fmt.Errorf("write HMAC: %w", err)
	}

	elapsed := time.Since(start).Seconds()
	if elapsed > 0 {
		log.Printf("[tcp_sender] sent %d bytes in %.1fs (%.1f MB/s)", sent, elapsed, float64(sent)/elapsed/1e6)
	}
	return nil
}
