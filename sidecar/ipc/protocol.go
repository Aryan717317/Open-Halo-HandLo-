// ipc/protocol.go
// Newline-delimited JSON over stdin/stdout between Tauri (Rust) and Go sidecar.
// Tauri writes commands → Go stdin
// Go writes events    → Go stdout → Tauri reads

package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/gestureshare/sidecar/mdns"
	"github.com/gestureshare/sidecar/webrtc"
)

// ── Message types ────────────────────────────────────────────────────────────

type MsgType string

const (
	// Inbound (Tauri → Go)
	CmdDiscover     MsgType = "CMD_DISCOVER"
	CmdStopDiscover MsgType = "CMD_STOP_DISCOVER"
	CmdPairRequest  MsgType = "CMD_PAIR_REQUEST"
	CmdPairAccept   MsgType = "CMD_PAIR_ACCEPT"
	CmdPairReject   MsgType = "CMD_PAIR_REJECT"
	CmdSendFile     MsgType = "CMD_SEND_FILE"
	CmdCancelTx     MsgType = "CMD_CANCEL_TX"
	CmdGetDeviceInfo MsgType = "CMD_GET_DEVICE_INFO"

	// Outbound (Go → Tauri)
	EvtPeerFound    MsgType = "EVT_PEER_FOUND"
	EvtPeerLost     MsgType = "EVT_PEER_LOST"
	EvtPairIncoming MsgType = "EVT_PAIR_INCOMING"
	EvtPairSuccess  MsgType = "EVT_PAIR_SUCCESS"
	EvtPairRejected MsgType = "EVT_PAIR_REJECTED"
	EvtTxOffer      MsgType = "EVT_TX_OFFER"
	EvtTxProgress   MsgType = "EVT_TX_PROGRESS"
	EvtTxComplete   MsgType = "EVT_TX_COMPLETE"
	EvtTxError      MsgType = "EVT_TX_ERROR"
	EvtDeviceInfo   MsgType = "EVT_DEVICE_INFO"
	EvtError        MsgType = "EVT_ERROR"
)

type IPCMessage struct {
	Type    MsgType         `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// ── Payload shapes ───────────────────────────────────────────────────────────

type PeerPayload struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	OS      string `json:"os"`
}

type PairRequestPayload struct {
	PeerID    string `json:"peer_id"`
	PeerName  string `json:"peer_name"`
	Code      string `json:"code,omitempty"` // 6-digit text code
	PublicKey string `json:"public_key"`     // hex-encoded ECDH pubkey
	TLSFingerprint string `json:"tls_fingerprint"`
}

type SendFilePayload struct {
	TransferID string `json:"transfer_id"`
	PeerID     string `json:"peer_id"`
	FilePath   string `json:"file_path"`
	FileName   string `json:"file_name"`
	FileSize   int64  `json:"file_size"`
	MimeType   string `json:"mime_type"`
}

type ProgressPayload struct {
	TransferID string  `json:"transfer_id"`
	BytesSent  int64   `json:"bytes_sent"`
	TotalBytes int64   `json:"total_bytes"`
	Percent    float64 `json:"percent"`
	SpeedBps   int64   `json:"speed_bps"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ── Router ───────────────────────────────────────────────────────────────────

type Router struct {
	discovery  *mdns.Discovery
	rtcManager *webrtc.Manager
}

func NewRouter(d *mdns.Discovery, r *webrtc.Manager) *Router {
	return &Router{discovery: d, rtcManager: r}
}

// Listen blocks, reading newline-delimited JSON from stdin
func (r *Router) Listen() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()
		var msg IPCMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			log.Printf("[ipc] bad message: %v", err)
			continue
		}
		go r.handle(msg)
	}
}

func (r *Router) handle(msg IPCMessage) {
	log.Printf("[ipc] received: %s", msg.Type)
	switch msg.Type {
	case CmdDiscover:
		r.handleDiscover()
	case CmdStopDiscover:
		r.discovery.Stop()
	case CmdPairRequest:
		var p PairRequestPayload
		json.Unmarshal(msg.Payload, &p)
		r.handlePairRequest(p)
	case CmdSendFile:
		var p SendFilePayload
		json.Unmarshal(msg.Payload, &p)
		r.handleSendFile(p)
	case CmdGetDeviceInfo:
		r.handleDeviceInfo()
	default:
		log.Printf("[ipc] unknown command: %s", msg.Type)
	}
}

// ── Handlers ─────────────────────────────────────────────────────────────────

func (r *Router) handleDiscover() {
	peers := r.discovery.Start()
	go func() {
		for peer := range peers {
			Emit(EvtPeerFound, PeerPayload{
				ID:      peer.ID,
				Name:    peer.Name,
				Address: peer.Address,
				Port:    peer.Port,
				OS:      peer.OS,
			})
		}
	}()
}

func (r *Router) handlePairRequest(p PairRequestPayload) {
	// TODO Phase 2: initiate HTTPS pairing handshake to peer
	log.Printf("[ipc] pair request to peer %s", p.PeerID)
	Emit(EvtPairSuccess, map[string]string{"peer_id": p.PeerID})
}

func (r *Router) handleSendFile(p SendFilePayload) {
	// TODO Phase 4: ECDH key exchange → WebRTC data channel → chunk + encrypt
	log.Printf("[ipc] send file %s to peer %s", p.FileName, p.PeerID)
}

func (r *Router) handleDeviceInfo() {
	info := map[string]string{
		"name":    getDeviceName(),
		"version": "0.1.0",
	}
	Emit(EvtDeviceInfo, info)
}

// ── Emit ─────────────────────────────────────────────────────────────────────

// Emit writes a JSON event to stdout for Tauri to consume
func Emit(t MsgType, payload any) {
	data, _ := json.Marshal(payload)
	msg := IPCMessage{Type: t, Payload: data}
	out, _ := json.Marshal(msg)
	fmt.Println(string(out)) // newline-delimited
}

func getDeviceName() string {
	name, err := os.Hostname()
	if err != nil {
		return "Unknown Device"
	}
	return name
}
