package rest

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gestureshare/sidecar/clipboard"
	"github.com/gestureshare/sidecar/ipc"
	"github.com/gestureshare/sidecar/session"
)

// Server runs the local HTTPS REST API
type Server struct {
	httpServer    *http.Server
	sessions      *session.Manager
	pendingOffers map[string]chan bool   // transferID -> decision
	pendingPairs  map[string]chan string // peerID -> responderPubKey (or empty if rejected)
	qrURL         string
	textCode      string
	mu            sync.Mutex
	certPEM       []byte
	certFP        string // SHA-256 fingerprint for TOFU
	wsManager     *WSManager
	clipboardSync *clipboard.ClipboardSync
}

func NewServer(sessions *session.Manager) *Server {
	return &Server{
		sessions:      sessions,
		pendingOffers: make(map[string]chan bool),
		pendingPairs:  make(map[string]chan string),
		wsManager:     NewWSManager(),
	}
}

// Start initialises TLS, binds to a free port, returns the port
func (s *Server) Start() (int, error) {
	cert, key, fp, err := generateSelfSignedCert()
	if err != nil {
		return 0, fmt.Errorf("cert generation: %w", err)
	}
	s.certFP = fp

	tlsCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return 0, err
	}

	// Bind to random free port
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return 0, err
	}
	port := listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	// Generate pairing data
	sessionID := generateToken() // 32 chars hex
	s.textCode = fmt.Sprintf("%06d", generateRandomInt(1000000))

	ip, _ := getLocalIP()
	s.qrURL = fmt.Sprintf("https://%s:%d/join#sid=%s&fp=%s", ip, port, sessionID, s.certFP)

	s.httpServer = &http.Server{
		Handler: mux,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{tlsCert},
			MinVersion:   tls.VersionTLS13,
		},
	}

	go func() {
		tlsListener := tls.NewListener(listener, s.httpServer.TLSConfig)
		if err := s.httpServer.Serve(tlsListener); err != nil && err != http.ErrServerClosed {
			log.Printf("[rest] server error: %v", err)
		}
	}()

	// Initialize Clipboard Sync
	cs, err := clipboard.NewClipboardSync(func(text string) {
		// On local clipboard change, push to all paired mobile clients
		log.Printf("[Clipboard] Local change detected: %s", text)
		// We'll push to all sessions for now. In a multi-device setup, we'd filter.
		s.sessions.Iterate(func(sess *session.Session) {
			if sess.IsPaired && sess.PeerID[:6] == "mobile" {
				s.wsManager.PushMessage(sess.Token, "CLIPBOARD_RX", map[string]string{"text": text})
			}
		})
	})
	if err == nil {
		s.clipboardSync = cs
		go s.clipboardSync.Start(context.Background())
	} else {
		log.Printf("[rest] failed to init clipboard: %v", err)
	}

	return port, nil
}

func (s *Server) GetPairingData() (string, string) {
	return s.qrURL, s.textCode
}

func (s *Server) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	s.httpServer.Shutdown(ctx)
}

// ── Route registration ────────────────────────────────────────────

func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Device info — no auth required
	mux.HandleFunc("GET /api/v1/device/info", s.handleDeviceInfo)
	mux.HandleFunc("GET /api/v1/device/ping", s.handlePing)

	// Pairing — no auth required (bootstraps the session)
	mux.HandleFunc("GET /api/v1/pair/data", s.handleGetPairingData)
	mux.HandleFunc("POST /api/v1/pair/request", s.handlePairRequest)
	mux.HandleFunc("POST /api/v1/session/register", s.handleSessionRegister)
	mux.HandleFunc("POST /api/v1/pair/accept", s.withAuth(s.handlePairAccept))
	mux.HandleFunc("POST /api/v1/pair/reject", s.withAuth(s.handlePairReject))

	// Transfer — auth required
	mux.HandleFunc("POST /api/v1/transfer/offer", s.withAuth(s.handleTransferOffer))
	mux.HandleFunc("POST /api/v1/transfer/accept", s.withAuth(s.handleTransferAccept))
	mux.HandleFunc("POST /api/v1/transfer/reject", s.withAuth(s.handleTransferReject))
	mux.HandleFunc("POST /api/v1/transfer/cancel", s.withAuth(s.handleTransferCancel))

	// Clipboard — auth required
	mux.HandleFunc("POST /api/v1/clipboard/push", s.withAuth(s.handleClipboardPush))

	// WebSocket — upgrades connection, auth via query param
	mux.HandleFunc("GET /ws", s.handleWS)
}

// ── Handlers ─────────────────────────────────────────────────────

func (s *Server) handleGetPairingData(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{
		"qrUrl":    s.qrURL,
		"textCode": s.textCode,
	})
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) handleDeviceInfo(w http.ResponseWriter, r *http.Request) {
	hostname, _ := net.LookupAddr("127.0.0.1")
	name := "Unknown"
	if len(hostname) > 0 {
		name = hostname[0]
	}
	writeJSON(w, 200, map[string]interface{}{
		"name":            name,
		"app":             "gestureshare",
		"version":         "1.0.0",
		"certFingerprint": s.certFP,
	})
}

func (s *Server) handleSessionRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PublicKey string `json:"publicKey"` // base64url-no-padding raw public key bytes
		SessionID string `json:"sessionId"` // if from QR
		Code      string `json:"code"`      // if from Text Code
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}

	// Validate code if provided
	if body.Code != "" && body.Code != s.textCode {
		writeJSON(w, 403, map[string]string{"error": "INVALID_CODE"})
		return
	}

	token := generateToken()
	peerID := "mobile-" + generateToken()[:8]

	s.sessions.Add(&session.Session{
		PeerID:        peerID,
		Token:         token,
		TempPublicKey: body.PublicKey,
		IsPaired:      true,
	})

	ipc.Emit(ipc.EvtPairSuccess, ipc.PairSuccessPayload{
		PeerID:   peerID,
		PeerName: "Mobile Client",
	})

	writeJSON(w, 200, map[string]string{
		"token":       token,
		"desktopName": "Desktop", // Should ideally be from deviceInfo
	})
}

func (s *Server) handlePairRequest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PeerID    string `json:"peerId"`
		PeerName  string `json:"peerName"`
		PublicKey string `json:"publicKey"`
		Code      string `json:"code,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}

	ch := make(chan string)
	s.mu.Lock()
	s.pendingPairs[body.PeerID] = ch
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.pendingPairs, body.PeerID)
		s.mu.Unlock()
	}()

	token := generateToken()
	s.sessions.Add(&session.Session{
		PeerID:        body.PeerID,
		Token:         token,
		TempPublicKey: body.PublicKey,
	})

	ipc.Emit(ipc.EvtPairIncoming, ipc.PairRequestPayload{
		PeerID:   body.PeerID,
		PeerName: body.PeerName,
		PeerAddr: r.RemoteAddr,
	})

	select {
	case responderPubKey := <-ch:
		if responderPubKey != "" {
			writeJSON(w, 200, map[string]string{
				"status":    "accepted",
				"token":     token,
				"publicKey": responderPubKey,
			})
		} else {
			writeJSON(w, 403, map[string]string{"status": "rejected"})
		}
	case <-time.After(60 * time.Second):
		writeJSON(w, 408, map[string]string{"error": "timeout"})
	}
}

func (s *Server) ResolvePair(peerID string, responderPubKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch, ok := s.pendingPairs[peerID]; ok {
		select {
		case ch <- responderPubKey:
		default:
		}
	}
}

func (s *Server) handlePairAccept(w http.ResponseWriter, r *http.Request) {
	// Not used by initiator in blocking flow, but keep for legacy or explicit confirmation
	writeJSON(w, 200, map[string]string{"status": "accepted"})
}

func (s *Server) handlePairReject(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "rejected"})
}

func (s *Server) handleTransferOffer(w http.ResponseWriter, r *http.Request) {
	var offer ipc.TransferOfferPayload
	if err := json.NewDecoder(r.Body).Decode(&offer); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}

	ch := make(chan bool)
	s.mu.Lock()
	s.pendingOffers[offer.TransferID] = ch
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.pendingOffers, offer.TransferID)
		s.mu.Unlock()
	}()

	// Include sender IP from RemoteAddr
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	offer.SenderAddress = host

	ipc.Emit(ipc.EvtTxOffer, offer)

	select {
	case accepted := <-ch:
		if accepted {
			writeJSON(w, 200, map[string]string{"status": "accepted"})
		} else {
			writeJSON(w, 200, map[string]string{"status": "rejected"})
		}
	case <-time.After(30 * time.Second):
		writeJSON(w, 408, map[string]string{"error": "timeout"})
	}
}

func (s *Server) ResolveOffer(transferID string, accepted bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch, ok := s.pendingOffers[transferID]; ok {
		select {
		case ch <- accepted:
		default:
		}
	}
}

func (s *Server) handleTransferAccept(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TransferID string `json:"transferId"`
		ECDHPubKey string `json:"ecdhPubKey"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	ipc.Emit("EVT_TX_ACCEPT", body) // Placeholder event
	writeJSON(w, 200, map[string]string{"status": "accepted"})
}

func (s *Server) handleTransferReject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TransferID string `json:"transferId"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	ipc.Emit("EVT_TX_REJECT", body) // Placeholder event
	writeJSON(w, 200, map[string]string{"status": "rejected"})
}

func (s *Server) handleTransferCancel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TransferID string `json:"transferId"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	ipc.Emit(ipc.EvtTxCancelled, body)
	writeJSON(w, 200, map[string]string{"status": "cancelled"})
}

func (s *Server) handleClipboardPush(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}

	if s.clipboardSync != nil {
		s.clipboardSync.Write(body.Text)
	}

	// Notify Tauri
	ipc.Emit(ipc.EvtClipboardRx, map[string]string{"text": body.Text})

	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	s.wsManager.HandleWS(w, r)
}

// ── Middleware ────────────────────────────────────────────────────

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-GestureShare-Token")
		if _, ok := s.sessions.GetByToken(token); !ok {
			writeJSON(w, 401, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

// ── TLS Helpers ───────────────────────────────────────────────────

func generateSelfSignedCert() (certPEM, keyPEM []byte, fingerprint string, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{Organization: []string{"GestureShare"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour), // session-scoped cert
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	privDER, _ := x509.MarshalECPrivateKey(priv)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER})

	// Fingerprint for TOFU verification on peer side
	cert, _ := x509.ParseCertificate(certDER)
	fp := fmt.Sprintf("%x", cert.Raw[:8]) // abbreviated for display
	fingerprint = fp

	return
}

// ── Utility ───────────────────────────────────────────────────────

func generateRandomInt(max int) int {
	b := make([]byte, 8)
	rand.Read(b)
	n := big.NewInt(0).SetBytes(b)
	return int(n.Int64() % int64(max))
}

func getLocalIP() (string, error) {
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ip4 := ipnet.IP.To4(); ip4 != nil {
					if !ip4.IsLinkLocalUnicast() {
						return ip4.String(), nil
					}
				}
			}
		}
	}
	return "127.0.0.1", fmt.Errorf("no suitable network interface found")
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// PairWith satisfies the PairingService interface used by the IPC router
func (s *Server) PairWith(peerID, peerAddr string, peerPort int) error { return nil }
func (s *Server) AcceptPair(peerID string) error                       { return nil }
func (s *Server) RejectPair(peerID string) error                       { return nil }
