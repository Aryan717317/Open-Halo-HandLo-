// transfer/chunker.go — Reads file, encrypts chunks, sends over WebRTC data channel
package transfer

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/gestureshare/sidecar/crypto"
	"github.com/pion/webrtc/v3"
)

const ChunkSize = 64 * 1024

func SendFile(
	dc *webrtc.DataChannel,
	filePath string,
	cipher *crypto.ChunkCipher,
	onProgress func(sent, total int64),
) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	stat, _ := file.Stat()
	totalSize := stat.Size()
	totalChunks := uint32((totalSize + ChunkSize - 1) / ChunkSize)

	buf := make([]byte, ChunkSize)
	var index uint32
	var sent int64
	start := time.Now()

	for {
		n, err := file.Read(buf)
		if n > 0 {
			encrypted, encErr := cipher.Encrypt(buf[:n])
			if encErr != nil {
				return fmt.Errorf("encrypt chunk %d: %w", index, encErr)
			}
			// Packet: [index:4][total:4][encSize:4][encrypted...]
			header := make([]byte, 12)
			binary.BigEndian.PutUint32(header[0:], index)
			binary.BigEndian.PutUint32(header[4:], totalChunks)
			binary.BigEndian.PutUint32(header[8:], uint32(len(encrypted)))
			packet := append(header, encrypted...)
			dc.Send(packet)
			sent += int64(n)
			index++
			if onProgress != nil {
				onProgress(sent, totalSize)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
	}

	elapsed := time.Since(start).Seconds()
	log.Printf("[transfer] sent %d bytes in %.1fs (%.1f MB/s)", sent, elapsed, float64(sent)/elapsed/1e6)
	return nil
}
