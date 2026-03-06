// commands/transfer.rs — Transfer control commands, bridged to Go sidecar
use crate::sidecar;
use serde::{Deserialize, Serialize};
use tauri::AppHandle;

#[derive(Deserialize)]
pub struct PairRequestArgs {
    peer_id: String,
    peer_name: String,
    public_key: String,
    tls_fingerprint: String,
    code: Option<String>,
}

#[derive(Deserialize)]
pub struct PairAcceptArgs {
    peer_id: String,
}

#[derive(Deserialize)]
pub struct PairRejectArgs {
    peer_id: String,
}

#[derive(Deserialize)]
pub struct SendFileArgs {
    transfer_id: String,
    peer_id: String,
    file_path: String,
    file_name: String,
    file_size: i64,
    mime_type: String,
}

#[tauri::command]
pub async fn start_discovery(app: AppHandle) -> Result<(), String> {
    sidecar::send_command(&app, "CMD_DISCOVER", serde_json::json!({}));
    Ok(())
}

#[tauri::command]
pub async fn stop_discovery(app: AppHandle) -> Result<(), String> {
    sidecar::send_command(&app, "CMD_STOP_DISCOVER", serde_json::json!({}));
    Ok(())
}

#[tauri::command]
pub async fn pair_request(app: AppHandle, args: PairRequestArgs) -> Result<(), String> {
    sidecar::send_command(&app, "CMD_PAIR_REQUEST", serde_json::json!({
        "peer_id": args.peer_id,
        "peer_name": args.peer_name,
        "public_key": args.public_key,
        "tls_fingerprint": args.tls_fingerprint,
        "code": args.code,
    }));
    Ok(())
}

#[tauri::command]
pub async fn pair_accept(app: AppHandle, args: PairAcceptArgs) -> Result<(), String> {
    sidecar::send_command(&app, "CMD_PAIR_ACCEPT", serde_json::json!({
        "peer_id": args.peer_id,
    }));
    Ok(())
}

#[tauri::command]
pub async fn pair_reject(app: AppHandle, args: PairRejectArgs) -> Result<(), String> {
    sidecar::send_command(&app, "CMD_PAIR_REJECT", serde_json::json!({
        "peer_id": args.peer_id,
    }));
    Ok(())
}

#[tauri::command]
pub async fn send_file(app: AppHandle, args: SendFileArgs) -> Result<(), String> {
    sidecar::send_command(&app, "CMD_SEND_FILE", serde_json::json!({
        "transfer_id": args.transfer_id,
        "peer_id": args.peer_id,
        "file_path": args.file_path,
        "file_name": args.file_name,
        "file_size": args.file_size,
        "mime_type": args.mime_type,
    }));
    Ok(())
}

#[tauri::command]
pub async fn cancel_transfer(app: AppHandle, transfer_id: String) -> Result<(), String> {
    sidecar::send_command(&app, "CMD_CANCEL_TX", serde_json::json!({
        "transfer_id": transfer_id
    }));
    Ok(())
}

#[tauri::command]
pub async fn accept_transfer(app: AppHandle, transfer_id: String) -> Result<(), String> {
    sidecar::send_command(&app, "CMD_TX_ACCEPT", serde_json::json!({
        "transfer_id": transfer_id
    }));
    Ok(())
}

#[tauri::command]
pub async fn reject_transfer(app: AppHandle, transfer_id: String) -> Result<(), String> {
    sidecar::send_command(&app, "CMD_TX_REJECT", serde_json::json!({
        "transfer_id": transfer_id
    }));
    Ok(())
}
