// Prevents additional console window on Windows in release
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod commands;
mod sidecar;

use tauri::Manager;
use std::sync::Mutex;

use std::sync::mpsc::Sender;

pub struct AppState {
    pub sidecar_tx: Mutex<Option<Sender<String>>>,
}

fn main() {
    env_logger::init();

    tauri::Builder::default()
        .manage(AppState {
            sidecar_tx: Mutex::new(None),
        })
        .invoke_handler(tauri::generate_handler![
            commands::file::pick_file,
            commands::file::get_downloads_dir,
            commands::transfer::start_discovery,
            commands::transfer::stop_discovery,
            commands::transfer::pair_request,
            commands::transfer::pair_accept,
            commands::transfer::pair_reject,
            commands::transfer::send_file,
            commands::transfer::cancel_transfer,
            commands::transfer::accept_transfer,
            commands::transfer::reject_transfer,
            commands::device::get_device_info,
        ])
        .setup(|app| {
            let app_handle = app.handle();
            sidecar::spawn(app_handle);
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
