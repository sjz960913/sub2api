mod redaction;
mod secret_store;
mod protocol;

use serde::Serialize;

#[derive(Serialize)]
struct CompanionStatus {
    protocol_version: u8,
    keyring_available: bool,
    approval_ui: bool,
}

#[tauri::command]
fn companion_status() -> CompanionStatus {
    CompanionStatus {
        protocol_version: 1,
        keyring_available: false,
        approval_ui: false,
    }
}

pub fn run() {
    tauri::Builder::default()
        .invoke_handler(tauri::generate_handler![companion_status])
        .run(tauri::generate_context!())
        .expect("failed to run Codex PC companion");
}
