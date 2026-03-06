package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// WSManager handles WebSocket connections for various sessions.
type WSManager struct {
	mu          sync.Mutex
	connections map[string]*websocket.Conn // keyed by session token
}

func NewWSManager() *WSManager {
	return &WSManager{
		connections: make(map[string]*websocket.Conn),
	}
}

// HandleWS upgrades the HTTP connection to a WebSocket.
func (wm *WSManager) HandleWS(w http.ResponseWriter, r *http.Request) {
	// 1. Extract session token from query params or headers
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	// 2. Upgrade the connection
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // For self-signed certificates in dev
	})
	if err != nil {
		log.Printf("[WS] Failed to accept connection: %v", err)
		return
	}
	defer c.Close(websocket.StatusInternalError, "closing")

	wm.mu.Lock()
	wm.connections[token] = c
	wm.mu.Unlock()

	log.Printf("[WS] Client connected with token: %s", token)

	// 3. Keep the connection alive and handle incoming (mostly just ping-pong for now)
	ctx := r.Context()
	for {
		_, _, err := c.Read(ctx)
		if err != nil {
			log.Printf("[WS] Connection lost for token %s: %v", token, err)
			break
		}
	}

	wm.mu.Lock()
	delete(wm.connections, token)
	wm.mu.Unlock()
}

// PushMessage sends a JSON-encoded message to the specified session's client.
func (wm *WSManager) PushMessage(token string, msgType string, payload interface{}) error {
	wm.mu.Lock()
	c, ok := wm.connections[token]
	wm.mu.Unlock()

	if !ok {
		return fmt.Errorf("no active connection for token: %s", token)
	}

	msg := struct {
		Type    string      `json:"type"`
		Payload interface{} `json:"payload"`
	}{
		Type:    msgType,
		Payload: payload,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	return c.Write(ctx, websocket.MessageText, data)
}
