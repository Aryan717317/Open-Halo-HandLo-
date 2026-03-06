// tauribridge.ts — All Tauri invoke() calls in one place
import { invoke } from '@tauri-apps/api/tauri';
import { listen } from '@tauri-apps/api/event';
import { peerStore } from '$lib/stores/peerStore';
import { transferStore } from '$lib/stores/transferStore';
import type { PeerInfo, ProgressEvent, TransferOffer } from '$lib/types';

// ── Discovery ─────────────────────────────────────────────────────────────────

export async function startDiscovery(): Promise<void> {
  await listen<PeerInfo>('EVT_PEER_FOUND', (e) => {
    peerStore.add(e.payload);
  });

  await listen<{ id: string }>('EVT_PEER_LOST', (e) => {
    peerStore.remove(e.payload.id);
  });

  await invoke('start_discovery');
}

export async function stopDiscovery(): Promise<void> {
  await invoke('stop_discovery');
}

// ── Pairing ───────────────────────────────────────────────────────────────────

export async function pairWithPeer(peerId: string, peerName: string): Promise<void> {
  await invoke('pair_request', {
    args: { peer_id: peerId, peer_name: peerName, public_key: "", tls_fingerprint: "", code: null }
  });
}

export async function pairWithCode(code: string): Promise<void> {
  await invoke('pair_request', {
    args: { peer_id: "", peer_name: "", public_key: "", tls_fingerprint: "", code }
  });
}

export async function acceptPair(peerId: string): Promise<void> {
  await invoke('pair_accept', { args: { peer_id: peerId } });
}

export async function rejectPair(peerId: string): Promise<void> {
  await invoke('pair_reject', { args: { peer_id: peerId } });
}

export async function onIncomingPair(handler: (req: any) => void): Promise<void> {
  await listen<any>('EVT_PAIR_INCOMING', (e) => handler(e.payload));
}

export async function onPairSuccess(handler: (res: any) => void): Promise<void> {
  await listen<any>('EVT_PAIR_SUCCESS', (e) => handler(e.payload));
}

export async function onPairRejected(handler: (res: any) => void): Promise<void> {
  await listen<any>('EVT_PAIR_REJECTED', (e) => handler(e.payload));
}

// ── File transfer ─────────────────────────────────────────────────────────────

export async function pickFile(): Promise<string | null> {
  return await invoke<string | null>('pick_file');
}

export async function sendFile(
  filePath: string,
  peerId: string,
  fileName: string,
  fileSize: number
): Promise<void> {
  const transferId = crypto.randomUUID();

  await listen<ProgressEvent>('EVT_TX_PROGRESS', (e) => {
    if (e.payload.transfer_id === transferId) {
      transferStore.updateProgress(e.payload);
    }
  });

  await listen<{ transfer_id: string }>('EVT_TX_COMPLETE', (e) => {
    if (e.payload.transfer_id === transferId) {
      transferStore.complete(transferId);
    }
  });

  await invoke('send_file', {
    args: { transferId, peerId, filePath, fileName, fileSize, mimeType: 'application/octet-stream' },
  });
}

export async function cancelTransfer(transferId: string): Promise<void> {
  await invoke('cancel_transfer', { transferId });
}

export async function acceptTransfer(offer: TransferOffer, savePath: string): Promise<void> {
  await invoke('accept_transfer', {
    transferId: offer.transfer_id,
    senderAddress: offer.sender_address,
    tcpPort: offer.tcp_port,
    savePath: savePath,
    fileSize: offer.file_size
  });
  transferStore.startReceiving(offer);
}

export async function rejectTransfer(transferId: string): Promise<void> {
  await invoke('reject_transfer', { transferId });
}

// ── Incoming transfer listener ────────────────────────────────────────────────

export async function onIncomingTransfer(
  handler: (offer: TransferOffer) => void
): Promise<void> {
  await listen<TransferOffer>('EVT_TX_OFFER', (e) => handler(e.payload));
}

// ── Device Info ──────────────────────────────────────────────────────────────

export async function requestDeviceInfo(handler: (info: any) => void): Promise<void> {
  await listen<any>('EVT_DEVICE_INFO', (e) => handler(e.payload));
  await invoke('get_device_info');
}
