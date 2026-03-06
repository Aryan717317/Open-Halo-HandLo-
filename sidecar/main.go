package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gestureshare/sidecar/ipc"
	"github.com/gestureshare/sidecar/mdns"
	"github.com/gestureshare/sidecar/rest"
	"github.com/gestureshare/sidecar/session"
)

func main() {
	log.SetOutput(os.Stderr) // keep stdout clean for IPC
	log.Println("[sidecar] GestureShare networking core starting...")

	// Core services
	sessions := session.NewManager()
	discovery := mdns.NewDiscovery()

	server := rest.NewServer(sessions)
	port, err := server.Start()
	if err != nil {
		log.Fatalf("[sidecar] failed to start rest server: %v", err)
	}
	log.Printf("[sidecar] REST server listening on port %d", port)

	router := ipc.NewRouter(discovery, sessions, server)

	// Graceful shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Println("[sidecar] Shutting down...")
		server.Stop()
		discovery.Stop()
		os.Exit(0)
	}()

	// Block on IPC read loop — Tauri writes commands to stdin
	log.Println("[sidecar] Ready. Listening for IPC commands...")
	router.Listen()
}
