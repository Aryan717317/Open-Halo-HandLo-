// mdns/discovery.go
// Broadcasts this device on the local network and discovers peers.
// Uses hashicorp/mdns — no internet required, works on LAN/WiFi.

package mdns

import (
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/hashicorp/mdns"
)

const (
	ServiceType    = "_gestureshare._tcp"
	DefaultPort    = 47291
	ScanInterval   = 3 * time.Second
	ScanTimeout    = 2 * time.Second
)

type PeerInfo struct {
	ID      string
	Name    string
	Address string
	Port    int
	OS      string
	Code    string // 6-digit pairing code if set
}

type Discovery struct {
	server   *mdns.Server
	peers    map[string]*PeerInfo
	mu       sync.RWMutex
	stopCh   chan struct{}
	peersCh  chan PeerInfo
}

func NewDiscovery() *Discovery {
	return &Discovery{
		peers:   make(map[string]*PeerInfo),
		stopCh:  make(chan struct{}),
		peersCh: make(chan PeerInfo, 32),
	}
}

// Start advertises this device and begins scanning for peers.
// Returns a channel that emits newly found peers.
func (d *Discovery) Start() <-chan PeerInfo {
	go d.advertise()
	go d.scanLoop()
	return d.peersCh
}

func (d *Discovery) Stop() {
	close(d.stopCh)
	if d.server != nil {
		d.server.Shutdown()
	}
}

// advertise registers this device on the local mDNS network
func (d *Discovery) advertise() {
	hostname, _ := os.Hostname()
	deviceName := fmt.Sprintf("GestureShare-%s", hostname)

	info := []string{
		"version=1.0",
		"app=gestureshare",
		fmt.Sprintf("os=%s", getOS()),
		fmt.Sprintf("name=%s", hostname),
	}

	service, err := mdns.NewMDNSService(
		deviceName,
		ServiceType,
		"",       // domain — default .local
		"",       // host — auto
		DefaultPort,
		[]net.IP{},
		info,
	)
	if err != nil {
		log.Printf("[mdns] advertise error: %v", err)
		return
	}

	server, err := mdns.NewServer(&mdns.Config{Zone: service})
	if err != nil {
		log.Printf("[mdns] server error: %v", err)
		return
	}

	d.server = server
	log.Printf("[mdns] advertising as %s on port %d", deviceName, DefaultPort)
}

// scanLoop continuously scans for peers every ScanInterval
func (d *Discovery) scanLoop() {
	ticker := time.NewTicker(ScanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.scan()
		}
	}
}

func (d *Discovery) scan() {
	entries := make(chan *mdns.ServiceEntry, 16)
	found := make(map[string]bool)

	go func() {
		for entry := range entries {
			peer := PeerInfo{
				ID:      entry.Name,
				Name:    entry.Host,
				Address: entry.AddrV4.String(),
				Port:    entry.Port,
				OS:      extractField(entry.InfoFields, "os"),
			}

			d.mu.Lock()
			_, known := d.peers[peer.ID]
			if !known {
				d.peers[peer.ID] = &peer
				d.peersCh <- peer // notify router
				log.Printf("[mdns] new peer: %s @ %s:%d", peer.Name, peer.Address, peer.Port)
			}
			d.mu.Unlock()

			found[peer.ID] = true
		}
	}()

	params := &mdns.QueryParam{
		Service: ServiceType,
		Timeout: ScanTimeout,
		Entries: entries,
	}
	mdns.Query(params)
	close(entries)

	// Remove peers that are no longer visible
	d.mu.Lock()
	defer d.mu.Unlock()
	for id := range d.peers {
		if !found[id] {
			delete(d.peers, id)
		}
	}
}

func (d *Discovery) GetPeers() []PeerInfo {
	d.mu.RLock()
	defer d.mu.RUnlock()
	peers := make([]PeerInfo, 0, len(d.peers))
	for _, p := range d.peers {
		peers = append(peers, *p)
	}
	return peers
}

// FindByCode finds a peer advertising a specific 6-digit pairing code
func (d *Discovery) FindByCode(code string) *PeerInfo {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, p := range d.peers {
		if p.Code == code {
			return p
		}
	}
	return nil
}

func extractField(fields []string, key string) string {
	prefix := key + "="
	for _, f := range fields {
		if len(f) > len(prefix) && f[:len(prefix)] == prefix {
			return f[len(prefix):]
		}
	}
	return ""
}

func getOS() string {
	// runtime.GOOS returns "darwin", "windows", "linux"
	return "desktop"
}
