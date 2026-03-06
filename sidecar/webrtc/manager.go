// webrtc/manager.go
// Manages WebRTC peer connections for P2P file transfer.
// Uses pion/webrtc — no STUN/TURN needed on LAN.

package webrtc

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/pion/webrtc/v3"
)

// LAN-only config — no ICE servers needed
var rtcConfig = webrtc.Configuration{}

type Manager struct {
	connections map[string]*webrtc.PeerConnection
	mu          sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		connections: make(map[string]*webrtc.PeerConnection),
	}
}

// CreateSender sets up a WebRTC peer connection as the sending side.
// Exchanges SDP with receiver over local TCP (no STUN).
func (m *Manager) CreateSender(
	peerID string,
	peerAddr string,
	peerPort int,
	onProgress func(sent, total int64),
	onComplete func(),
) error {
	pc, err := webrtc.NewPeerConnection(rtcConfig)
	if err != nil {
		return fmt.Errorf("create peer connection: %w", err)
	}

	m.mu.Lock()
	m.connections[peerID] = pc
	m.mu.Unlock()

	// Ordered, reliable data channel for file transfer
	ordered := true
	dc, err := pc.CreateDataChannel("file-transfer", &webrtc.DataChannelInit{
		Ordered: &ordered,
	})
	if err != nil {
		return fmt.Errorf("create data channel: %w", err)
	}

	dc.OnOpen(func() {
		log.Printf("[webrtc] data channel open to %s", peerID)
	})

	dc.OnClose(func() {
		log.Printf("[webrtc] data channel closed to %s", peerID)
		onComplete()
	})

	// Create SDP offer
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("create offer: %w", err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("set local desc: %w", err)
	}

	// Exchange SDP over local TCP — replaces STUN entirely on LAN
	answer, err := exchangeSDPAsSender(offer, peerAddr, peerPort)
	if err != nil {
		return fmt.Errorf("SDP exchange: %w", err)
	}

	return pc.SetRemoteDescription(answer)
}

// CreateReceiver listens for an incoming WebRTC SDP offer over local TCP.
func (m *Manager) CreateReceiver(
	listenPort int,
	onDataChannel func(*webrtc.DataChannel),
) error {
	pc, err := webrtc.NewPeerConnection(rtcConfig)
	if err != nil {
		return fmt.Errorf("create peer connection: %w", err)
	}

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		log.Printf("[webrtc] incoming data channel: %s", dc.Label())
		onDataChannel(dc)
	})

	// Listen for SDP offer on local TCP
	return listenForSDPOffer(pc, listenPort)
}

// ── Local TCP SDP exchange (replaces STUN on LAN) ────────────────────────────

func exchangeSDPAsSender(
	offer webrtc.SessionDescription,
	peerAddr string,
	peerPort int,
) (webrtc.SessionDescription, error) {
	addr := fmt.Sprintf("%s:%d", peerAddr, peerPort)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return webrtc.SessionDescription{}, fmt.Errorf("dial peer: %w", err)
	}
	defer conn.Close()

	// Send offer
	enc := json.NewEncoder(conn)
	if err := enc.Encode(offer); err != nil {
		return webrtc.SessionDescription{}, fmt.Errorf("send offer: %w", err)
	}

	// Receive answer
	var answer webrtc.SessionDescription
	dec := json.NewDecoder(conn)
	if err := dec.Decode(&answer); err != nil {
		return webrtc.SessionDescription{}, fmt.Errorf("receive answer: %w", err)
	}

	log.Printf("[webrtc] SDP exchange complete with %s", addr)
	return answer, nil
}

func listenForSDPOffer(pc *webrtc.PeerConnection, port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer ln.Close()

	log.Printf("[webrtc] waiting for SDP offer on port %d", port)
	conn, err := ln.Accept()
	if err != nil {
		return fmt.Errorf("accept: %w", err)
	}
	defer conn.Close()

	// Receive offer
	var offer webrtc.SessionDescription
	if err := json.NewDecoder(conn).Decode(&offer); err != nil {
		return fmt.Errorf("decode offer: %w", err)
	}

	if err := pc.SetRemoteDescription(offer); err != nil {
		return fmt.Errorf("set remote desc: %w", err)
	}

	// Send answer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return fmt.Errorf("create answer: %w", err)
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		return fmt.Errorf("set local desc: %w", err)
	}

	return json.NewEncoder(conn).Encode(answer)
}

func (m *Manager) Close(peerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if pc, ok := m.connections[peerID]; ok {
		pc.Close()
		delete(m.connections, peerID)
	}
}
