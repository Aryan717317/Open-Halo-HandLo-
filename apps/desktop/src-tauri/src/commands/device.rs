// commands/device.rs — Device metadata
use serde::Serialize;

#[derive(Serialize)]
pub struct DeviceInfo {
    pub name: String,
    pub os: String,
    pub version: String,
}

#[tauri::command]
pub fn get_device_info() -> DeviceInfo {
    DeviceInfo {
        name: hostname::get()
            .map(|h| h.to_string_lossy().to_string())
            .unwrap_or_else(|_| "Unknown".to_string()),
        os: std::env::consts::OS.to_string(),
        version: env!("CARGO_PKG_VERSION").to_string(),
    }
}
