// types.ts — Shared type definitions across the app

export interface PeerInfo {
  id: string;
  name: string;
  address: string;
  port: number;
  os: string;
}

export interface ProgressEvent {
  transfer_id: string;
  bytes_sent: number;
  total_bytes: number;
  percent: number;
  speed_bps: number;
}

export interface TransferOffer {
  transfer_id: string;
  peer_id: string;
  peer_name: string;
  file_name: string;
  file_size: number;
  mime_type: string;
}

export type ConnectionMethod = 'qr' | 'code' | 'lan';

export type AppScreen =
  | 'connect'     // choose connection method
  | 'pairing'     // mid-pairing
  | 'ready'       // paired, gesture camera active
  | 'selecting'   // grab detected, file picker open
  | 'sending'     // transfer in progress (sender)
  | 'receiving';  // transfer in progress (receiver)
