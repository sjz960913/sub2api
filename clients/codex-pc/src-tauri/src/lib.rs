mod codex_adapter;
mod device_registration;
mod panel_auth;
mod redaction;
mod secret_store;
mod protocol;

use codex_adapter::AppServerClient;
use codex_adapter::StartedTask;
use codex_adapter::ThreadPage;
use device_registration::DeviceRegistrar;
use device_registration::DeviceRegistrationError;
use device_registration::RegisteredDevice;
use panel_auth::LoginResult;
use panel_auth::PanelAuthService;
use panel_auth::PanelAuthStatus;
use panel_auth::PublicSession;
use serde::Serialize;
use secret_store::NativeSecretStore;
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

struct PanelAuthState {
    service: Arc<PanelAuthService>,
    registrar: Arc<DeviceRegistrar>,
    registered_device: Mutex<Option<RegisteredDevice>>,
}

impl Default for PanelAuthState {
    fn default() -> Self {
        let store = Arc::new(NativeSecretStore::default());
        Self {
            service: Arc::new(
                PanelAuthService::new(store.clone())
                    .expect("failed to initialize panel HTTP client"),
            ),
            registrar: Arc::new(
                DeviceRegistrar::new(store).expect("failed to initialize device registrar"),
            ),
            registered_device: Mutex::new(None),
        }
    }
}

#[tauri::command]
fn companion_status(state: State<'_, PanelAuthState>) -> CompanionStatus {
    CompanionStatus {
        protocol_version: 1,
        keyring_available: state.service.secure_store_supported(),
        approval_ui: false,
    }
}

#[tauri::command]
fn panel_auth_status(state: State<'_, PanelAuthState>) -> PanelAuthStatus {
    state.service.status()
}

#[tauri::command]
async fn panel_login(
    state: State<'_, PanelAuthState>,
    site_url: String,
    email: String,
    password: String,
    turnstile_token: Option<String>,
) -> Result<LoginResult, String> {
    let service = Arc::clone(&state.service);
    tauri::async_runtime::spawn_blocking(move || {
        service.login(site_url, email, password, turnstile_token)
    })
    .await
    .map_err(|_| "PANEL_TASK_ERROR".to_owned())?
    .map_err(|error| error.public_code().to_owned())
}

#[tauri::command]
async fn panel_complete_two_factor(
    state: State<'_, PanelAuthState>,
    code: String,
) -> Result<PublicSession, String> {
    let service = Arc::clone(&state.service);
    tauri::async_runtime::spawn_blocking(move || service.complete_two_factor(code))
        .await
        .map_err(|_| "PANEL_TASK_ERROR".to_owned())?
        .map_err(|error| error.public_code().to_owned())
}

#[tauri::command]
async fn panel_restore_session(
    state: State<'_, PanelAuthState>,
    site_url: String,
    email: String,
) -> Result<PublicSession, String> {
    let service = Arc::clone(&state.service);
    tauri::async_runtime::spawn_blocking(move || service.restore(site_url, email))
        .await
        .map_err(|_| "PANEL_TASK_ERROR".to_owned())?
        .map_err(|error| error.public_code().to_owned())
}

#[tauri::command]
async fn panel_refresh_session(
    state: State<'_, PanelAuthState>,
) -> Result<PublicSession, String> {
    let service = Arc::clone(&state.service);
    tauri::async_runtime::spawn_blocking(move || service.refresh())
        .await
        .map_err(|_| "PANEL_TASK_ERROR".to_owned())?
        .map_err(|error| error.public_code().to_owned())
}

#[tauri::command]
async fn panel_logout(state: State<'_, PanelAuthState>) -> Result<(), String> {
    let service = Arc::clone(&state.service);
    tauri::async_runtime::spawn_blocking(move || service.logout())
        .await
        .map_err(|_| "PANEL_TASK_ERROR".to_owned())?
        .map_err(|error| error.public_code().to_owned())
}

#[tauri::command]
async fn collaboration_register_device(
    state: State<'_, PanelAuthState>,
) -> Result<RegisteredDevice, String> {
    let auth = Arc::clone(&state.service);
    let registrar = Arc::clone(&state.registrar);
    let result = tauri::async_runtime::spawn_blocking(move || {
        let mut context = auth
            .connection_context()
            .map_err(|error| error.public_code().to_owned())?;
        match registrar.register(&context) {
            Ok(device) => Ok(device),
            Err(DeviceRegistrationError::Unauthorized) => {
                auth.refresh()
                    .map_err(|error| error.public_code().to_owned())?;
                context = auth
                    .connection_context()
                    .map_err(|error| error.public_code().to_owned())?;
                registrar
                    .register(&context)
                    .map_err(|error| error.public_code().to_owned())
            }
            Err(error) => Err(error.public_code().to_owned()),
        }
    })
    .await
    .map_err(|_| "COLLAB_TASK_ERROR".to_owned())??;
    let mut registered = state
        .registered_device
        .lock()
        .map_err(|_| "COLLAB_STATE_ERROR".to_owned())?;
    *registered = Some(result.clone());
    Ok(result)
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
        .manage(PanelAuthState::default())
        .invoke_handler(tauri::generate_handler![
            companion_status,
            panel_auth_status,
            panel_login,
            panel_complete_two_factor,
            panel_restore_session,
            panel_refresh_session,
            panel_logout,
            collaboration_register_device,
            codex_start,
            codex_stop,
            codex_list_threads,
            codex_start_task,
            codex_interrupt
        ])
        .run(tauri::generate_context!())
        .expect("failed to run Codex PC companion");
}
