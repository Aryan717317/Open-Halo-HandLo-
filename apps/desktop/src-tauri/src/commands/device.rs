// commands/device.rs — Device metadata
use serde_json::json;
use tauri::AppHandle;

#[tauri::command]
pub fn get_device_info(app: AppHandle) {
    crate::sidecar::send_command(&app, "CMD_GET_DEVICE_INFO", json!({}));
}
