// sidecar/mod.rs
// Spawns the Go networking binary as a child process.
// Tauri <-> Go communication: newline-delimited JSON over stdin/stdout.

use std::io::{BufRead, BufReader, Write};
use std::process::{Command, Stdio};
use std::thread;
use tauri::{AppHandle, Manager};
use serde_json::Value;

pub fn spawn(app: AppHandle) {
    let app_clone = app.clone();

    // Use a standard library MPSC channel since we're spawning standard threads
    let (tx, rx) = std::sync::mpsc::channel::<String>();
    
    // Store the sender in AppState
    if let Some(state) = app.try_state::<crate::AppState>() {
        *state.sidecar_tx.lock().unwrap() = Some(tx);
    }

    thread::spawn(move || {
        // In dev: run `go run .` from sidecar dir
        // In prod: Tauri bundles the compiled sidecar binary
        let mut child = Command::new("go")
            .args(["run", "."])
            .current_dir("../../../../sidecar")
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::inherit()) // Go logs → terminal
            .spawn()
            .expect("Failed to spawn Go sidecar. Is Go installed?");

        let stdout = child.stdout.take().unwrap();
        let mut stdin = child.stdin.take().unwrap();

        // Spawn a separate thread to handle writing to stdin
        thread::spawn(move || {
            for msg in rx {
                if let Err(e) = writeln!(stdin, "{}", msg) {
                    log::error!("[sidecar writer error] {}", e);
                    break;
                }
            }
        });

        let reader = BufReader::new(stdout);

        // Read events from Go sidecar → forward to Svelte frontend
        for line in reader.lines() {
            match line {
                Ok(json_str) => {
                    if let Ok(msg) = serde_json::from_str::<Value>(&json_str) {
                        let event_type = msg["type"].as_str().unwrap_or("unknown");
                        // Forward to SvelteKit via Tauri event system
                        app_clone.emit_all(event_type, &msg["payload"]).ok();
                        log::debug!("[sidecar→tauri] {}", event_type);
                    }
                }
                Err(e) => log::error!("[sidecar] read error: {}", e),
            }
        }
    });
}

/// Send a command from Tauri to the Go sidecar via stdin
pub fn send_command(app: &AppHandle, cmd_type: &str, payload: Value) {
    let msg = serde_json::json!({
        "type": cmd_type,
        "payload": payload
    });
    
    let msg_str = msg.to_string();
    log::debug!("[tauri→sidecar] {}", cmd_type);
    
    if let Some(state) = app.try_state::<crate::AppState>() {
        let tx_guard = state.sidecar_tx.lock().unwrap();
        if let Some(tx) = tx_guard.as_ref() {
            let _ = tx.send(msg_str);
        } else {
            log::error!("Cannot send command to sidecar: stdin channel not initialized");
        }
    }
}
