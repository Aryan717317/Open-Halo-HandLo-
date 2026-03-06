// peerStore.ts — Discovered peers on the local network
import { writable, derived } from 'svelte/store';
import type { PeerInfo } from '$lib/types';

function createPeerStore() {
  const { subscribe, update } = writable<Map<string, PeerInfo>>(new Map());

  return {
    subscribe,
    add(peer: PeerInfo) {
      update((m) => { m.set(peer.id, peer); return new Map(m); });
    },
    remove(id: string) {
      update((m) => { m.delete(id); return new Map(m); });
    },
    clear() {
      update(() => new Map());
    },
  };
}

export const peerStore = createPeerStore();

export const peers = derived(peerStore, ($m) => Array.from($m.values()));
export const hasPeers = derived(peers, ($p) => $p.length > 0);
