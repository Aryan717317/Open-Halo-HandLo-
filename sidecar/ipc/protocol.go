// ipc/protocol.go
// Newline-delimited JSON over stdin/stdout between Tauri (Rust) and Go sidecar.
// Tauri writes commands → Go stdin
// Go writes events    → Go stdout → Tauri reads

package ipc

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/gestureshare/sidecar/mdns"
	"github.com/gestureshare/sidecar/session"
	"github.com/gestureshare/sidecar/transfer"
)

// ── Message types ────────────────────────────────────────────────────────────

type MsgType string

const (
	// Inbound (Tauri → Go)
	CmdDiscover      MsgType = "CMD_DISCOVER"
	CmdStopDiscover  MsgType = "CMD_STOP_DISCOVER"
	CmdGetDeviceInfo MsgType = "CMD_GET_DEVICE_INFO"
	CmdPairRequest   MsgType = "CMD_PAIR_REQUEST"
	CmdPairAccept    MsgType = "CMD_PAIR_ACCEPT"
	CmdPairReject    MsgType = "CMD_PAIR_REJECT"
	CmdSendFile      MsgType = "CMD_SEND_FILE"
	CmdCancelTx      MsgType = "CMD_CANCEL_TX"
	CmdTxAccept      MsgType = "CMD_TX_ACCEPT"
	CmdTxReject      MsgType = "CMD_TX_REJECT"

	// Outbound (Go → Tauri)
	EvtDeviceInfo   MsgType = "EVT_DEVICE_INFO"
	EvtPeerFound    MsgType = "EVT_PEER_FOUND"
	EvtPeerLost     MsgType = "EVT_PEER_LOST"
	EvtPairIncoming MsgType = "EVT_PAIR_INCOMING"
	EvtPairSuccess  MsgType = "EVT_PAIR_SUCCESS"
	EvtPairRejected MsgType = "EVT_PAIR_REJECTED"
	EvtTxOffer      MsgType = "EVT_TX_OFFER"
	EvtTxProgress   MsgType = "EVT_TX_PROGRESS"
	EvtTxComplete   MsgType = "EVT_TX_COMPLETE"
	EvtTxCancelled  MsgType = "EVT_TX_CANCELLED"
	EvtTxError      MsgType = "EVT_TX_ERROR"
	EvtError        MsgType = "EVT_ERROR"
)

// ── Envelope ─────────────────────────────────────────────────────────────────

type IPCMessage struct {
	Type    MsgType         `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// ── Payload shapes ───────────────────────────────────────────────────────────

type DeviceInfoPayload struct {
	Name    string `json:"name"`
	OS      string `json:"os"`
	Version string `json:"version"`
	LocalIP string `json:"local_ip"`
}

type PeerPayload struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	OS      string `json:"os"`
}

type PairRequestPayload struct {
	PeerID         string `json:"peer_id"`
	PeerName       string `json:"peer_name"`
	PeerAddr       string `json:"address,omitempty"`
	PeerPort       int    `json:"port,omitempty"`
	Code           string `json:"code,omitempty"`
	PublicKey      string `json:"public_key,omitempty"`
	TLSFingerprint string `json:"tls_fingerprint,omitempty"`
}

type SendFilePayload struct {
	TransferID string `json:"transfer_id"`
	PeerID     string `json:"peer_id"`
	FilePath   string `json:"file_path"`
	FileName   string `json:"file_name"`
	FileSize   int64  `json:"file_size"`
	MimeType   string `json:"mime_type"`
}

type TransferOfferPayload struct {
	TransferID    string `json:"transfer_id"`
	PeerID        string `json:"peer_id"`
	FileName      string `json:"file_name"`
	FileSize      int64  `json:"file_size"`
	TcpPort       int    `json:"tcp_port"`
	SenderAddress string `json:"sender_address,omitempty"`
}

type ProgressPayload struct {
	TransferID string  `json:"transfer_id"`
	BytesSent  int64   `json:"bytes_sent"`
	TotalBytes int64   `json:"total_bytes"`
	Percent    float64 `json:"percent"`
	SpeedBps   int64   `json:"speed_bps"`
	EtaSeconds float64 `json:"eta_seconds"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ── Router ───────────────────────────────────────────────────────────────────

type TransferService interface {
	ResolveOffer(transferID string, accepted bool)
}

// Router dispatches incoming IPC messages to the correct handler.
type Router struct {
	discovery       *mdns.Discovery
	sessions        *session.Manager
	transferService TransferService
}

// NewRouter creates a new IPC router.
func NewRouter(discovery *mdns.Discovery, sessions *session.Manager, ts TransferService) *Router {
	return &Router{
		discovery:       discovery,
		sessions:        sessions,
		transferService: ts,
	}
}

// Listen blocks, reading newline-delimited JSON from stdin.
func (r *Router) Listen() {
	scanner := bufio.NewScanner(os.Stdin)
	// Increase scanner buffer to handle large payloads
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var msg IPCMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			log.Printf("[ipc] bad message: %v", err)
			continue
		}
		go r.handle(msg)
	}
	if err := scanner.Err(); err != nil {
		log.Printf("[ipc] stdin read error: %v", err)
	}
}

func (r *Router) handle(msg IPCMessage) {
	log.Printf("[ipc] received: %s", msg.Type)
	switch msg.Type {
	case CmdGetDeviceInfo:
		r.handleDeviceInfo()
	case CmdDiscover:
		r.handleDiscover()
	case CmdStopDiscover:
		r.discovery.Stop()
	case CmdPairRequest:
		var p PairRequestPayload
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			emitError("PAIR_REQUEST_PARSE", fmt.Sprintf("bad payload: %v", err))
			return
		}
		r.handlePairRequest(p)
	case CmdPairAccept:
		var p map[string]string
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			emitError("PAIR_ACCEPT_PARSE", fmt.Sprintf("bad payload: %v", err))
			return
		}
		r.handlePairAccept(p["peer_id"])
	case CmdPairReject:
		var p map[string]string
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			emitError("PAIR_REJECT_PARSE", fmt.Sprintf("bad payload: %v", err))
			return
		}
		r.handlePairReject(p["peer_id"])
	case CmdSendFile:
		var p SendFilePayload
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			emitError("SEND_FILE_PARSE", fmt.Sprintf("bad payload: %v", err))
			return
		}
		r.handleSendFile(p)
	case CmdCancelTx:
		// TODO Phase 3: cancel active transfer
		log.Println("[ipc] cancel transfer — not yet implemented")
	case CmdTxAccept:
		var p struct {
			TransferID    string `json:"transfer_id"`
			SenderAddress string `json:"sender_address"`
			TcpPort       int    `json:"tcp_port"`
			SavePath      string `json:"save_path"`
			FileSize      int64  `json:"file_size"`
		}
		if err := json.Unmarshal(msg.Payload, &p); err == nil {
			r.transferService.ResolveOffer(p.TransferID, true)

			// Start Receiver
			testKey := make([]byte, 32)
			copy(testKey, "phase3_test_key_32_bytes_long!!!")

			go func() {
				err := transfer.ReceiveFileTCP(p.SenderAddress, p.TcpPort, testKey, p.SavePath, func(received, total int64) {
					Emit(EvtTxProgress, ProgressPayload{
						TransferID: p.TransferID,
						BytesSent:  received,
						TotalBytes: total,
						Percent:    float64(received) / float64(total) * 100,
					})
				}, p.FileSize)

				if err != nil {
					emitError("TCP_RECEIVER_ERROR", err.Error())
				} else {
					Emit(EvtTxComplete, map[string]string{"transfer_id": p.TransferID})
				}
			}()
		}
	case CmdTxReject:
		var p map[string]string
		if err := json.Unmarshal(msg.Payload, &p); err == nil {
			r.transferService.ResolveOffer(p["transfer_id"], false)
		}
	default:
		log.Printf("[ipc] unknown command: %s", msg.Type)
	}
}

// ── Handlers ─────────────────────────────────────────────────────────────────

func (r *Router) handleDeviceInfo() {
	Emit(EvtDeviceInfo, DeviceInfoPayload{
		Name:    getDeviceName(),
		OS:      runtime.GOOS,
		Version: "0.1.0",
		LocalIP: getLocalIP(),
	})
}

func (r *Router) handleDiscover() {
	peers, lost := r.discovery.Start()
	go func() {
		for {
			select {
			case peer := <-peers:
				Emit(EvtPeerFound, PeerPayload{
					ID:      peer.ID,
					Name:    peer.Name,
					Address: peer.Address,
					Port:    peer.Port,
					OS:      peer.OS,
				})
			case id := <-lost:
				Emit(EvtPeerLost, map[string]string{"id": id})
			}
		}
	}()
}

func (r *Router) handlePairRequest(p PairRequestPayload) {
	peers := r.discovery.GetPeers()
	var target *mdns.PeerInfo
	for _, peer := range peers {
		if peer.ID == p.PeerID {
			target = &peer
			break
		}
	}

	if target == nil {
		emitError("PAIR_REQUEST_NA", fmt.Sprintf("peer %s not found in discovery", p.PeerID))
		return
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	body, _ := json.Marshal(map[string]string{
		"peerId":   getDeviceName(),
		"peerName": getDeviceName(),
		"code":     p.Code,
	})

	url := fmt.Sprintf("https://%s:%d/api/v1/pair/request", target.Address, target.Port)
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		emitError("PAIR_REQUEST_FAIL", fmt.Sprintf("POST failed: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		emitError("PAIR_REQUEST_REJECT", fmt.Sprintf("peer returned %d", resp.StatusCode))
		return
	}

	log.Printf("[ipc] pair request to peer %s successful", p.PeerID)
	Emit(EvtPairSuccess, map[string]string{"peer_id": p.PeerID, "peer_name": target.Name})
}

func (r *Router) handlePairAccept(peerID string) {
	log.Printf("[ipc] pair accept for peer %s", peerID)
	Emit(EvtPairSuccess, map[string]string{"peer_id": peerID})
}

func (r *Router) handlePairReject(peerID string) {
	log.Printf("[ipc] pair reject for peer %s", peerID)
	Emit(EvtPairRejected, map[string]string{"peer_id": peerID})
}

func (r *Router) handleSendFile(p SendFilePayload) {
	log.Printf("[ipc] starting tcp transfer for %s to peer %s", p.FileName, p.PeerID)

	// In a real app, we'd use the derived session key.
	// For Phase 3, we'll use a test key (32 bytes).
	testKey := make([]byte, 32)
	copy(testKey, "phase3_test_key_32_bytes_long!!!")

	// 1. Start TCP Sender
	port, err := transfer.SendFileTCP(p.FilePath, testKey, func(sent, total int64) {
		Emit(EvtTxProgress, ProgressPayload{
			TransferID: p.TransferID,
			BytesSent:  sent,
			TotalBytes: total,
			Percent:    float64(sent) / float64(total) * 100,
		})
	})

	if err != nil {
		emitError("TCP_SENDER_START", err.Error())
		return
	}

	// 2. Find peer address
	peers := r.discovery.GetPeers()
	var target *mdns.PeerInfo
	for _, peer := range peers {
		if peer.ID == p.PeerID {
			target = &peer
			break
		}
	}

	if target == nil {
		emitError("PEER_NOT_FOUND", "peer not in discovery cache")
		return
	}

	// 3. Send REST offer
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}

	offerBody, _ := json.Marshal(map[string]interface{}{
		"transfer_id": p.TransferID,
		"file_name":   p.FileName,
		"file_size":   p.FileSize,
		"mime_type":   p.MimeType,
		"tcp_port":    port,
	})

	offerURL := fmt.Sprintf("https://%s:%d/api/v1/transfer/offer", target.Address, target.Port)
	req, _ := http.NewRequest("POST", offerURL, bytes.NewBuffer(offerBody))

	// Add auth token if we have one
	if s, ok := r.sessions.Get(p.PeerID); ok {
		req.Header.Set("X-GestureShare-Token", s.Token)
	}

	resp, err := client.Do(req)
	if err != nil {
		emitError("TRANSFER_OFFER_FAILED", err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		emitError("TRANSFER_OFFER_REJECTED", fmt.Sprintf("peer status %d", resp.StatusCode))
		return
	}

	log.Printf("[ipc] transfer offer accepted by peer, TCP stream should start")
}

// ── Emit ─────────────────────────────────────────────────────────────────────

// Emit writes a JSON event to stdout for Tauri to consume.
func Emit(t MsgType, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[ipc] marshal payload error: %v", err)
		return
	}
	msg := IPCMessage{Type: t, Payload: data}
	out, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[ipc] marshal message error: %v", err)
		return
	}
	fmt.Println(string(out)) // newline-delimited
}

func emitError(code, message string) {
	Emit(EvtError, ErrorPayload{Code: code, Message: message})
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func getDeviceName() string {
	name, err := os.Hostname()
	if err != nil {
		return "Unknown Device"
	}
	return name
}

// getLocalIP returns the first non-loopback, non-link-local IPv4 address.
// Algorithm follows WIRE_PROTOCOL.md § Local IP Selection.
func getLocalIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ip4 := ipnet.IP.To4(); ip4 != nil {
					if !ip4.IsLinkLocalUnicast() {
						return ip4.String()
					}
				}
			}
		}
	}
	return "127.0.0.1"
}
