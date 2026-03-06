// commands/file.rs — File system operations exposed to SvelteKit
use tauri::api::dialog::FileDialogBuilder;
use tauri::api::path::download_dir;

#[tauri::command]
pub async fn pick_file() -> Result<Option<String>, String> {
    // Opens native file picker — returns selected file path
    let (tx, rx) = tokio::sync::oneshot::channel();

    FileDialogBuilder::new()
        .set_title("Select file to share")
        .pick_file(move |path| {
            let result = path.map(|p| p.to_string_lossy().to_string());
            tx.send(result).ok();
        });

    rx.await.map_err(|e| e.to_string())
}

#[tauri::command]
pub fn get_downloads_dir() -> Result<String, String> {
    download_dir()
        .map(|p| p.to_string_lossy().to_string())
        .ok_or_else(|| "Cannot find downloads directory".to_string())
}
