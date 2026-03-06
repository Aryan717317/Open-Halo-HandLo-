// transferStore.ts — Active file transfer state
import { writable } from 'svelte/store';
import type { ProgressEvent } from '$lib/types';

export type TransferStatus = 'pending' | 'active' | 'complete' | 'error' | 'cancelled';

export interface Transfer {
  id: string;
  fileName: string;
  fileSize: number;
  peerId: string;
  peerName: string;
  direction: 'send' | 'receive';
  status: TransferStatus;
  progress: number;  // 0–100
  speedBps: number;
  startedAt: number;
}

function createTransferStore() {
  const { subscribe, update } = writable<Map<string, Transfer>>(new Map());

  return {
    subscribe,
    add(transfer: Transfer) {
      update((m) => { m.set(transfer.id, transfer); return new Map(m); });
    },
    updateProgress(evt: ProgressEvent) {
      update((m) => {
        const t = m.get(evt.transfer_id);
        if (t) {
          m.set(evt.transfer_id, {
            ...t,
            status: 'active',
            progress: evt.percent,
            speedBps: evt.speed_bps,
          });
        }
        return new Map(m);
      });
    },
    complete(id: string) {
      update((m) => {
        const t = m.get(id);
        if (t) m.set(id, { ...t, status: 'complete', progress: 100 });
        return new Map(m);
      });
    },
    error(id: string) {
      update((m) => {
        const t = m.get(id);
        if (t) m.set(id, { ...t, status: 'error' });
        return new Map(m);
      });
    },
  };
}

export const transferStore = createTransferStore();
