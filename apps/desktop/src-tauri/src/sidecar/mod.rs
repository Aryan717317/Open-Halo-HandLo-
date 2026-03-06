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
    // In production: use the stored stdin handle
    // For scaffold: commands are sent via stored child process stdin
    log::debug!("[tauri→sidecar] {}", cmd_type);
    let _ = msg; // TODO: wire up stdin writer in Phase 1
}
