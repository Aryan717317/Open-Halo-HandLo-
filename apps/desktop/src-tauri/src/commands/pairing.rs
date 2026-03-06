// commands/pairing.rs
use tauri::AppHandle;
use crate::sidecar::{send_to_sidecar, IPCMessage};

#[tauri::command]
pub async fn pair_with_peer(
    app: AppHandle,
    peer_id: String,
    peer_addr: String,
    peer_port: u16,
) -> Result<(), String> {
    let msg = IPCMessage {
        msg_type: "PAIR_REQUEST".to_string(),
        id: None,
        payload: Some(serde_json::json!({
            "peerId": peer_id,
            "peerAddr": peer_addr,
            "peerPort": peer_port,
        })),
    };
    send_to_sidecar(&app, &msg).map_err(|e| e.to_string())
}

#[tauri::command]
pub async fn accept_pair(app: AppHandle, peer_id: String) -> Result<(), String> {
    let msg = IPCMessage {
        msg_type: "PAIR_ACCEPT".to_string(),
        id: None,
        payload: Some(serde_json::json!({ "peerId": peer_id })),
    };
    send_to_sidecar(&app, &msg).map_err(|e| e.to_string())
}

#[tauri::command]
pub async fn reject_pair(app: AppHandle, peer_id: String) -> Result<(), String> {
    let msg = IPCMessage {
        msg_type: "PAIR_REJECT".to_string(),
        id: None,
        payload: Some(serde_json::json!({ "peerId": peer_id })),
    };
    send_to_sidecar(&app, &msg).map_err(|e| e.to_string())
}

// commands/discovery.rs  (inlined here for brevity)
use tauri::AppHandle as AH;
use crate::sidecar::{send_to_sidecar as sts, IPCMessage as IPCM};

#[tauri::command]
pub async fn start_discovery(app: AH) -> Result<(), String> {
    sts(&app, &IPCM {
        msg_type: "START_DISCOVERY".to_string(),
        id: None,
        payload: None,
    }).map_err(|e| e.to_string())
}

#[tauri::command]
pub async fn stop_discovery(app: AH) -> Result<(), String> {
    sts(&app, &IPCM {
        msg_type: "STOP_DISCOVERY".to_string(),
        id: None,
        payload: None,
    }).map_err(|e| e.to_string())
}
