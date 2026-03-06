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

export async function pairWithCode(code: string): Promise<void> {
  await invoke('pair_request', { args: { code } });
}

export async function acceptPair(peerId: string): Promise<void> {
  await invoke('pair_accept', { args: { peerId } });
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

// ── Incoming transfer listener ────────────────────────────────────────────────

export async function onIncomingTransfer(
  handler: (offer: TransferOffer) => void
): Promise<void> {
  await listen<TransferOffer>('EVT_TX_OFFER', (e) => handler(e.payload));
}
