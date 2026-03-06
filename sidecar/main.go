package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gestureshare/sidecar/ipc"
	"github.com/gestureshare/sidecar/mdns"
	"github.com/gestureshare/sidecar/webrtc"
)

func main() {
	log.SetOutput(os.Stderr) // keep stdout clean for IPC
	log.Println("[sidecar] GestureShare networking core starting...")

	// Core services
	discovery := mdns.NewDiscovery()
	rtcManager := webrtc.NewManager()
	router := ipc.NewRouter(discovery, rtcManager)

	// Graceful shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Println("[sidecar] Shutting down...")
		discovery.Stop()
		os.Exit(0)
	}()

	// Block on IPC read loop — Tauri writes commands to stdin
	log.Println("[sidecar] Ready. Listening for IPC commands...")
	router.Listen()
}
