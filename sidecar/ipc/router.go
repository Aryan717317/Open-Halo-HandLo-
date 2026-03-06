package ipc

import (
	"encoding/json"
	"log"
)

// Handler interface that services must implement
type DiscoveryService interface {
	StartDiscovery()
	StopDiscovery()
}

type RTCService interface {
	SendFile(transferID, peerID, filePath, ecdhPubKey string) error
	CancelTransfer(transferID string)
}

type PairingService interface {
	PairWith(peerID, peerAddr string, peerPort int) error
	AcceptPair(peerID string) error
	RejectPair(peerID string) error
}

// Router dispatches incoming IPC messages to correct service
type Router struct {
	discovery DiscoveryService
	rtc       RTCService
	pairing   PairingService
}

func NewRouter(discovery DiscoveryService, rtc RTCService, pairing PairingService) *Router {
	return &Router{discovery: discovery, rtc: rtc, pairing: pairing}
}

func (r *Router) ReadLoop() {
	ReadLoop(r.handle)
}

func (r *Router) handle(msg IPCMessage) {
	log.Printf("[ipc] received: %s", msg.Type)

	switch msg.Type {

	case MsgStartDiscovery:
		r.discovery.StartDiscovery()

	case MsgStopDiscovery:
		r.discovery.StopDiscovery()

	case MsgPairRequest:
		var p PairRequestPayload
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			emitError("PAIR_REQUEST_PARSE", err.Error())
			return
		}
		if err := r.pairing.PairWith(p.PeerID, p.PeerAddr, p.PeerPort); err != nil {
			emitError("PAIR_FAILED", err.Error())
		}

	case MsgPairAccept:
		var p struct{ PeerID string `json:"peerId"` }
		if err := json.Unmarshal(msg.Payload, &p); err == nil {
			r.pairing.AcceptPair(p.PeerID)
		}

	case MsgPairReject:
		var p struct{ PeerID string `json:"peerId"` }
		if err := json.Unmarshal(msg.Payload, &p); err == nil {
			r.pairing.RejectPair(p.PeerID)
		}

	case MsgTransferOffer:
		var p TransferOfferPayload
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			emitError("TRANSFER_OFFER_PARSE", err.Error())
			return
		}
		go func() {
			if err := r.rtc.SendFile(p.TransferID, p.PeerID, p.FilePath, p.ECDHPubKey); err != nil {
				emitError("TRANSFER_FAILED", err.Error())
			}
		}()

	case MsgTransferCancel:
		var p struct{ TransferID string `json:"transferId"` }
		if err := json.Unmarshal(msg.Payload, &p); err == nil {
			r.rtc.CancelTransfer(p.TransferID)
		}

	default:
		log.Printf("[ipc] unhandled message type: %s", msg.Type)
	}
}

func emitError(code, msg string) {
	Emit(MsgError, ErrorPayload{Code: code, Message: msg})
}
