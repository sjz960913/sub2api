mod codex_adapter;
mod redaction;
mod secret_store;
mod protocol;

use codex_adapter::AppServerClient;
use codex_adapter::StartedTask;
use codex_adapter::ThreadPage;
use serde::Serialize;
use secret_store::NativeSecretStore;
use secret_store::SecretStore;
use std::sync::Arc;
use std::sync::Mutex;
use tauri::State;

#[derive(Serialize)]
struct CompanionStatus {
    protocol_version: u8,
    keyring_available: bool,
    approval_ui: bool,
}

#[derive(Default)]
struct CodexAdapterState {
    client: Mutex<Option<AppServerClient>>,
}

#[derive(Serialize)]
struct CodexAdapterStatus {
    running: bool,
}

struct SecretStoreState {
    store: Arc<dyn SecretStore>,
}

impl Default for SecretStoreState {
    fn default() -> Self {
        Self {
            store: Arc::new(NativeSecretStore::default()),
        }
    }
}

#[tauri::command]
fn companion_status(state: State<'_, SecretStoreState>) -> CompanionStatus {
    CompanionStatus {
        protocol_version: 1,
        keyring_available: state.store.is_supported(),
        approval_ui: false,
    }
}

#[tauri::command]
fn codex_start(state: State<'_, CodexAdapterState>) -> Result<CodexAdapterStatus, String> {
    let mut client = state.client.lock().map_err(|_| "CODEX_STATE_ERROR")?;
    if client.is_none() {
        *client = Some(AppServerClient::start_default().map_err(|error| error.public_code())?);
    }
    Ok(CodexAdapterStatus { running: true })
}

#[tauri::command]
fn codex_stop(state: State<'_, CodexAdapterState>) -> Result<CodexAdapterStatus, String> {
    let mut client = state.client.lock().map_err(|_| "CODEX_STATE_ERROR")?;
    client.take();
    Ok(CodexAdapterStatus { running: false })
}

#[tauri::command]
fn codex_list_threads(
    state: State<'_, CodexAdapterState>,
    limit: u32,
    cursor: Option<String>,
    search_term: Option<String>,
    archived: bool,
) -> Result<ThreadPage, String> {
    let client = state.client.lock().map_err(|_| "CODEX_STATE_ERROR")?;
    let client = client.as_ref().ok_or("CODEX_NOT_RUNNING")?;
    client
        .list_threads(limit, cursor.as_deref(), search_term.as_deref(), archived)
        .map_err(|error| error.public_code().to_owned())
}

#[tauri::command]
fn codex_start_task(
    state: State<'_, CodexAdapterState>,
    thread_id: String,
    prompt: String,
) -> Result<StartedTask, String> {
    let client = state.client.lock().map_err(|_| "CODEX_STATE_ERROR")?;
    let client = client.as_ref().ok_or("CODEX_NOT_RUNNING")?;
    client
        .start_task(&thread_id, &prompt)
        .map_err(|error| error.public_code().to_owned())
}

#[tauri::command]
fn codex_interrupt(
    state: State<'_, CodexAdapterState>,
    thread_id: String,
    turn_id: String,
) -> Result<(), String> {
    let client = state.client.lock().map_err(|_| "CODEX_STATE_ERROR")?;
    let client = client.as_ref().ok_or("CODEX_NOT_RUNNING")?;
    client
        .interrupt(&thread_id, &turn_id)
        .map_err(|error| error.public_code().to_owned())
}

pub fn run() {
    tauri::Builder::default()
        .manage(CodexAdapterState::default())
        .manage(SecretStoreState::default())
        .invoke_handler(tauri::generate_handler![
            companion_status,
            codex_start,
            codex_stop,
            codex_list_threads,
            codex_start_task,
            codex_interrupt
        ])
        .run(tauri::generate_context!())
        .expect("failed to run Codex PC companion");
}
