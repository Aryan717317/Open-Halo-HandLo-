// transfer/receiver.go — Receives encrypted chunks and reassembles file
package transfer

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/gestureshare/sidecar/crypto"
)

type Receiver struct {
	cipher     *crypto.ChunkCipher
	chunks     map[uint32][]byte
	totalChunks uint32
	mu         sync.Mutex
	outputDir  string
	fileName   string
	done       chan struct{}
}

func NewReceiver(cipher *crypto.ChunkCipher, outputDir, fileName string) *Receiver {
	return &Receiver{
		cipher:    cipher,
		chunks:    make(map[uint32][]byte),
		outputDir: outputDir,
		fileName:  fileName,
		done:      make(chan struct{}),
	}
}

// HandleChunk processes a raw packet from the WebRTC data channel
func (r *Receiver) HandleChunk(packet []byte) error {
	if len(packet) < 12 {
		return fmt.Errorf("packet too short")
	}

	index := binary.BigEndian.Uint32(packet[0:4])
	total := binary.BigEndian.Uint32(packet[4:8])
	encSize := binary.BigEndian.Uint32(packet[8:12])
	encrypted := packet[12 : 12+encSize]

	plaintext, err := r.cipher.Decrypt(encrypted)
	if err != nil {
		return fmt.Errorf("decrypt chunk %d: %w", index, err)
	}

	r.mu.Lock()
	r.chunks[index] = plaintext
	r.totalChunks = total
	received := uint32(len(r.chunks))
	r.mu.Unlock()

	log.Printf("[receiver] chunk %d/%d received", index+1, total)

	if received == total {
		return r.assemble()
	}
	return nil
}

func (r *Receiver) assemble() error {
	outPath := filepath.Join(r.outputDir, r.fileName)
	file, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer file.Close()

	r.mu.Lock()
	defer r.mu.Unlock()

	for i := uint32(0); i < r.totalChunks; i++ {
		chunk, ok := r.chunks[i]
		if !ok {
			return fmt.Errorf("missing chunk %d", i)
		}
		if _, err := file.Write(chunk); err != nil {
			return fmt.Errorf("write chunk %d: %w", i, err)
		}
	}

	log.Printf("[receiver] file assembled: %s", outPath)
	close(r.done)
	return nil
}

func (r *Receiver) Wait() <-chan struct{} {
	return r.done
}
