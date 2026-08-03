use crate::codex_adapter::{
    AppServerClient, NormalizedCompletedItem, NormalizedHistoricalItem, ThreadPage,
};
use crate::device_registration::RegisteredDevice;
use crate::panel_auth::{PanelAuthService, PanelConnectionContext};
use crate::protocol::collaboration_wire::{CodexSession, EventEnvelope, ThreadSummary};
use chrono::{SecondsFormat, TimeZone, Utc};
use futures_util::{SinkExt, StreamExt};
use serde::Serialize;
use serde_json::{json, Map, Value};
use std::collections::HashSet;
use std::sync::atomic::{AtomicBool, AtomicI64, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Duration;
use tokio::sync::mpsc;
use tokio::task::JoinHandle;
use tokio_tungstenite::connect_async_with_config;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;
use tokio_tungstenite::tungstenite::http::HeaderValue;
use tokio_tungstenite::tungstenite::protocol::{Message, WebSocketConfig};
use url::Url;
use uuid::Uuid;
use zeroize::Zeroizing;

const MAX_WS_MESSAGE_BYTES: usize = 1024 * 1024;
const COMMAND_TIMEOUT: Duration = Duration::from_secs(6 * 60 * 60);

pub type SharedCodexClient = Arc<Mutex<Option<Arc<AppServerClient>>>>;

#[derive(Clone, Debug, Serialize, PartialEq, Eq)]
pub struct RelayStatus {
    pub state: String,
    pub device_id: Option<String>,
    pub last_error: Option<String>,
}

pub struct RelayController {
    status: Arc<Mutex<RelayStatus>>,
    task: Mutex<Option<JoinHandle<()>>>,
}

impl Default for RelayController {
    fn default() -> Self {
        Self {
            status: Arc::new(Mutex::new(RelayStatus {
                state: "disconnected".to_owned(),
                device_id: None,
                last_error: None,
            })),
            task: Mutex::new(None),
        }
    }
}

impl RelayController {
    pub fn status(&self) -> RelayStatus {
        self.status
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
    }

    pub fn start(
        &self,
        auth: Arc<PanelAuthService>,
        device: RegisteredDevice,
        codex: SharedCodexClient,
    ) -> Result<RelayStatus, &'static str> {
        let mut task = self.task.lock().map_err(|_| "COLLAB_STATE_ERROR")?;
        if let Some(previous) = task.take() {
            previous.abort();
        }
        update_status(&self.status, "connecting", Some(&device.device_id), None);
        let status = Arc::clone(&self.status);
        *task = Some(tokio::spawn(async move {
            relay_reconnect_loop(auth, device, codex, status).await;
        }));
        Ok(self.status())
    }

    pub fn disconnect(&self) -> Result<RelayStatus, &'static str> {
        if let Some(task) = self.task.lock().map_err(|_| "COLLAB_STATE_ERROR")?.take() {
            task.abort();
        }
        update_status(&self.status, "disconnected", None, None);
        Ok(self.status())
    }
}

#[derive(Clone)]
struct OutboundEvent {
    event_type: &'static str,
    request_id: Option<String>,
    payload: Map<String, Value>,
}

enum ConnectionExit {
    Retry(&'static str),
    TokenExpired,
    DeviceRevoked,
}

async fn relay_reconnect_loop(
    auth: Arc<PanelAuthService>,
    device: RegisteredDevice,
    codex: SharedCodexClient,
    status: Arc<Mutex<RelayStatus>>,
) {
    let mut retry_seconds = 1_u64;
    loop {
        let auth_for_context = Arc::clone(&auth);
        let context = match tokio::task::spawn_blocking(move || {
            auth_for_context.connection_context()
        })
        .await
        {
            Ok(Ok(context)) => context,
            _ => {
                update_status(
                    &status,
                    "disconnected",
                    Some(&device.device_id),
                    Some("PANEL_SESSION_NOT_FOUND"),
                );
                return;
            }
        };
        match run_connection(&context, &device, Arc::clone(&codex), Arc::clone(&status)).await {
            ConnectionExit::DeviceRevoked => {
                update_status(
                    &status,
                    "revoked",
                    Some(&device.device_id),
                    Some("COLLAB_DEVICE_REVOKED"),
                );
                return;
            }
            ConnectionExit::TokenExpired => {
                update_status(&status, "refreshing", Some(&device.device_id), None);
                let auth_for_refresh = Arc::clone(&auth);
                if !matches!(
                    tokio::task::spawn_blocking(move || auth_for_refresh.refresh()).await,
                    Ok(Ok(_))
                ) {
                    update_status(
                        &status,
                        "disconnected",
                        Some(&device.device_id),
                        Some("PANEL_REFRESH_FAILED"),
                    );
                    return;
                }
                retry_seconds = 1;
                continue;
            }
            ConnectionExit::Retry(error) => {
                update_status(
                    &status,
                    "reconnecting",
                    Some(&device.device_id),
                    Some(error),
                );
                tokio::time::sleep(Duration::from_secs(retry_seconds)).await;
                retry_seconds = (retry_seconds * 2).min(30);
            }
        }
    }
}

async fn run_connection(
    context: &PanelConnectionContext,
    device: &RegisteredDevice,
    codex: SharedCodexClient,
    status: Arc<Mutex<RelayStatus>>,
) -> ConnectionExit {
    let request = match websocket_request(context, &device.device_id) {
        Ok(request) => request,
        Err(error) => return ConnectionExit::Retry(error),
    };
    let mut config = WebSocketConfig::default();
    config.max_message_size = Some(MAX_WS_MESSAGE_BYTES);
    config.max_frame_size = Some(MAX_WS_MESSAGE_BYTES);
    let (mut socket, _) = match connect_async_with_config(request, Some(config), false).await {
        Ok(result) => result,
        Err(_) => return ConnectionExit::Retry("COLLAB_CONNECT_FAILED"),
    };
    update_status(&status, "connected", Some(&device.device_id), None);

    let sequence = Arc::new(AtomicI64::new(1));
    let hello = OutboundEvent {
        event_type: "device.hello",
        request_id: None,
        payload: map_from(json!({
            "companion_version": env!("CARGO_PKG_VERSION"),
            "protocol_version": 1,
            "capabilities": {
                "app_server": true,
                "thread_read": true,
                "thread_write": true,
                "image_input": false
            }
        })),
    };
    if send_event(&mut socket, &sequence, hello).await.is_err() {
        return ConnectionExit::Retry("COLLAB_WRITE_FAILED");
    }

    let (outgoing_tx, mut outgoing_rx) = mpsc::unbounded_channel::<OutboundEvent>();
    let active_command = Arc::new(AtomicBool::new(false));
    let active_turn = Arc::new(Mutex::new(None::<(String, String, String)>));
    let cancelled = Arc::new(Mutex::new(HashSet::<String>::new()));
    let heartbeat_seconds = u64::try_from(device.heartbeat_interval_seconds)
        .unwrap_or(20)
        .clamp(5, 60);
    let mut heartbeat = tokio::time::interval(Duration::from_secs(heartbeat_seconds));
    heartbeat.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Delay);

    loop {
        tokio::select! {
            _ = heartbeat.tick() => {
                let event = OutboundEvent {
                    event_type: "heartbeat",
                    request_id: Some(Uuid::new_v4().to_string()),
                    payload: map_from(json!({
                        "device_id": device.device_id,
                        "app_server_status": if codex_is_ready(&codex) { "ready" } else { "unavailable" },
                        "active_thread_count": if active_command.load(Ordering::Acquire) { 1 } else { 0 }
                    })),
                };
                if send_event(&mut socket, &sequence, event).await.is_err() {
                    return ConnectionExit::Retry("COLLAB_WRITE_FAILED");
                }
            }
            Some(event) = outgoing_rx.recv() => {
                if send_event(&mut socket, &sequence, event).await.is_err() {
                    return ConnectionExit::Retry("COLLAB_WRITE_FAILED");
                }
            }
            message = socket.next() => {
                let Some(message) = message else {
                    return ConnectionExit::Retry("COLLAB_DISCONNECTED");
                };
                match message {
                    Ok(Message::Text(text)) => {
                        let Ok(event) = serde_json::from_str::<EventEnvelope>(text.as_str()) else {
                            return ConnectionExit::Retry("COLLAB_PROTOCOL_ERROR");
                        };
                        if let Some(exit) = handle_server_event(
                            event,
                            Arc::clone(&codex),
                            outgoing_tx.clone(),
                            Arc::clone(&active_command),
                            Arc::clone(&active_turn),
                            Arc::clone(&cancelled),
                        ) {
                            return exit;
                        }
                    }
                    Ok(Message::Ping(payload)) => {
                        if socket.send(Message::Pong(payload)).await.is_err() {
                            return ConnectionExit::Retry("COLLAB_WRITE_FAILED");
                        }
                    }
                    Ok(Message::Close(frame)) => {
                        let code = frame.map(|frame| u16::from(frame.code)).unwrap_or_default();
                        return match code {
                            4401 => ConnectionExit::TokenExpired,
                            4403 => ConnectionExit::DeviceRevoked,
                            _ => ConnectionExit::Retry("COLLAB_DISCONNECTED"),
                        };
                    }
                    Ok(Message::Binary(_)) => return ConnectionExit::Retry("COLLAB_PROTOCOL_ERROR"),
                    Ok(_) => {}
                    Err(_) => return ConnectionExit::Retry("COLLAB_READ_FAILED"),
                }
            }
        }
    }
}

fn handle_server_event(
    event: EventEnvelope,
    codex: SharedCodexClient,
    outgoing: mpsc::UnboundedSender<OutboundEvent>,
    active_command: Arc<AtomicBool>,
    active_turn: Arc<Mutex<Option<(String, String, String)>>>,
    cancelled: Arc<Mutex<HashSet<String>>>,
) -> Option<ConnectionExit> {
    if event.v != 1 || event.event_id.len() < 8 {
        return Some(ConnectionExit::Retry("COLLAB_PROTOCOL_ERROR"));
    }
    match event.r#type.as_str() {
        "heartbeat.ack" | "device.presence_changed" | "announcement.invalidated" => {}
        "server.shutdown" => {
            if event.payload.get("reason").and_then(Value::as_str) == Some("device_revoked") {
                return Some(ConnectionExit::DeviceRevoked);
            }
            return Some(ConnectionExit::Retry("COLLAB_SERVER_RESTART"));
        }
        "session_sync.requested" => {
            spawn_session_sync(event, codex, outgoing);
        }
        "thread_sync.requested" => {
            spawn_thread_sync(event, codex, outgoing);
        }
        "command.dispatched" => {
            spawn_command(
                event,
                codex,
                outgoing,
                active_command,
                active_turn,
                cancelled,
            );
        }
        "command.cancel_requested" => {
            request_cancel(event, codex, active_turn, cancelled);
        }
        _ => {}
    }
    None
}

fn spawn_thread_sync(
    event: EventEnvelope,
    codex: SharedCodexClient,
    outgoing: mpsc::UnboundedSender<OutboundEvent>,
) {
    let Some(sync_id) = event
        .payload
        .get("sync_id")
        .and_then(Value::as_str)
        .map(str::to_owned)
    else {
        return;
    };
    let Some(thread_id) = event
        .payload
        .get("thread_id")
        .and_then(Value::as_str)
        .map(str::to_owned)
    else {
        return;
    };
    let request_id = event.request_id;
    let after_item_id = optional_string(&event.payload, "after_item_id", 512);
    let limit = event
        .payload
        .get("limit")
        .and_then(Value::as_u64)
        .and_then(|value| usize::try_from(value).ok())
        .unwrap_or(100)
        .clamp(1, 200);
    tokio::task::spawn_blocking(move || {
        let result = with_codex(&codex, |client| {
            client.read_thread_snapshot(&thread_id, after_item_id.as_deref(), limit)
        });
        match result {
            Ok(snapshot) => {
                let thread = ThreadSummary {
                    thread_id: snapshot.thread_id,
                    title: snapshot.title,
                    status: snapshot.status,
                    write_state: snapshot.write_state,
                };
                let items = snapshot
                    .items
                    .iter()
                    .enumerate()
                    .map(|(index, item)| historical_relay_item(item, index as i64))
                    .collect::<Vec<_>>();
                queue_event(
                    &outgoing,
                    "thread_sync.completed",
                    request_id,
                    json!({
                        "sync_id": sync_id,
                        "snapshot_version": Utc::now().timestamp_millis(),
                        "thread": thread,
                        "items": items,
                        "next_cursor": null
                    }),
                );
            }
            Err(error_code) => queue_event(
                &outgoing,
                "thread_sync.failed",
                request_id,
                json!({"sync_id": sync_id, "error_code": error_code.to_lowercase()}),
            ),
        }
    });
}

fn spawn_session_sync(
    event: EventEnvelope,
    codex: SharedCodexClient,
    outgoing: mpsc::UnboundedSender<OutboundEvent>,
) {
    let Some(sync_id) = event
        .payload
        .get("sync_id")
        .and_then(Value::as_str)
        .map(str::to_owned)
    else {
        return;
    };
    let request_id = event.request_id;
    let limit = event
        .payload
        .get("limit")
        .and_then(Value::as_u64)
        .and_then(|value| u32::try_from(value).ok())
        .unwrap_or(50)
        .clamp(1, 100);
    let cursor = optional_string(&event.payload, "cursor", 1024);
    let search_term = optional_string(&event.payload, "search_term", 200);
    let archived = event
        .payload
        .get("archived")
        .and_then(Value::as_bool)
        .unwrap_or(false);
    tokio::task::spawn_blocking(move || {
        let result = with_codex(&codex, |client| {
            client.list_threads(limit, cursor.as_deref(), search_term.as_deref(), archived)
        });
        match result {
            Ok(page) => {
                let (items, next_cursor) = session_items(page);
                queue_event(
                    &outgoing,
                    "session_sync.completed",
                    request_id,
                    json!({
                        "sync_id": sync_id,
                        "snapshot_version": Utc::now().timestamp_millis(),
                        "items": items,
                        "next_cursor": next_cursor
                    }),
                );
            }
            Err(error_code) => queue_event(
                &outgoing,
                "session_sync.failed",
                request_id,
                json!({"sync_id": sync_id, "error_code": error_code.to_lowercase()}),
            ),
        }
    });
}

fn spawn_command(
    event: EventEnvelope,
    codex: SharedCodexClient,
    outgoing: mpsc::UnboundedSender<OutboundEvent>,
    active_command: Arc<AtomicBool>,
    active_turn: Arc<Mutex<Option<(String, String, String)>>>,
    cancelled: Arc<Mutex<HashSet<String>>>,
) {
    let Some(command_id) = event
        .payload
        .get("command_id")
        .and_then(Value::as_str)
        .map(str::to_owned)
    else {
        return;
    };
    let Some(thread_id) = event
        .payload
        .get("thread_id")
        .and_then(Value::as_str)
        .map(str::to_owned)
    else {
        return;
    };
    let Some(prompt) = event
        .payload
        .get("input")
        .and_then(Value::as_array)
        .and_then(|input| input.first())
        .filter(|input| input.get("type").and_then(Value::as_str) == Some("text"))
        .and_then(|input| input.get("text"))
        .and_then(Value::as_str)
        .map(str::to_owned)
    else {
        return;
    };
    let request_id = event.request_id.or_else(|| Some(command_id.clone()));
    queue_event(
        &outgoing,
        "command.received",
        request_id.clone(),
        json!({"command_id": command_id}),
    );
    if active_command.swap(true, Ordering::AcqRel) {
        queue_event(
            &outgoing,
            "command.failed",
            request_id,
            json!({"command_id": command_id, "error_code": "companion_busy"}),
        );
        return;
    }

    tokio::task::spawn_blocking(move || {
        if is_cancelled(&cancelled, &command_id) {
            queue_event(
                &outgoing,
                "command.failed",
                request_id,
                json!({"command_id": command_id, "error_code": "cancelled_before_start"}),
            );
            if let Ok(mut cancelled_commands) = cancelled.lock() {
                cancelled_commands.remove(&command_id);
            }
            active_command.store(false, Ordering::Release);
            return;
        }
        let client = match current_codex(&codex) {
            Ok(client) => client,
            Err(error_code) => {
                queue_event(
                    &outgoing,
                    "command.failed",
                    request_id,
                    json!({"command_id": command_id, "error_code": error_code.to_lowercase()}),
                );
                active_command.store(false, Ordering::Release);
                return;
            }
        };
        let started = match client.start_task_with_client_message_id(
            &thread_id,
            &prompt,
            Some(&command_id),
        ) {
            Ok(started) => started,
            Err(error) => {
                queue_event(
                    &outgoing,
                    "command.failed",
                    request_id,
                    json!({"command_id": command_id, "error_code": error.public_code().to_lowercase()}),
                );
                active_command.store(false, Ordering::Release);
                return;
            }
        };
        if let Ok(mut active) = active_turn.lock() {
            *active = Some((
                command_id.clone(),
                started.thread_id.clone(),
                started.turn_id.clone(),
            ));
        }
        queue_event(
            &outgoing,
            "command.started",
            request_id.clone(),
            json!({"command_id": command_id, "turn_id": started.turn_id}),
        );
        if is_cancelled(&cancelled, &command_id) {
            let _ = client.interrupt(&started.thread_id, &started.turn_id);
        }
        match client.wait_for_turn_completion(&started.turn_id, COMMAND_TIMEOUT) {
            Ok(completion) => {
                for (index, item) in completion.items.iter().enumerate() {
                    queue_event(
                        &outgoing,
                        "command.item",
                        request_id.clone(),
                        json!({
                            "command_id": command_id,
                            "thread_id": started.thread_id,
                            "turn_id": started.turn_id,
                            "item": relay_item(item, index as i64)
                        }),
                    );
                }
                if completion.status == "failed" {
                    queue_event(
                        &outgoing,
                        "command.failed",
                        request_id,
                        json!({"command_id": command_id, "error_code": "codex_turn_failed"}),
                    );
                } else {
                    queue_event(
                        &outgoing,
                        "command.completed",
                        request_id,
                        json!({"command_id": command_id, "status": completion.status}),
                    );
                }
            }
            Err(error) => queue_event(
                &outgoing,
                "command.failed",
                request_id,
                json!({"command_id": command_id, "error_code": error.public_code().to_lowercase()}),
            ),
        }
        if let Ok(mut active) = active_turn.lock() {
            *active = None;
        }
        if let Ok(mut cancelled_commands) = cancelled.lock() {
            cancelled_commands.remove(&command_id);
        }
        active_command.store(false, Ordering::Release);
    });
}

fn request_cancel(
    event: EventEnvelope,
    codex: SharedCodexClient,
    active_turn: Arc<Mutex<Option<(String, String, String)>>>,
    cancelled: Arc<Mutex<HashSet<String>>>,
) {
    let Some(command_id) = event
        .payload
        .get("command_id")
        .and_then(Value::as_str)
        .map(str::to_owned)
    else {
        return;
    };
    if let Ok(mut cancelled_commands) = cancelled.lock() {
        cancelled_commands.insert(command_id.clone());
    }
    let active = active_turn
        .lock()
        .ok()
        .and_then(|active| active.clone())
        .filter(|(active_command, _, _)| active_command == &command_id);
    if let Some((_, thread_id, turn_id)) = active {
        tokio::task::spawn_blocking(move || {
            if let Ok(client) = current_codex(&codex) {
                let _ = client.interrupt(&thread_id, &turn_id);
            }
        });
    }
}

fn relay_item(item: &NormalizedCompletedItem, sequence: i64) -> Value {
    let created_at = Utc
        .timestamp_millis_opt(item.completed_at_ms)
        .single()
        .unwrap_or_else(Utc::now)
        .to_rfc3339_opts(SecondsFormat::Millis, true);
    let mut output = Map::from_iter([
        ("item_id".to_owned(), json!(item.item_id)),
        ("sequence".to_owned(), json!(sequence)),
        ("type".to_owned(), json!(item.item_type)),
        ("status".to_owned(), json!(item.status)),
        ("created_at".to_owned(), json!(created_at)),
    ]);
    if let Some(role) = &item.role {
        output.insert("role".to_owned(), json!(role));
    }
    if let Some(title) = &item.title {
        output.insert("title".to_owned(), json!(title));
    }
    if let Some(summary) = &item.summary {
        output.insert("summary".to_owned(), json!(summary));
    }
    if let Some(text) = &item.text {
        output.insert(
            "content".to_owned(),
            json!([{"type": "markdown", "text": text}]),
        );
    }
    Value::Object(output)
}

fn historical_relay_item(item: &NormalizedHistoricalItem, sequence: i64) -> Value {
    let mut value = relay_item(&item.item, sequence);
    if let Some(object) = value.as_object_mut() {
        object.insert("turn_id".to_owned(), json!(item.turn_id));
    }
    value
}

fn session_items(page: ThreadPage) -> (Vec<CodexSession>, Option<String>) {
    let items = page
        .data
        .into_iter()
        .map(|thread| {
            let updated_at = thread
                .updated_at
                .and_then(|timestamp| Utc.timestamp_opt(timestamp, 0).single())
                .unwrap_or_else(Utc::now)
                .to_rfc3339_opts(SecondsFormat::Secs, true);
            let write_state = if thread.status == "active" {
                "busy"
            } else if thread.can_write && thread.status == "notLoaded" {
                "writable_resumable"
            } else if thread.can_write {
                "writable_loaded"
            } else {
                "read_only"
            };
            CodexSession {
                thread_id: thread.id,
                title: thread.title,
                preview: None,
                cwd_display: thread.cwd_label,
                created_at: None,
                updated_at,
                status: Some(thread.status),
                archived: false,
                write_state: write_state.to_owned(),
                write_state_reason: None,
            }
        })
        .collect();
    (items, page.next_cursor)
}

async fn send_event<S>(
    socket: &mut tokio_tungstenite::WebSocketStream<S>,
    sequence: &AtomicI64,
    event: OutboundEvent,
) -> Result<(), ()>
where
    S: tokio::io::AsyncRead + tokio::io::AsyncWrite + Unpin,
{
    let envelope = EventEnvelope {
        v: 1,
        r#type: event.event_type.to_owned(),
        event_id: Uuid::new_v4().to_string(),
        request_id: event.request_id,
        sequence: sequence.fetch_add(1, Ordering::Relaxed),
        occurred_at: Utc::now().to_rfc3339_opts(SecondsFormat::Millis, true),
        payload: event.payload,
    };
    let encoded = serde_json::to_string(&envelope).map_err(|_| ())?;
    if encoded.len() > MAX_WS_MESSAGE_BYTES {
        return Err(());
    }
    socket
        .send(Message::Text(encoded.into()))
        .await
        .map_err(|_| ())
}

fn websocket_request(
    context: &PanelConnectionContext,
    device_id: &str,
) -> Result<tokio_tungstenite::tungstenite::http::Request<()>, &'static str> {
    let mut url = Url::parse(&context.site_url).map_err(|_| "COLLAB_INVALID_SITE")?;
    let scheme = match url.scheme() {
        "https" => "wss",
        "http" => "ws",
        _ => return Err("COLLAB_INVALID_SITE"),
    };
    url.set_scheme(scheme).map_err(|_| "COLLAB_INVALID_SITE")?;
    url = url
        .join("api/v1/collaboration/ws")
        .map_err(|_| "COLLAB_INVALID_SITE")?;
    let mut request = url
        .as_str()
        .into_client_request()
        .map_err(|_| "COLLAB_INVALID_SITE")?;
    let authorization = Zeroizing::new(format!("Bearer {}", context.access_token.as_str()));
    request.headers_mut().insert(
        "Authorization",
        HeaderValue::from_str(authorization.as_str()).map_err(|_| "COLLAB_INVALID_TOKEN")?,
    );
    request
        .headers_mut()
        .insert("X-Sub2API-Client-Type", HeaderValue::from_static("pc"));
    request.headers_mut().insert(
        "X-Sub2API-Device-ID",
        HeaderValue::from_str(device_id).map_err(|_| "COLLAB_INVALID_DEVICE")?,
    );
    request
        .headers_mut()
        .insert("X-Sub2API-Protocol-Version", HeaderValue::from_static("1"));
    Ok(request)
}

fn queue_event(
    outgoing: &mpsc::UnboundedSender<OutboundEvent>,
    event_type: &'static str,
    request_id: Option<String>,
    payload: Value,
) {
    let _ = outgoing.send(OutboundEvent {
        event_type,
        request_id,
        payload: map_from(payload),
    });
}

fn map_from(value: Value) -> Map<String, Value> {
    value.as_object().cloned().unwrap_or_default()
}

fn optional_string(payload: &Map<String, Value>, key: &str, max_len: usize) -> Option<String> {
    payload
        .get(key)
        .and_then(Value::as_str)
        .filter(|value| value.len() <= max_len)
        .map(str::to_owned)
}

fn current_codex(codex: &SharedCodexClient) -> Result<Arc<AppServerClient>, &'static str> {
    codex
        .lock()
        .map_err(|_| "CODEX_STATE_ERROR")?
        .as_ref()
        .cloned()
        .ok_or("CODEX_NOT_RUNNING")
}

fn with_codex<T>(
    codex: &SharedCodexClient,
    operation: impl FnOnce(&AppServerClient) -> Result<T, crate::codex_adapter::AdapterError>,
) -> Result<T, &'static str> {
    let client = current_codex(codex)?;
    operation(&client).map_err(|error| error.public_code())
}

fn codex_is_ready(codex: &SharedCodexClient) -> bool {
    codex.lock().map(|client| client.is_some()).unwrap_or(false)
}

fn is_cancelled(cancelled: &Mutex<HashSet<String>>, command_id: &str) -> bool {
    cancelled
        .lock()
        .map(|commands| commands.contains(command_id))
        .unwrap_or(true)
}

fn update_status(
    status: &Mutex<RelayStatus>,
    state: &str,
    device_id: Option<&str>,
    last_error: Option<&str>,
) {
    let mut status = status.lock().unwrap_or_else(|error| error.into_inner());
    status.state = state.to_owned();
    status.device_id = device_id.map(str::to_owned);
    status.last_error = last_error.map(str::to_owned);
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn websocket_request_uses_headers_and_never_query_tokens() {
        let context = PanelConnectionContext {
            site_url: "https://panel.example.com/base/".to_owned(),
            access_token: Zeroizing::new("jwt-secret".to_owned()),
        };
        let request = websocket_request(&context, "018f7f3e-86f6-7cc8-98ec-4f56dc1f2321").unwrap();
        assert_eq!(
            request.uri().to_string(),
            "wss://panel.example.com/base/api/v1/collaboration/ws"
        );
        assert!(request.uri().query().is_none());
        assert_eq!(request.headers()["X-Sub2API-Client-Type"], "pc");
        assert_eq!(request.headers()["X-Sub2API-Protocol-Version"], "1");
    }

    #[test]
    fn relay_command_item_contains_no_raw_command_or_path_fields() {
        let item = NormalizedCompletedItem {
            item_id: "item_1".to_owned(),
            item_type: "command_execution".to_owned(),
            role: None,
            title: Some("命令执行".to_owned()),
            summary: Some("completed · exit 0".to_owned()),
            text: None,
            status: "completed".to_owned(),
            completed_at_ms: 123,
        };
        let value = relay_item(&item, 1);
        let object = value.as_object().unwrap();
        for forbidden in ["command", "cwd", "stderr", "raw", "source_path"] {
            assert!(!object.contains_key(forbidden));
        }
        assert_eq!(object.get("type"), Some(&json!("command_execution")));
    }

    #[test]
    fn session_mapping_only_uses_sanitized_path_label() {
        let page = ThreadPage {
            data: vec![crate::codex_adapter::NormalizedThread {
                id: "thread_1".to_owned(),
                title: "Fix login".to_owned(),
                cwd_label: Some("…/project".to_owned()),
                status: "notLoaded".to_owned(),
                can_write: true,
                updated_at: Some(123),
            }],
            next_cursor: None,
        };
        let (items, _) = session_items(page);
        let json = serde_json::to_string(&items).unwrap();
        assert!(json.contains("…/project"));
        assert!(!json.contains("/home/"));
    }
}
